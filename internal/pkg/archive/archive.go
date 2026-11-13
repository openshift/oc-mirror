package archive

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	digest "github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/history"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/signature"
)

type MirrorArchive struct {
	Archiver
	adder                  archiveAdder
	destination            string
	iscPath                string
	workingDir             string
	cacheDir               string
	history                history.History
	blobGatherer           BlobsGatherer
	maxSize                int64
	strictArchiving        bool
	ignoreReleaseSignature bool
	logger                 clog.PluggableLoggerInterface
}

// NewMirrorArchive creates a new MirrorArchive instance
func NewMirrorArchive(opts *mirror.CopyOptions, destination, iscPath, workingDir, cacheDir string, maxSize int64, logg clog.PluggableLoggerInterface) (*MirrorArchive, error) {
	// create the history interface
	history, err := history.NewHistory(workingDir, opts.Global.Since, logg, history.OSFileCreator{})
	if err != nil {
		return &MirrorArchive{}, err
	}

	bg := NewImageBlobGatherer(opts, logg)

	if maxSize == 0 {
		maxSize = defaultSegSize
	}
	maxSize *= segMultiplier

	ma := MirrorArchive{
		destination:            destination,
		history:                history,
		blobGatherer:           bg,
		workingDir:             workingDir,
		cacheDir:               cacheDir,
		iscPath:                iscPath,
		maxSize:                maxSize,
		strictArchiving:        opts.Global.StrictArchiving,
		ignoreReleaseSignature: opts.Global.IgnoreReleaseSignature,
		logger:                 logg,
	}
	return &ma, nil
}

// BuildArchive creates an archive that contains:
// * docker/v2/repositories : manifests for all mirrored images
// * docker/v2/blobs/sha256 : blobs that haven't been mirrored (diff)
// * working-dir
// * image set config
//
// onBlobsGathered, if non-nil, runs once the image blobs diff is gathered (the last step
// needing the local registry) and before working-dir - which includes the registry's
// still-open log file - is archived. This lets callers stop the registry at the right
// time, avoiding a race between the tar header size and the log file still growing.
func (o *MirrorArchive) BuildArchive(ctx context.Context, schema v2alpha1.CollectorSchema, onBlobsGathered func()) error {
	if err := o.createTarball(); err != nil {
		return fmt.Errorf("unable to create the mirror archive: %w", err)
	}

	// 0 - make sure that any tarWriters or files opened by the adder are closed as we leave this method
	defer o.adder.close()
	addedBlobs, err := o.addContents(ctx, schema, onBlobsGathered)
	if err != nil {
		return err
	}

	// 6 - update history file with addedBlobs
	_, err = o.history.Append(addedBlobs)
	if err != nil {
		return fmt.Errorf("unable to update history metadata: %w", err)
	}

	return nil
}

// addContents is shared by archive creation and its metadata-only estimate.
func (o *MirrorArchive) addContents(ctx context.Context, schema v2alpha1.CollectorSchema, onBlobsGathered func()) (sets.Set[string], error) {
	// 1 - Add files and directories under the cache's docker/v2/repositories to the archive
	repositoriesDir := filepath.Join(o.cacheDir, cacheRepositoriesDir)
	err := o.adder.addAllFolder(repositoriesDir, o.cacheDir)
	if err != nil {
		return nil, fmt.Errorf("unable to add cache repositories to the archive : %w", err)
	}
	// 2 - Add blobs
	blobsInHistory, err := o.history.Read()
	if err != nil && !errors.Is(err, &history.EmptyHistoryError{}) {
		return nil, fmt.Errorf("unable to read history metadata from working-dir : %w", err)
	}
	// ignoring the error otherwise: continuing with an empty map in blobsInHistory

	addedBlobs, err := o.addImagesDiff(ctx, schema, blobsInHistory)
	if err != nil {
		return nil, fmt.Errorf("unable to add image blobs to the archive : %w", err)
	}

	// 3 - local registry no longer needed: let the caller stop it before working-dir is added
	if onBlobsGathered != nil {
		onBlobsGathered()
	}

	// 4- Add working-dir contents to archive
	err = o.adder.addAllFolder(o.workingDir, filepath.Dir(o.workingDir))
	if err != nil {
		return nil, fmt.Errorf("unable to add working-dir to the archive : %w", err)
	}
	// 5 - Add imageSetConfig
	iscName := imageSetConfigPrefix + time.Now().UTC().Format(time.RFC3339)
	err = o.adder.addFile(o.iscPath, iscName)
	if err != nil {
		return nil, fmt.Errorf("unable to add image set configuration to the archive : %w", err)
	}
	return addedBlobs, nil
}

