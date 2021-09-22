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
	"time"

	"github.com/google/uuid"
	"github.com/mholt/archiver/v3"
	"github.com/opencontainers/go-digest"
	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/library-go/pkg/image/registryclient"
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

const (
	icspSizeLimit = 250000
	icspScope     = "repository"
)

type UuidError struct {
	InUuid   uuid.UUID
	CurrUuid uuid.UUID
}

func (u *UuidError) Error() string {
	return fmt.Sprintf("mismatched uuids, want %v, got %v", u.CurrUuid, u.InUuid)
}

type SequenceError struct {
	wantSeq int
	gotSeq  int
}

func (s *SequenceError) Error() string {
	return fmt.Sprintf("invalid bundle sequence order, want %v, got %v", s.wantSeq, s.gotSeq)
}

type ErrArchiveFileNotFound struct {
	filename string
}

func (e *ErrArchiveFileNotFound) Error() string {
	return fmt.Sprintf("file %s not found in archive", e.filename)
}

func (o *Options) Run(ctx context.Context, cmd *cobra.Command, f kcmdutil.Factory) error {

	logrus.Infof("Publishing image set from archive %q to registry %q", o.ArchivePath, o.ToMirror)

	var currentMeta v1alpha1.Metadata
	var incomingMeta v1alpha1.Metadata
	a := archive.NewArchiver()

	// Validating user path input
	if err := o.ValidatePaths(); err != nil {
		return err
	}

	// Create workspace
	cleanup, tmpdir, err := mktempDir(o.Dir)
	if err != nil {
		return err
	}

	// Handle cleanup of disk
	if !o.SkipCleanup {
		defer cleanup()
	}

	logrus.Debugf("Unarchiving metadata into %s", tmpdir)

	// Get file information from the source archives
	filesInArchive, err := o.readImageSet(a)
	if err != nil {
		return err
	}

	// Extract incoming metadata
	if err := unpack(config.MetadataBasePath, tmpdir, filesInArchive); err != nil {
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

		incomingRun := incomingMeta.PastMirrors[len(incomingMeta.PastMirrors)-1]
		if incomingRun.Sequence != 1 {
			return &SequenceError{1, incomingRun.Sequence}
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
			return &SequenceError{(currRun.Sequence + 1), incomingRun.Sequence}
		}
	}

	if err := o.unpackImageSet(a, o.Dir); err != nil {
		return err
	}

	// Load image associations to find layers not present locally.
	assocs, err := readAssociations(filepath.Join(o.Dir, config.AssociationsBasePath))
	if err != nil {
		return err
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
		errs     []error
		allICSPs []operatorv1alpha1.ImageContentSourcePolicy
	)

	// Create target dir for manifest directory
	manifestDir, err := o.createManifestDir()
	if err != nil {
		return err
	}

	namedICSPMappings := map[string]map[reference.DockerImageReference]reference.DockerImageReference{}
	for _, imageName := range assocs.Keys() {

		genericMappings := []imgmirror.Mapping{}
		releaseMapping := imgmirror.Mapping{}
		// Map of remote layer digest to the set of paths they should be fetched to.
		missingLayers := map[string][]string{}

		// Save original source and mirror destination mapping to generate ICSP for this image.
		imageRef, err := reference.Parse(imageName)
		if err != nil {
			errs = append(errs, fmt.Errorf("error parsing image name %q for ICSP generation: %v", imageName, err))
			continue
		}
		dstRef := imageRef
		dstRef.Registry = toMirrorRef.Ref.Registry
		namedICSPMappings[imageRef.Name] = map[reference.DockerImageReference]reference.DockerImageReference{
			imageRef: dstRef,
		}

		values, _ := assocs.Search(imageName)

		// Create temp workspace for image processing
		_, unpackDir, err := mktempDir(tmpdir)
		if err != nil {
			return err
		}

		for _, assoc := range values {

			manifestPath := filepath.Join("v2", assoc.Path, "manifests")

			// Ensure child manifests are all unpacked
			// TODO: find a way to ensure these will be process on their
			// No longer stored in the map
			logrus.Debugf("reading assoc: %s", assoc.Name)
			if len(assoc.ManifestDigests) != 0 {
				for _, manifestDigest := range assoc.ManifestDigests {
					if hasManifest := assocs.SetContainsKey(imageName, manifestDigest); !hasManifest {
						errs = append(errs, fmt.Errorf("image %q: expected associations to have manifest %s but was not found", imageName, manifestDigest))
						continue
					}
					manifestArchivePath := filepath.Join(manifestPath, manifestDigest)
					switch _, err := os.Stat(manifestArchivePath); {
					case err == nil:
						logrus.Debugf("Manifest found %s found in %s", manifestDigest, assoc.Path)
					case errors.Is(err, os.ErrNotExist):
						if err := unpack(manifestArchivePath, unpackDir, filesInArchive); err != nil {
							errs = append(errs, err)
						}
					default:
						errs = append(errs, fmt.Errorf("accessing image %q manifest %q: %v", imageName, manifestDigest, err))
					}
				}
			}

			// Unpack association main manifest
			if err := unpack(filepath.Join(manifestPath, assoc.ID), unpackDir, filesInArchive); err != nil {
				errs = append(errs, err)
				continue
			}

			for _, layerDigest := range assoc.LayerDigests {
				logrus.Debugf("Found layer %v for image %s", layerDigest, imageName)
				// Construct blob path, which is adjacent to the manifests path.
				blobPath := filepath.Join("blobs", layerDigest)
				imagePath := filepath.Join(unpackDir, "v2", assoc.Path)
				imageBlobPath := filepath.Join(imagePath, blobPath)
				aerr := &ErrArchiveFileNotFound{}
				switch err := unpack(blobPath, imagePath, filesInArchive); {
				case err == nil:
					logrus.Debugf("Blob %s found in %s", layerDigest, assoc.Path)
				case errors.Is(err, os.ErrNotExist) || errors.As(err, &aerr):
					// Image layer must exist in the mirror registry since it wasn't archived,
					// so fetch the layer and place it in the blob dir so it can be mirrored by `oc`.
					missingLayers[layerDigest] = append(missingLayers[layerDigest], imageBlobPath)
				default:
					errs = append(errs, fmt.Errorf("accessing image %q blob %q at %s: %v", imageName, layerDigest, blobPath, err))
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
				if err := unpack(filepath.Join(manifestPath, assoc.TagSymlink), unpackDir, filesInArchive); err != nil {
					errs = append(errs, err)
					continue
				}
				m.Source.Ref.Tag = assoc.TagSymlink
			}
			m.Destination = toMirrorRef
			m.Destination.Ref.Namespace = m.Source.Ref.Namespace
			m.Destination.Ref.Name = m.Source.Ref.Name
			m.Destination.Ref.Tag = m.Source.Ref.Tag
			m.Destination.Ref.ID = m.Source.Ref.ID

			if len(missingLayers) != 0 {
				// Fetch all layers and mount them at the specified paths.
				if err := o.fetchBlobs(ctx, incomingMeta, m, missingLayers); err != nil {
					return err
				}
			}

			switch assoc.Type {
			case image.TypeGeneric:
				genericMappings = append(genericMappings, m)
			case image.TypeOCPRelease:
				m.Destination.Ref.Tag = ""
				m.Destination.Ref.ID = ""
				if strings.Contains(assoc.Name, "ocp-release") {
					releaseMapping = m
				}
			case image.TypeOperatorCatalog:
				// Create a catalog source file for index
				mapping := map[imagesource.TypedImageReference]imagesource.TypedImageReference{m.Source: m.Destination}
				if err := o.writeCatalogSource(manifestDir, mapping); err != nil {
					errs = append(errs, fmt.Errorf("image %q: error writing catalog source: %v", imageName, err))
					continue
				}
				genericMappings = append(genericMappings, m)
			case image.TypeOperatorBundle, image.TypeOperatorRelatedImage:
				genericMappings = append(genericMappings, m)
			case image.TypeInvalid:
				errs = append(errs, fmt.Errorf("image %q: image type is not set", imageName))
			default:
				errs = append(errs, fmt.Errorf("image %q: invalid image type %v", imageName, assoc.Type))
			}
		}

		// Mirror all generic mappings for this image
		if len(genericMappings) != 0 {
			if err := o.mirrorImage(genericMappings, unpackDir); err != nil {
				errs = append(errs, err)
			}
		}

		// If this is a release image mirror the full release
		if releaseMapping.Source.String() != "" {
			if err := o.mirrorRelease(releaseMapping, cmd, f, unpackDir); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) != 0 {
		return utilerrors.NewAggregate(errs)
	}

	logrus.Debug("rebuilding catalog images")

	ctlgRefs, err := o.rebuildCatalogs(ctx, tmpdir, filesInArchive)
	if err != nil {
		return fmt.Errorf("error rebuilding catalog images from file-based catalogs: %v", err)
	}
	// Create CatalogSource manifests and save ICSP data for all catalog refs.
	// Source and dest refs are treated the same intentionally since
	// the source image does not exist and destination image was built above.
	for _, ref := range ctlgRefs {
		namedICSPMappings[ref.Ref.Name] = map[reference.DockerImageReference]reference.DockerImageReference{ref.Ref: ref.Ref}

		if err := writeCatalogSource(ref, ref, manifestDir); err != nil {
			return fmt.Errorf("error writing CatalogSource for catalog image %q: %v", ref.Ref.Exact(), err)
		}
	}

	// Generate ICSPs for all images.
	for imageName, icspMapping := range namedICSPMappings {
		icsps, err := GenerateICSPs(imageName, icspSizeLimit, icspScope, icspMapping)
		if err != nil {
			return fmt.Errorf("error generating ICSP for image name %q: %v", imageName, err)
		}
		allICSPs = append(allICSPs, icsps...)
	}

	// Write an aggregation of ICSPs
	if err := WriteICSPs(manifestDir, allICSPs); err != nil {
		return fmt.Errorf("error writing ICSPs: %v", err)
	}

	// Install catalogsource and icsp
	logrus.Info("CatalogSource and ICSP install not implemented")

	// Replace old metadata with new metadata
	if err := backend.WriteMetadata(ctx, &incomingMeta, config.MetadataBasePath); err != nil {
		return err
	}

	return nil
}

// readAssociations will process and return data from the image associations file
func readAssociations(assocPath string) (assocs image.AssociationSet, err error) {
	f, err := os.Open(assocPath)
	if err != nil {
		return assocs, fmt.Errorf("error opening image associations file: %v", err)
	}
	defer f.Close()

	return assocs, assocs.Decode(f)
}

// unpackImageSet unarchives all provided tar archives
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
				if err := archive.Unarchive(a, path, dest, []string{"blobs", "v2"}); err != nil {
					return err
				}
			}

			return nil
		})

	} else {

		logrus.Infof("Extracting archive %s", o.ArchivePath)
		if err := archive.Unarchive(a, o.ArchivePath, dest, []string{"blobs"}); err != nil {
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
	// Allowing exisitng files to be written to for now since we
	// some blobs appears to be written multiple time
	// TODO: investigate this issue
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("error creating blob file: %v", err)
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("error copying blob %q: %v", filepath.Base(dstPath), err)
	}
	return nil
}

