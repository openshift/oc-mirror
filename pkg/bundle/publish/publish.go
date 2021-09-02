package publish

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/mholt/archiver/v3"
	"github.com/openshift/oc/pkg/cli/image/mirror"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/RedHatGov/bundle/pkg/archive"
	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/RedHatGov/bundle/pkg/image"
	"github.com/RedHatGov/bundle/pkg/metadata/storage"
)

type UuidError struct {
	InUuid   uuid.UUID
	CurrUuid uuid.UUID
}

func (u *UuidError) Error() string {
	return fmt.Sprintf("Mismatched UUIDs. Want %v, got %v", u.CurrUuid, u.InUuid)
}

type SequenceError struct {
	inSeq   int
	CurrSeq int
}

func (s *SequenceError) Error() string {
	return fmt.Sprintf("Bundle Sequence out of order. Current sequence %v, incoming sequence %v", s.CurrSeq, s.inSeq)
}

func (o *Options) Run(ctx context.Context) error {

	var currentMeta v1alpha1.Metadata
	var incomingMeta v1alpha1.Metadata
	a := archive.NewArchiver()

	logrus.Infof("Publish image set from archive %q to registry %q", o.ArchivePath, o.ToMirror)

	// Create workspace
	tmpdir, err := ioutil.TempDir(o.Dir, "imageset")
	if err != nil {
		return err
	}

	logrus.Debugf("Created temporary directory %s", tmpdir)

	if !o.SkipCleanup {
		defer os.RemoveAll(tmpdir)
	}

	// Get file information from the source archives
	filesInArchive, err := o.readImageSet(a)
	if err != nil {
		return err
	}

	target := filepath.Join(config.PublishDir, config.MetadataFile)
	dest := filepath.Join(o.Dir, target)

	// Create backend for o.Dir
	backend, err := storage.NewLocalBackend(o.Dir)
	if err != nil {
		return fmt.Errorf("error opening local backend: %v", err)
	}

	// Create a local workspace backend
	workspace, err := storage.NewLocalBackend(tmpdir)
	if err != nil {
		return fmt.Errorf("error opening local backend: %v", err)
	}

	// Check for existing metadata. Metadata will be extracted before
	// the extraction of the archive so imageset mismatches can
	// be handled before the longer unarchiving process
	if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {

		logrus.Infof("No existing metadata found. Setting up new workspace")

		// Extract incoming metadata
		archive, ok := filesInArchive[config.MetadataFile]
		if !ok {
			return errors.New("metadata is not in archive")
		}

		logrus.Debug("Extracting incoming metadta ")
		if err := a.Extract(archive, target, tmpdir); err != nil {
			return err
		}

		// Find first file and load metadata from that
		if err := workspace.ReadMetadata(ctx, &incomingMeta, target); err != nil {
			return fmt.Errorf("error reading incoming metadata: %v", err)
		}

	} else {

		// Extract incoming metadata
		archive, ok := filesInArchive[config.MetadataFile]
		if !ok {
			return errors.New("metadata is not in archive")
		}

		logrus.Debug("Extract incoming metadata")
		if err := a.Extract(archive, target, tmpdir); err != nil {
			return err
		}

		// Compare metadata UID and sequence number
		if err := backend.ReadMetadata(ctx, &currentMeta, target); err != nil {
			return fmt.Errorf("error reading current metadata: %v", err)
		}

		if err := workspace.ReadMetadata(ctx, &incomingMeta, target); err != nil {
			return fmt.Errorf("error reading incoming metadata: %v", err)
		}

		logrus.Debug("Checking metadata UID")
		if incomingMeta.MetadataSpec.Uid != currentMeta.MetadataSpec.Uid {
			return &UuidError{currentMeta.MetadataSpec.Uid, incomingMeta.MetadataSpec.Uid}
		}

		logrus.Debug("Check metadata sequence number")
		currRun := currentMeta.PastMirrors[len(currentMeta.PastMirrors)-1]
		incomingRun := incomingMeta.PastMirrors[len(incomingMeta.PastMirrors)-1]
		if incomingRun.Sequence != (currRun.Sequence + 1) {
			return &SequenceError{incomingRun.Sequence, currRun.Sequence}
		}
	}

	// Unarchive full imageset after metadata checks
	if err := o.getImageSet(a, tmpdir); err != nil {
		return err
	}

	// Load image associations to find layers not present locally.
	assocPath := filepath.Join(tmpdir, config.InternalDir, config.AssociationsFile)

	assocs, err := readAssociations(assocPath)
	if err != nil {
		return err
	}

	var (
		errs           []error
		listTypeAssocs []image.Association
	)
	for imageName, assoc := range assocs {

		assoc := assoc

		// Skip handling list-type images until all manifest layers have been pulled.
		if len(assoc.ManifestDigests) != 0 {
			listTypeAssocs = append(listTypeAssocs, assoc)
			// Validate that each manifest has an association as a sanity check.
			logrus.Debugf("Found %d manifests for image %s", len(assoc.ManifestDigests), imageName)
			for _, manifestDigest := range assoc.ManifestDigests {
				if _, hasManifest := assocs[manifestDigest]; !hasManifest {
					errs = append(errs, fmt.Errorf("image %q: expected associations to have manifest %s but was not found", imageName, manifestDigest))
				}
			}
		}

		for _, layerDigest := range assoc.LayerDigests {
			logrus.Debugf("Found layer %v for image %s", layerDigest, imageName)
			// Construct blob path, which is adjacent to the manifests path.
			// If a layer exists in the archive (err == nil), copy the layer
			// in the expected location.
			blobPath := filepath.Join("blobs", layerDigest)
			if _, err := os.Stat(filepath.Join(tmpdir, blobPath)); err != nil && errors.Is(err, os.ErrNotExist) {

				logrus.Debugf("Blob %s not found in archive", blobPath)
				// Image layer must exist in the mirror registry since it wasn't archived,
				// so GET the layer and place it in the blob dir so it can be mirrored by `oc`.

				// TODO: implement layer pulling.

			} else if err != nil {
				errs = append(errs, fmt.Errorf("access image %q blob %q at %s: %v", imageName, layerDigest, blobPath, err))
			} else {

				logrus.Debugf("Found blob %s", blobPath)

				// Copy blob to expected path
				archive, ok := filesInArchive[layerDigest]
				if !ok {
					errs = append(errs, fmt.Errorf("layer digest %s not found in archive", layerDigest))
				}

				logrus.Debugf("Extracting blob %s", blobPath)
				err := a.Extract(archive, blobPath, filepath.Join(tmpdir, assoc.Path))
				if err != nil {
					errs = append(errs, fmt.Errorf("error extracting blob %s: %v", layerDigest, err))
				}
			}
		}
	}
	if len(errs) != 0 {
		return utilerrors.NewAggregate(errs)
	}

	// Build, checksum, and push manifest lists and index images.
	for _, assoc := range listTypeAssocs {
		_ = assoc

		// TODO: build manifest list or index depending on media type in the manifest file.

		// TODO: use image lib to checksum content (this might be done implicitly).

		// TODO: push image to mirror registry.
	}
	if len(errs) != 0 {
		return utilerrors.NewAggregate(errs)
	}

	// import imagecontentsourcepolicy
	logrus.Info("ICSP importing not implemented")

	// import catalogsource
	logrus.Info("CatalogSource importing not implemented")

	// install imagecontentsourcepolicy
	logrus.Info("ICSP creation not implemented")

	// install catalogsource
	logrus.Info("CatalogSource creation not implemented")

	// Replace old metadata with new metadata
	if err := backend.WriteMetadata(context.Background(), &incomingMeta, target); err != nil {
		return err
	}

	return nil
}