// createTarball creates a tarball, there are two possible implementations currently:
// strictAdder: any files that exceed the maxArchiveSize specified in the imageSetConfig will
// cause the BuildArchive method to stop and return in error.
//
// permissiveAdder:
// any files that exceed the maxArchiveSize specified in the imageSetConfig will
// be added to standalone archives, and flagged in a warning at the end of the execution
func (o *MirrorArchive) createTarball() error {
	var err error
	var adder archiveAdder

	if o.strictArchiving {
		adder, err = newStrictAdder(o.maxSize, o.destination, o.logger)
	} else {
		adder, err = newPermissiveAdder(o.maxSize, o.destination, o.logger)
	}

	if err != nil {
		return err
	}

	o.adder = adder

	return nil
}

func (o *MirrorArchive) addImagesDiff(ctx context.Context, schema v2alpha1.CollectorSchema, historyBlobs sets.Set[string]) (sets.Set[string], error) {
	allAddedBlobs := sets.New[string]()
	for _, img := range schema.AllImages {
		platformFilters := schema.PlatformFilters[img.Origin]
		allowedPlatforms := make([]string, len(platformFilters))
		for i, p := range platformFilters {
			allowedPlatforms[i] = p.String()
		}
		imgBlobs, err := o.blobGatherer.GatherBlobs(ctx, img.Destination, allowedPlatforms)
		var sigErr *SignatureBlobGathererError
		if err != nil && !errors.As(err, &sigErr) {
			return nil, fmt.Errorf("unable to find blobs corresponding to %s: %w", img.Destination, err)
		}

		if err := handleSignatureErrors(img, err); err != nil {
			var archiveErr *ArchiveError
			if errors.As(err, &archiveErr) && archiveErr.ReleaseErr != nil {
				if o.ignoreReleaseSignature {
					o.logger.Warn("[archive] skipping missing cosign signature for release image %s (--ignore-release-signature is set): %v", img.Destination, archiveErr.ReleaseErr)
				} else {
					return nil, archiveErr
				}
			}
		}

		addedBlobs, err := o.addBlobsDiff(imgBlobs, historyBlobs, allAddedBlobs)
		if err != nil {
			return nil, fmt.Errorf("unable to add blobs corresponding to %s: %w", img.Destination, err)
		}

		allAddedBlobs = allAddedBlobs.Union(addedBlobs)
	}

	return allAddedBlobs, nil
}

func (o *MirrorArchive) addBlobsDiff(collectedBlobs, historyBlobs, alreadyAddedBlobs sets.Set[string]) (sets.Set[string], error) {
	blobsInDiff := sets.New[string]()
	for hash := range collectedBlobs {
		if historyBlobs.Has(hash) || alreadyAddedBlobs.Has(hash) {
			continue
		}
		// Add to tar
		blobPath, err := BlobPath(o.cacheDir, digest.Digest(hash))
		if err != nil {
			return nil, err
		}
		err = o.adder.addAllFolder(filepath.Dir(blobPath), o.cacheDir)
		if err != nil {
			return nil, err
		}
		blobsInDiff.Insert(hash)
	}
	return blobsInDiff, nil
}

func RemovePastArchives(destination string) error {
	if _, err := os.Stat(destination); err != nil {
		// Destination directory doesn't exist: no-op
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to get past archives: %w", err)
	}
	const globPat string = "mirror_*.tar"
	files, err := filepath.Glob(filepath.Join(destination, globPat))
	if err != nil {
		return fmt.Errorf("error getting glob %q matches: %w", globPat, err)
	}
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("error removing tar file: %w", err)
		}
	}
	return nil
}

func handleSignatureErrors(img v2alpha1.CopyImageSchema, err error) error {
	var sigErr *SignatureBlobGathererError
	if err == nil || !errors.As(err, &sigErr) {
		return nil
	}

	switch {
	case img.Type.IsOperator() && img.RebuiltTag == "":
		return &ArchiveError{OperatorErr: sigErr.SigError}
	case img.Type.IsRelease() && img.Type != v2alpha1.TypeCincinnatiGraph:
		return &ArchiveError{ReleaseErr: sigErr.SigError}
	case img.Type.IsAdditionalImage():
		return &ArchiveError{AdditionalImgErr: sigErr.SigError}
	case img.Type.IsHelmImage():
		return &ArchiveError{HelmErr: sigErr.SigError}
	}

	return nil
}