func (o *Options) fetchBlobs(ctx context.Context, meta v1alpha1.Metadata, mapping imgmirror.Mapping, missingLayers map[string][]string) error {
	catalogNamespaceNames := []string{}
	dstRef := mapping.Destination.Ref
	catalogNamespaceNames = append(catalogNamespaceNames, path.Join(dstRef.Namespace, dstRef.Name))

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

func unpack(archiveFilePath, dest string, filesInArchive map[string]string) error {

	name := filepath.Base(archiveFilePath)
	archivePath, found := filesInArchive[name]
	if !found {
		return &ErrArchiveFileNotFound{name}
	}

	if err := archive.NewArchiver().Extract(archivePath, archiveFilePath, dest); err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(dest, archiveFilePath)); err != nil {
		return err
	}

	return nil
}

func mktempDir(dir string) (func(), string, error) {
	dir, err := ioutil.TempDir(dir, "images.*")
	return func() {
		if err := os.RemoveAll(dir); err != nil {
			logrus.Fatal(err)
		}
	}, dir, err
}

// mirrorRelease uses the `oc release mirror` library to mirror OCP release
func (o *Options) mirrorRelease(mapping imgmirror.Mapping, cmd *cobra.Command, f kcmdutil.Factory, fromDir string) error {
	logrus.Debugf("mirroring release image: %s", mapping.Source.String())
	relOpts := release.NewMirrorOptions(o.IOStreams)
	relOpts.From = mapping.Source.String()
	relOpts.FromDir = fromDir
	relOpts.To = mapping.Destination.String()
	relOpts.SecurityOptions.Insecure = o.SkipTLS
	relOpts.DryRun = o.DryRun
	if err := relOpts.Complete(cmd, f, nil); err != nil {
		return fmt.Errorf("error initializing release mirror options: %v", err)
	}
	if err := relOpts.Validate(); err != nil {
		return fmt.Errorf("invalid release mirror options: %v", err)
	}
	if err := relOpts.Run(); err != nil {
		return fmt.Errorf("error running %q release mirror: %v", mapping, err)
	}

	return nil
}

