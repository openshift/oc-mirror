package publish

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/mholt/archiver/v3"
	"github.com/opencontainers/go-digest"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/library-go/pkg/image/registryclient"
	"github.com/openshift/oc/pkg/cli/admin/catalog"
	"github.com/openshift/oc/pkg/cli/admin/release"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	imagemanifest "github.com/openshift/oc/pkg/cli/image/manifest"
	imgmirror "github.com/openshift/oc/pkg/cli/image/mirror"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"

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

func (o *Options) Run(ctx context.Context, cmd *cobra.Command, f kcmdutil.Factory) error {

	logrus.Infof("Publishing image set from archive %q to registry %q", o.ArchivePath, o.ToMirror)

	var currentMeta v1alpha1.Metadata
	var incomingMeta v1alpha1.Metadata
	a := archive.NewArchiver()

	// Create workspace
	tmpdir, err := ioutil.TempDir(o.Dir, "imageset")
	if err != nil {
		return err
	}
	if !o.SkipCleanup {
		defer os.RemoveAll(tmpdir)
	}

	logrus.Debugf("Using temporary directory %s to unarchive metadata", tmpdir)

	// Get file information from the source archives
	filesInArchive, err := o.readImageSet(a)
	if err != nil {
		return err
	}

	// Extract incoming metadata
	archive, ok := filesInArchive[config.MetadataFile]
	if !ok {
		return errors.New("metadata is not in archive")
	}

	logrus.Debug("Extracting incoming metadta")
	if err := a.Extract(archive, config.MetadataBasePath, tmpdir); err != nil {
		return err
	}

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
	existingMeta := filepath.Join(o.Dir, config.MetadataBasePath)
	if _, err := os.Stat(existingMeta); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		logrus.Infof("No existing metadata found. Setting up new workspace")

		// Find first file and load metadata from that
		if err := workspace.ReadMetadata(ctx, &incomingMeta, config.MetadataBasePath); err != nil {
			return fmt.Errorf("error reading incoming metadata: %v", err)
		}

	} else {

		// Compare metadata UID and sequence number
		if err := backend.ReadMetadata(ctx, &currentMeta, config.MetadataBasePath); err != nil {
			return fmt.Errorf("error reading current metadata: %v", err)
		}

		if err := workspace.ReadMetadata(ctx, &incomingMeta, config.MetadataBasePath); err != nil {
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
	if err := o.unpackImageSet(a, o.Dir); err != nil {
		return err
	}

	// Load image associations to find layers not present locally.
	assocPath := filepath.Join(o.Dir, config.AssociationsBasePath)
	assocs, err := readAssociations(assocPath)
	if err != nil {
		return fmt.Errorf("error reading associations from %s: %v", o.Dir, err)
	}

	toMirrorRef, err := imagesource.ParseReference(o.ToMirror)
	if err != nil {
		return fmt.Errorf("error parsing mirror registry %q: %v", o.ToMirror, err)
	}
	logrus.Debugf("mirror reference: %#v", toMirrorRef)
	if toMirrorRef.Type != imagesource.DestinationRegistry {
		return fmt.Errorf("destination %q must be a registry reference", o.ToMirror)
	}

	var (
		errs []error
		// Mappings for mirroring image types.
		genericMappings []imgmirror.Mapping
		releaseMappings []imgmirror.Mapping
		catalogMappings []imgmirror.Mapping
		// Map of remote layer digest to the set of paths they should be fetched to.
		missingLayers = map[string][]string{}
	)
	for imageName, assoc := range assocs {

		assoc := assoc
		logrus.Debugf("reading assoc: %s", assoc.Name)

		// All manifest layers will be pulled below if associated,
		// so just sanity-check that the layers are referenced in some association.
		if len(assoc.ManifestDigests) != 0 {
			for _, manifestDigest := range assoc.ManifestDigests {
				if _, hasManifest := assocs[manifestDigest]; !hasManifest {
					errs = append(errs, fmt.Errorf("image %q: expected associations to have manifest %s but was not found", imageName, manifestDigest))
				}
			}
		}

		for _, layerDigest := range assoc.LayerDigests {
			logrus.Debugf("Found layer %v for image %s", layerDigest, imageName)
			// Construct blob path, which is adjacent to the manifests path.
			imageBlobPath := filepath.Join(o.Dir, "v2", assoc.Path, "blobs", layerDigest)
			blobPath := filepath.Join(o.Dir, "blobs", layerDigest)
			switch _, err := os.Stat(blobPath); {
			case err == nil:
				// If a layer exists in the archive, simply copy it to the blob path
				// adjacent to its parent manifest.
				if src, err := os.Open(blobPath); err == nil {
					err = copyBlobFile(src, imageBlobPath)
					if err := src.Close(); err != nil {
						logrus.Error(err)
					}
				} else {
					err = fmt.Errorf("error opening existing blob file: %v", err)
				}
			case errors.Is(err, os.ErrNotExist):
				// Image layer must exist in the mirror registry since it wasn't archived,
				// so fetch the layer and place it in the blob dir so it can be mirrored by `oc`.
				missingLayers[layerDigest] = append(missingLayers[layerDigest], imageBlobPath)
			default:
				err = fmt.Errorf("accessing image %q blob %q at %s: %v", imageName, layerDigest, blobPath, err)
			}
			if err != nil {
				errs = append(errs, err)
			}
		}

		m := imgmirror.Mapping{Name: assoc.Name}
		if m.Source, err = imagesource.ParseReference("file://" + assoc.Path); err != nil {
			errs = append(errs, fmt.Errorf("error parsing source ref %q: %v", assoc.Path, err))
			continue
		}
		// The mirrorer is not a fan of accepting an image ID when a tag symlink is available
		// for some reason.
		// TODO(estroz): investigate the cause of this behavior.
		if assoc.TagSymlink == "" {
			m.Source.Ref.ID = assoc.ID
		} else {
			m.Source.Ref.Tag = assoc.TagSymlink
		}
		m.Destination = toMirrorRef
		m.Destination.Ref.Namespace = m.Source.Ref.Namespace
		m.Destination.Ref.Name = m.Source.Ref.Name
		m.Destination.Ref.Tag = m.Source.Ref.Tag
		m.Destination.Ref.ID = m.Source.Ref.ID

		switch assoc.Type {
		case image.TypeGeneric:
			genericMappings = append(genericMappings, m)
		case image.TypeOCPRelease:
			releaseMappings = append(releaseMappings, m)
		case image.TypeOperatorCatalog:
			catalogMappings = append(catalogMappings, m)
		case image.TypeOperatorBundle, image.TypeOperatorRelatedImage:
			// Let the `catalog mirror` API call mirror all bundle and related images in the catalog.
			// TODO(estroz): this may be incorrect if bundle and related images not in a catalog can be archived,
			// ex. as an additional image. Can probably get around this by mirroring
			// images of this type not mapped by preceding `catalog mirror` calls.
		case image.TypeInvalid:
			errs = append(errs, fmt.Errorf("image %q: image type is not set", imageName))
		default:
			errs = append(errs, fmt.Errorf("image %q: invalid image type %v", imageName, assoc.Type))
		}
	}
	if len(errs) != 0 {
		return utilerrors.NewAggregate(errs)
	}

	if len(missingLayers) != 0 {
		// Fetch all layers and mount them at the specified paths.
		if err := o.fetchBlobs(ctx, incomingMeta, catalogMappings, missingLayers); err != nil {
			return err
		}
	}

	// Now that all layers have been pulled, symlink all tagged manifests to their digest files.
	for _, assoc := range assocs {
		if assoc.TagSymlink == "" {
			continue
		}
		manifestsPath := filepath.Join(o.Dir, "v2", assoc.Path, "manifests")
		srcPath := filepath.Join(manifestsPath, assoc.ID)
		dstPath := filepath.Join(manifestsPath, assoc.TagSymlink)
		if _, err := os.Stat(dstPath); err == nil || errors.Is(err, os.ErrExist) {
			logrus.Debugf("image %s: tag %s symlink for manifest %s already exists", assoc.Name, assoc.TagSymlink, assoc.ID)
			continue
		}
		if err := os.Symlink(srcPath, dstPath); err != nil {
			errs = append(errs, fmt.Errorf("error symlinking manifest digest %q to tag %q: %v", assoc.ID, assoc.TagSymlink, err))
		}
	}
	if len(errs) != 0 {
		return utilerrors.NewAggregate(errs)
	}

	// import imagecontentsourcepolicy
	logrus.Info("ICSP importing not implemented")

	// import catalogsource
	logrus.Info("CatalogSource importing not implemented")

	// Mirror all file sources of each available image type to mirror registry.
	if len(genericMappings) != 0 {
		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			var srcs []string
			for _, m := range genericMappings {
				srcs = append(srcs, m.Source.String())
			}
			logrus.Debugf("mirroring generic images: %q", srcs)
		}
		genOpts := imgmirror.NewMirrorImageOptions(o.IOStreams)
		genOpts.Mappings = genericMappings
		genOpts.DryRun = o.DryRun
		genOpts.FromFileDir = o.Dir
		genOpts.FilterOptions = imagemanifest.FilterOptions{FilterByOS: ".*"}
		genOpts.SkipMultipleScopes = true
		genOpts.KeepManifestList = true
		genOpts.SecurityOptions.Insecure = o.SkipTLS
		if err := genOpts.Validate(); err != nil {
			return fmt.Errorf("invalid image mirror options: %v", err)
		}
		if err := genOpts.Run(); err != nil {
			return fmt.Errorf("error running generic image mirror: %v", err)
		}
	}

	for _, m := range releaseMappings {
		logrus.Debugf("mirroring release image: %s", m.Source)
		relOpts := release.NewMirrorOptions(o.IOStreams)
		relOpts.From = m.Source.String()
		relOpts.To = m.Destination.String()
		if err := relOpts.Complete(cmd, f, nil); err != nil {
			return fmt.Errorf("error initializing release mirror options: %v", err)
		}
		if err := relOpts.Validate(); err != nil {
			return fmt.Errorf("invalid release mirror options: %v", err)
		}
		if err := relOpts.Run(); err != nil {
			return fmt.Errorf("error running %q release mirror: %v", m, err)
		}
	}

	// Change to the working dir since catalog mirroring does not respect
	// FileDir in the "expected" manner (unclear why).
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(o.Dir); err != nil {
		return err
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			logrus.Error(err)
		}
	}()
	for _, m := range catalogMappings {
		logrus.Debugf("mirroring catalog image: %s", m.Source)

		catOpts := catalog.NewMirrorCatalogOptions(o.IOStreams)
		catOpts.DryRun = o.DryRun
		catOpts.MaxPathComponents = 2
		catOpts.SecurityOptions.Insecure = o.SkipTLS
		catOpts.FilterOptions = imagemanifest.FilterOptions{FilterByOS: ".*"}

		args := []string{
			m.Source.String(),
			o.ToMirror,
		}
		if err := catOpts.Complete(&cobra.Command{}, args); err != nil {
			return fmt.Errorf("error constructing catalog options: %v", err)
		}
		if err := catOpts.Validate(); err != nil {
			return fmt.Errorf("invalid catalog mirror options: %v", err)
		}
		if err := catOpts.Run(); err != nil {
			return fmt.Errorf("error mirroring catalog: %v", err)
		}
	}
	if err := os.Chdir(wd); err != nil {
		return err
	}

	// install imagecontentsourcepolicy
	logrus.Info("ICSP creation not implemented")

	// install catalogsource
	logrus.Info("CatalogSource creation not implemented")

	// Replace old metadata with new metadata
	if err := backend.WriteMetadata(ctx, &incomingMeta, config.MetadataBasePath); err != nil {
		return err
	}

	return nil
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
func (o *Options) unpackImageSet(a archive.Archiver, dest string) error {

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

// TODO(estroz): symlink blobs instead of copying them to avoid data duplication.
// `oc` mirror libs should be able to follow these symlinks.
func copyBlobFile(src io.Reader, dstPath string) error {
	logrus.Debugf("copying blob to %s", dstPath)
	if err := os.MkdirAll(filepath.Dir(dstPath), os.ModePerm); err != nil {
		return err
	}
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("error creating blob file: %v", err)
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("error copying blob %q: %v", filepath.Base(dstPath), err)
	}
	return nil
}