// BlobPath returns the data file used by the local registry for a blob.
func BlobPath(cacheDir string, d digest.Digest) (string, error) {
	if err := d.Validate(); err != nil {
		return "", fmt.Errorf("invalid blob digest: %w", err)
	}
	return filepath.Join(cacheDir, cacheBlobsDir, d.Algorithm().String(), d.Encoded()[:2], d.Encoded(), "data"), nil
}

// CacheMetadata describes registry link files for the inspected images. The last
// descriptor of each image is its top-level manifest, as returned by InspectImage.
// Keys are paths relative to the registry root; values are file lengths.
func CacheMetadata(images []v2alpha1.CopyImageSchema, blobsBySource map[string][]types.BlobInfo) (map[string]int64, error) {
	files := make(map[string]int64)
	for _, img := range images {
		if err := addCacheImageMetadata(files, img, blobsBySource[img.Source]); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func addCacheImageMetadata(files map[string]int64, img v2alpha1.CopyImageSchema, blobs []types.BlobInfo) error {
	ref, err := image.ParseRef(img.Destination)
	if err != nil {
		return err
	}
	root := filepath.Join(cacheRepositoriesDir, ref.PathComponent)
	for _, blob := range blobs {
		if err := blob.Digest.Validate(); err != nil {
			return fmt.Errorf("invalid metadata digest: %w", err)
		}
		section := "_layers"
		if mirror.IsManifest(blob.MediaType) {
			section = filepath.Join("_manifests", "revisions")
		}
		files[filepath.Join(root, section, blob.Digest.Algorithm().String(), blob.Digest.Encoded(), "link")] = int64(len(blob.Digest.String()))
		if signed := blob.Annotations[consts.SignatureFor]; signed != "" {
			tag, err := signature.SigstoreAttachmentTag(digest.Digest(signed))
			if err != nil {
				return err
			}
			addTagMetadata(files, root, tag, blob.Digest)
		}
	}
	if len(blobs) > 0 && ref.Tag != "" && mirror.IsManifest(blobs[len(blobs)-1].MediaType) {
		addTagMetadata(files, root, ref.Tag, blobs[len(blobs)-1].Digest)
	}
	return nil
}

func addTagMetadata(files map[string]int64, root, tag string, d digest.Digest) {
	tags := filepath.Join(root, "_manifests", "tags", tag)
	files[filepath.Join(tags, "current", "link")] = int64(len(d.String()))
	files[filepath.Join(tags, "index", d.Algorithm().String(), d.Encoded(), "link")] = int64(len(d.String()))
}

// EstimateSize predicts a new archive without copying blobs or updating history.
// Its filesystem inputs are a snapshot; catalogs must be prepared beforehand,
// and files generated or changed later can alter the final archive size.
func EstimateSize(ctx context.Context, opts *mirror.CopyOptions, cacheDir string, schema v2alpha1.CollectorSchema, blobsBySource map[string][]types.BlobInfo, cacheMetadata map[string]int64, archiveSize int64, logger clog.PluggableLoggerInterface) (int64, error) {
	archiver, err := NewMirrorArchive(opts, "", opts.Global.ConfigPath, opts.Global.WorkingDir, cacheDir, archiveSize, logger)
	if err != nil {
		return 0, err
	}
	metadata, err := CacheMetadata(schema.AllImages, blobsBySource)
	if err != nil {
		return 0, err
	}
	maps.Copy(metadata, cacheMetadata)
	counter := &sizeAdder{maxSize: archiver.maxSize, strict: opts.Global.StrictArchiving, total: 1024, files: make(map[string]bool), pending: make(map[string]int64)}
	for name, size := range metadata {
		counter.pending[filepath.Join(cacheDir, name)] = size
	}
	gathered := make(map[string]sets.Set[string])
	for _, img := range schema.AllImages {
		if gathered[img.Destination] == nil {
			gathered[img.Destination] = sets.New[string]()
		}
		for _, blob := range blobsBySource[img.Source] {
			path, err := BlobPath(cacheDir, blob.Digest)
			if err != nil {
				return 0, err
			}
			counter.pending[path] = blob.Size
			gathered[img.Destination].Insert(blob.Digest.String())
		}
	}
	counter.pendingPaths = slices.Sorted(maps.Keys(counter.pending))
	archiver.adder = counter
	archiver.blobGatherer = descriptorGatherer(gathered)
	_, err = archiver.addContents(ctx, schema, nil)
	return counter.total, err
}

// Adapt the already inspected descriptors to the existing archive selection.
type descriptorGatherer map[string]sets.Set[string]

func (g descriptorGatherer) GatherBlobs(_ context.Context, destination string, _ []string) (sets.Set[string], error) {
	return g[destination], nil
}