// mirrorImages uses the `oc mirror` library to mirror generic images
func (o *Options) mirrorImage(mappings []imgmirror.Mapping, fromDir string) error {
	// Mirror all file sources of each available image type to mirror registry.
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		var srcs []string
		for _, m := range mappings {
			srcs = append(srcs, m.Source.String())
		}
		logrus.Debugf("mirroring generic images: %q", srcs)
	}
	genOpts := imgmirror.NewMirrorImageOptions(o.IOStreams)
	genOpts.Mappings = mappings
	genOpts.DryRun = o.DryRun
	genOpts.FromFileDir = fromDir
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

	return nil
}

// writeCatalogSource will write a CatalogSource for catalog index
func (o *Options) writeCatalogSource(manifestDir string, mapping map[imagesource.TypedImageReference]imagesource.TypedImageReference) error {
	errs := []error{}
	for source, dest := range mapping {
		images := make(map[string]struct{})
		images[dest.String()] = struct{}{}
		mapping, mapErrs := mappingForImages(images, source, dest, 2)
		if len(mapErrs) > 0 {
			errs = append(errs, mapErrs...)
			continue
		}

		mappedIndex, err := mount(source, dest, 2)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		mapping[source] = mappedIndex

		if err := WriteCatalogSource(source, manifestDir, mapping); err != nil {
			errs = append(errs, err)
		}
	}
	return utilerrors.NewAggregate(errs)
}

func (o *Options) createManifestDir() (manifestDir string, err error) {
	manifestDir = filepath.Join(
		o.Dir,
		fmt.Sprintf("manifest-%v", time.Now().Unix()),
	)

	if err := os.MkdirAll(manifestDir, os.ModePerm); err != nil {
		return manifestDir, err
	}

	return manifestDir, nil
}