// mirrortoReg will process a mirror mapping and push to the destination registry
func (o *Options) mirrorToReg(mappings []mirror.Mapping) error {

	iostreams := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	imageOpts := mirror.NewMirrorImageOptions(iostreams)
	imageOpts.FromFileDir = o.Dir
	imageOpts.Mappings = mappings
	imageOpts.SecurityOptions.Insecure = o.SkipTLS
	imageOpts.DryRun = o.DryRun

	return imageOpts.Run()
}

// readAssociations will process and return data from the image associations file
func readAssociations(assocPath string) (assocs image.Associations, err error) {

	f, err := os.Open(assocPath)
	if err != nil {
		return assocs, fmt.Errorf("error opening image associations file: %v", err)
	}
	defer f.Close()

	return assocs, assocs.Decode(f)
}

// getImage unarchives all provided tar archives
func (o *Options) getImageSet(a archive.Archiver, dest string) error {

	file, err := os.Stat(o.ArchivePath)
	if err != nil {
		return err
	}

	if file.IsDir() {

		err = filepath.Walk(o.ArchivePath, func(path string, info os.FileInfo, err error) error {

			if err != nil {
				return fmt.Errorf("traversing %s: %v", path, err)
			}
			if info == nil {
				return fmt.Errorf("no file info")
			}

			extension := filepath.Ext(path)
			extension = strings.TrimPrefix(extension, ".")

			if extension == a.String() {
				logrus.Debugf("Extracting archive %s", path)
				if err := a.Unarchive(path, dest); err != nil {
					return err
				}
			}

			return nil
		})

	} else {

		logrus.Infof("Extracting archive %s", o.ArchivePath)
		if err := a.Unarchive(o.ArchivePath, dest); err != nil {
			return err
		}
	}

	return err
}

// readImage set will create a map with all the files located in the archives
func (o *Options) readImageSet(a archive.Archiver) (map[string]string, error) {

	filesinArchive := make(map[string]string)

	file, err := os.Stat(o.ArchivePath)
	if err != nil {
		return nil, err
	}

	if file.IsDir() {

		// Walk the directory and load the files from the archives
		// into the map
		logrus.Infoln("Detected multiple archive files")
		err = filepath.Walk(o.ArchivePath, func(path string, info os.FileInfo, err error) error {

			if err != nil {
				return fmt.Errorf("traversing %s: %v", path, err)
			}
			if info == nil {
				return fmt.Errorf("no file info")
			}

			extension := filepath.Ext(path)
			extension = strings.TrimPrefix(extension, ".")

			if extension == a.String() {
				logrus.Debugf("Found archive %s", path)
				return a.Walk(path, func(f archiver.File) error {
					filesinArchive[f.Name()] = path
					return nil
				})
			}

			return nil
		})

	} else {
		// Walk the archive and load the file names into the map
		err = a.Walk(o.ArchivePath, func(f archiver.File) error {
			filesinArchive[f.Name()] = o.ArchivePath
			return nil
		})
	}

	return filesinArchive, err
}
