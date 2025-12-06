package archive

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	digest "github.com/opencontainers/go-digest"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/history"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type MirrorArchive struct {
	Archiver
	adder           archiveAdder
	destination     string
	iscPath         string
	workingDir      string
	cacheDir        string
	history         history.History
	blobGatherer    BlobsGatherer
	maxSize         int64
	strictArchiving bool
	logger          clog.PluggableLoggerInterface
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
		destination:     destination,
		history:         history,
		blobGatherer:    bg,
		workingDir:      workingDir,
		cacheDir:        cacheDir,
		iscPath:         iscPath,
		maxSize:         maxSize,
		strictArchiving: opts.Global.StrictArchiving,
		logger:          logg,
	}
	return &ma, nil
}

// BuildArchive creates an archive that contains:
// * docker/v2/repositories : manifests for all mirrored images
// * docker/v2/blobs/sha256 : blobs that haven't been mirrored (diff)
// * working-dir
// * image set config
func (o *MirrorArchive) BuildArchive(ctx context.Context, collectedImages []v2alpha1.CopyImageSchema) error {
	if err := o.createTarball(); err != nil {
		return fmt.Errorf("unable to create the mirror archive: %w", err)
	}

	// 0 - make sure that any tarWriters or files opened by the adder are closed as we leave this method
	defer o.adder.close()
	// 1 - Add files and directories under the cache's docker/v2/repositories to the archive
	repositoriesDir := filepath.Join(o.cacheDir, cacheRepositoriesDir)
	err := o.adder.addAllFolder(repositoriesDir, o.cacheDir)
	if err != nil {
		return fmt.Errorf("unable to add cache repositories to the archive : %w", err)
	}
	// 2- Add working-dir contents to archive
	err = o.adder.addAllFolder(o.workingDir, filepath.Dir(o.workingDir))
	if err != nil {
		return fmt.Errorf("unable to add working-dir to the archive : %w", err)
	}
	// 3 - Add imageSetConfig
	iscName := imageSetConfigPrefix + time.Now().UTC().Format(time.RFC3339)
	err = o.adder.addFile(o.iscPath, iscName)
	if err != nil {
		return fmt.Errorf("unable to add image set configuration to the archive : %w", err)
	}
	// 4 - Add blobs
	blobsInHistory, err := o.history.Read()
	if err != nil && !errors.Is(err, &history.EmptyHistoryError{}) {
		return fmt.Errorf("unable to read history metadata from working-dir : %w", err)
	}
	// ignoring the error otherwise: continuing with an empty map in blobsInHistory

	addedBlobs, err := o.addImagesDiff(ctx, collectedImages, blobsInHistory)
	if err != nil {
		return fmt.Errorf("unable to add image blobs to the archive : %w", err)
	}
	// 5 - update history file with addedBlobs
	_, err = o.history.Append(addedBlobs)
	if err != nil {
		return fmt.Errorf("unable to update history metadata: %w", err)
	}

	return nil
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

func (o *MirrorArchive) addImagesDiff(ctx context.Context, collectedImages []v2alpha1.CopyImageSchema, historyBlobs map[string]struct{}) (map[string]struct{}, error) {
	allAddedBlobs := make(map[string]struct{})
	for _, img := range collectedImages {
		imgBlobs, err := o.blobGatherer.GatherBlobs(ctx, img.Destination)
		var sigErr *SignatureBlobGathererError
		if err != nil && !errors.As(err, &sigErr) {
			return nil, fmt.Errorf("unable to find blobs corresponding to %s: %w", img.Destination, err)
		}

		// Handle signature errors - only return error if it's a fatal signature error
		if sigHandleErr := handleSignatureErrors(img, err); sigHandleErr != nil {
			var archiveErr *ArchiveError
			if errors.As(sigHandleErr, &archiveErr) && archiveErr.ReleaseErr != nil {
				// For release images, signature errors should not be fatal
				// Log the error but continue processing
				o.logger.Warn("signature error for release image %s: %v", img.Destination, archiveErr.ReleaseErr)
			} else if errors.As(sigHandleErr, &archiveErr) {
				// For other image types, return the error
				return nil, archiveErr
			}
		}

		addedBlobs, err := o.addBlobsDiff(imgBlobs, historyBlobs, allAddedBlobs)
		if err != nil {
			return nil, fmt.Errorf("unable to add blobs corresponding to %s: %w", img.Destination, err)
		}

		for hash, value := range addedBlobs {
			allAddedBlobs[hash] = value
		}

	}

	return allAddedBlobs, nil
}

func (o *MirrorArchive) addBlobsDiff(collectedBlobs, historyBlobs map[string]struct{}, alreadyAddedBlobs map[string]struct{}) (map[string]struct{}, error) {
	blobsInDiff := make(map[string]struct{})
	for hash := range collectedBlobs {
		_, alreadyMirrored := historyBlobs[hash]
		_, previouslyAdded := alreadyAddedBlobs[hash]
		skip := alreadyMirrored || previouslyAdded
		if !skip {
			// Add to tar
			d, err := digest.Parse(hash)
			if err != nil {
				return nil, fmt.Errorf("error parsing digest %w", err)
			}
			blobPath := filepath.Join(o.cacheDir, cacheBlobsDir, d.Algorithm().String(), d.Encoded()[:2], d.Encoded())
			err = o.adder.addAllFolder(blobPath, o.cacheDir)
			if err != nil {
				return nil, err
			}
			blobsInDiff[hash] = struct{}{}
		}
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