func (o *Options) fetchBlobs(ctx context.Context, meta v1alpha1.Metadata, mappings []imgmirror.Mapping, missingLayers map[string][]string) error {
	catalogNamespaceNames := []string{}
	for _, m := range mappings {
		dstRef := m.Destination.Ref
		catalogNamespaceNames = append(catalogNamespaceNames, path.Join(dstRef.Namespace, dstRef.Name))
	}

	blobResources := map[string]string{}
	for _, blob := range meta.PastBlobs {
		resource := blob.NamespaceName
		for _, nsName := range catalogNamespaceNames {
			if nsName == resource {
				// Blob is associated with the catalog image itself.
				blobResources[blob.ID] = nsName
				continue
			}
			suffix := strings.TrimPrefix(resource, nsName+"/")
			if suffix == resource {
				// Blob is not a child of the catalog image in nsName.
				continue
			}
			// Blob may belong to multiple images.
			if _, seenBlob := blobResources[blob.ID]; !seenBlob {
				blobResources[blob.ID] = suffix
				continue
			}
		}
	}

	restctx, err := config.CreateContext(nil, false, o.SkipTLS)
	if err != nil {
		return err
	}

	var errs []error
	for layerDigest, dstBlobPaths := range missingLayers {
		resource, hasResource := blobResources[layerDigest]
		if !hasResource {
			errs = append(errs, fmt.Errorf("layer %s: no registry resource path found", layerDigest))
			continue
		}
		if err := o.fetchBlob(ctx, restctx, resource, layerDigest, dstBlobPaths); err != nil {
			errs = append(errs, fmt.Errorf("layer %s: %v", layerDigest, err))
			continue
		}
	}

	return utilerrors.NewAggregate(errs)
}

// fetchBlob fetches a blob at <o.ToMirror>/<resource>/blobs/<layerDigest>
// then copies it to each path in dstPaths.
func (o *Options) fetchBlob(ctx context.Context, restctx *registryclient.Context, resource, layerDigest string, dstPaths []string) error {

	refStr := path.Join(o.ToMirror, resource)
	ref, err := reference.Parse(refStr)
	if err != nil {
		return fmt.Errorf("parse ref %s: %v", refStr, err)
	}

	logrus.Debugf("copying blob %s from %s", layerDigest, ref.Exact())

	repo, err := restctx.RepositoryForRef(ctx, ref, o.SkipTLS)
	if err != nil {
		return fmt.Errorf("create repo for %s: %v", ref, err)
	}
	dgst, err := digest.Parse(layerDigest)
	if err != nil {
		return err
	}
	rc, err := repo.Blobs(ctx).Open(ctx, dgst)
	if err != nil {
		return fmt.Errorf("open blob: %v", err)
	}
	defer rc.Close()
	for _, dstPath := range dstPaths {
		if err := copyBlobFile(rc, dstPath); err != nil {
			return fmt.Errorf("copy blob: %v", err)
		}
		if _, err := rc.Seek(0, 0); err != nil {
			return fmt.Errorf("seek to start of blob: %v", err)
		}
	}

	return nil
}
