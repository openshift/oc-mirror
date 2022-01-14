package mirror

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

	"github.com/openshift/oc-mirror/pkg/archive"
	"github.com/openshift/oc-mirror/pkg/bundle"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
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

func (o *MirrorOptions) Publish(ctx context.Context, cmd *cobra.Command, f kcmdutil.Factory) error {

	logrus.Infof("Publishing image set from archive %q to registry %q", o.From, o.ToMirror)

	var currentMeta v1alpha1.Metadata
	var incomingMeta v1alpha1.Metadata
	a := archive.NewArchiver()

	// Set target dir for resulting artifacts
	if o.OutputDir == "" {
		dir, err := o.createResultsDir()
		o.OutputDir = dir
		if err != nil {
			return err
		}
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
	filesInArchive, err := bundle.ReadImageSet(a, o.From)
	if err != nil {
		return err
	}

	// Extract imageset
	if err := o.unpackImageSet(a, tmpdir); err != nil {
		return err
	}

	// Create a local workspace backend for incoming data
	workspace, err := storage.NewLocalBackend(tmpdir)
	if err != nil {
		return fmt.Errorf("error opening local backend: %v", err)
	}
	// Load incoming metadta
	if err := workspace.ReadMetadata(ctx, &incomingMeta, config.MetadataBasePath); err != nil {
		return fmt.Errorf("error reading incoming metadata: %v", err)
	}

	var metaImage string
	if len(o.UserNamespace) != 0 {
		metaImage = fmt.Sprintf("%s/%s/oc-mirror:%s", o.ToMirror, o.UserNamespace, incomingMeta.Uid)
	} else {
		metaImage = fmt.Sprintf("%s/oc-mirror:%s", o.ToMirror, incomingMeta.Uid)
	}

	// Determine stateless or stateful mode
	var backend storage.Backend
	if incomingMeta.SingleUse {
		logrus.Warn("metadata has single-use label, using stateless mode")
		cfg := v1alpha1.StorageConfig{
			Local: &v1alpha1.LocalConfig{
				Path: o.Dir,
			},
		}
		backend, err = storage.ByConfig(o.Dir, cfg)
		if err != nil {
			return err
		}
		defer func() {
			if err := backend.Cleanup(ctx, config.MetadataBasePath); err != nil {
				logrus.Error(err)
			}
		}()
	} else {
		cfg := v1alpha1.StorageConfig{
			Registry: &v1alpha1.RegistryConfig{
				ImageURL: metaImage,
				SkipTLS:  o.DestSkipTLS,
			},
		}
		backend, err = storage.ByConfig(o.Dir, cfg)
		if err != nil {
			return err
		}
	}

	// Read in current metadata, if present
	switch err := backend.ReadMetadata(ctx, &currentMeta, config.MetadataBasePath); {
	case err != nil && !errors.Is(err, storage.ErrMetadataNotExist):
		return err
	case err != nil:
		logrus.Infof("No existing metadata found. Setting up new workspace")
		// Check that this is the first imageset
		incomingRun := incomingMeta.PastMirrors[len(incomingMeta.PastMirrors)-1]
		if incomingRun.Sequence != 1 {
			return &SequenceError{1, incomingRun.Sequence}
		}
	default:
		// UUID check removed because the image is stored as
		// UUID name. A mismatch will now be seen as a new workspace.
		// TODO: Add a way show the user what UUID images existing in the registry?

		// Complete metadata checks
		logrus.Debug("Check metadata sequence number")
		currRun := currentMeta.PastMirrors[len(currentMeta.PastMirrors)-1]
		incomingRun := incomingMeta.PastMirrors[len(incomingMeta.PastMirrors)-1]
		if incomingRun.Sequence != (currRun.Sequence + 1) {
			return &SequenceError{currRun.Sequence + 1, incomingRun.Sequence}
		}
	}

	// Unpack chart to user destination if it exists
	logrus.Debugf("Unpacking any provided Helm charts to %s", o.OutputDir)
	if err := unpack(config.HelmDir, o.OutputDir, filesInArchive); err != nil {
		return err
	}

	// Load image associations to find layers not present locally.
	assocs, err := readAssociations(filepath.Join(tmpdir, config.AssociationsBasePath))
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

	namedICSPData := map[string]ICSPGenerator{}
	for _, imageName := range assocs.Keys() {

		genericMappings := []imgmirror.Mapping{}
		releaseMapping := imgmirror.Mapping{}

		// Save original source and mirror destination mapping to generate ICSP for this image.
		imageRef, err := reference.Parse(imageName)
		if err != nil {
			errs = append(errs, fmt.Errorf("error parsing image name %q for ICSP generation: %v", imageName, err))
			continue
		}
		dstRef := imageRef
		dstRef.Registry = toMirrorRef.Ref.Registry
		dstRef.Namespace = path.Join(o.UserNamespace, dstRef.Namespace)

		icspData := ICSPGenerator{
			ICSPMapping: map[reference.DockerImageReference]reference.DockerImageReference{
				imageRef: dstRef,
			},
			ImageName: imageRef.Name,
		}

		values, _ := assocs.Search(imageName)

		// Create temp workspace for image processing
		cleanUnpackDir, unpackDir, err := mktempDir(tmpdir)
		if err != nil {
			return err
		}

		for _, assoc := range values {

			// Map of remote layer digest to the set of paths they should be fetched to.
			missingLayers := map[string][]string{}
			manifestPath := filepath.Join("v2", assoc.Path, "manifests")

			// Ensure child manifests are all unpacked
			logrus.Debugf("reading assoc: %s", assoc.Name)
			if len(assoc.ManifestDigests) != 0 {
				for _, manifestDigest := range assoc.ManifestDigests {
					if hasManifest := assocs.ContainsKey(imageName, manifestDigest); !hasManifest {
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
			m.Destination.Ref.Name = m.Source.Ref.Name
			m.Destination.Ref.Tag = m.Source.Ref.Tag
			m.Destination.Ref.ID = m.Source.Ref.ID
			m.Destination.Ref.Namespace = path.Join(o.UserNamespace, m.Source.Ref.Namespace)

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
				genericMappings = append(genericMappings, m)
				icspData.AddOperatorLabel = true
			case image.TypeOperatorBundle, image.TypeOperatorRelatedImage:
				genericMappings = append(genericMappings, m)
				icspData.AddOperatorLabel = true
			case image.TypeInvalid:
				errs = append(errs, fmt.Errorf("image %q: image type is not set", imageName))
			default:
				errs = append(errs, fmt.Errorf("image %q: invalid image type %v", imageName, assoc.Type))
			}

			if len(missingLayers) != 0 {
				// Fetch all layers and mount them at the specified paths.
				if err := o.fetchBlobs(ctx, currentMeta, m, missingLayers); err != nil {
					return err
				}
			}
		}

		// Add mapping for ICSP generation
		namedICSPData[imageRef.Name] = icspData

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

		// Cleanup temp image processing workspace as images are processed
		if !o.SkipCleanup {
			cleanUnpackDir()
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
	for sourceRef, destRef := range ctlgRefs {
		icspData := ICSPGenerator{
			ICSPMapping:      map[reference.DockerImageReference]reference.DockerImageReference{sourceRef: destRef},
			ImageName:        sourceRef.Name,
			AddOperatorLabel: true,
		}
		namedICSPData[sourceRef.Name] = icspData

		if err := WriteCatalogSource(sourceRef, destRef, o.OutputDir); err != nil {
			return fmt.Errorf("error writing CatalogSource for catalog image %q: %v", destRef.Exact(), err)
		}
	}

	// Generate ICSPs for all images.
	for name, data := range namedICSPData {
		logrus.Debugf("Generating ICSP for image %q", name)
		icsps, err := data.Run(icspScope, icspSizeLimit)
		if err != nil {
			return fmt.Errorf("error generating ICSP for image name %q: %v", name, err)
		}
		allICSPs = append(allICSPs, icsps...)
	}

	// Write an aggregation of ICSPs
	if err := WriteICSPs(o.OutputDir, allICSPs); err != nil {
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

// unpackImageSet unarchives all provided tar archives	if err != nil {
func (o *MirrorOptions) unpackImageSet(a archive.Archiver, dest string) error {

	// archive that we do not want to unpack
	exclude := []string{"blobs", "v2", config.HelmDir}

	file, err := os.Stat(o.From)
	if err != nil {
		return err
	}

	if file.IsDir() {

		err = filepath.Walk(o.From, func(path string, info os.FileInfo, err error) error {

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
				if err := archive.Unarchive(a, path, dest, exclude); err != nil {
					return err
				}
			}

			return nil
		})

	} else {

		logrus.Infof("Extracting archive %s", o.From)
		if err := archive.Unarchive(a, o.From, dest, exclude); err != nil {
			return err
		}
	}

	return err
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

func (o *MirrorOptions) fetchBlobs(ctx context.Context, meta v1alpha1.Metadata, mapping imgmirror.Mapping, missingLayers map[string][]string) error {

	restctx, err := config.CreateDefaultContext(o.DestSkipTLS)
	if err != nil {
		return err
	}

	var errs []error
	for layerDigest, dstBlobPaths := range missingLayers {
		imgRef, err := o.findBlobRepo(meta, layerDigest)
		if err != nil {
			errs = append(errs, fmt.Errorf("error finding remote layer %q: %v", layerDigest, err))
		}
		if err := o.fetchBlob(ctx, restctx, imgRef.Ref, layerDigest, dstBlobPaths); err != nil {
			errs = append(errs, fmt.Errorf("layer %s: %v", layerDigest, err))
			continue
		}
	}

	return utilerrors.NewAggregate(errs)
}

// fetchBlob fetches a blob at <o.ToMirror>/<resource>/blobs/<layerDigest>
// then copies it to each path in dstPaths.
func (o *MirrorOptions) fetchBlob(ctx context.Context, restctx *registryclient.Context, ref reference.DockerImageReference, layerDigest string, dstPaths []string) error {

	logrus.Debugf("copying blob %s from %s", layerDigest, ref.Exact())
	repo, err := restctx.RepositoryForRef(ctx, ref, o.DestSkipTLS)
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
			return fmt.Errorf("copy blob for %s: %v", ref, err)
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
// QUESTION(jpower): should we just mirror release one by one
// The namespace is not the same as the image name
func (o *MirrorOptions) mirrorRelease(mapping imgmirror.Mapping, cmd *cobra.Command, f kcmdutil.Factory, fromDir string) error {
	logrus.Debugf("mirroring release image: %s", mapping.Source.String())
	relOpts := release.NewMirrorOptions(o.IOStreams)
	relOpts.From = mapping.Source.String()
	relOpts.FromDir = fromDir
	relOpts.To = mapping.Destination.String()
	relOpts.SecurityOptions.Insecure = o.DestSkipTLS
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
func (o *MirrorOptions) mirrorImage(mappings []imgmirror.Mapping, fromDir string) error {
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
	// Filter must be a wildcard for publishing because we
	// cannot filter images within a catalog
	genOpts.FilterOptions = imagemanifest.FilterOptions{FilterByOS: ".*"}
	genOpts.SkipMultipleScopes = true
	genOpts.KeepManifestList = true
	genOpts.SecurityOptions.Insecure = o.DestSkipTLS
	if err := genOpts.Validate(); err != nil {
		return fmt.Errorf("invalid image mirror options: %v", err)
	}
	if err := genOpts.Run(); err != nil {
		return fmt.Errorf("error running generic image mirror: %v", err)
	}

	return nil
}

func (o *MirrorOptions) createResultsDir() (resultsDir string, err error) {
	resultsDir = filepath.Join(
		o.Dir,
		fmt.Sprintf("results-%v", time.Now().Unix()),
	)
	if err := os.MkdirAll(resultsDir, os.ModePerm); err != nil {
		return resultsDir, err
	}
	return resultsDir, nil
}

func (o *MirrorOptions) findBlobRepo(meta v1alpha1.Metadata, layerDigest string) (imagesource.TypedImageReference, error) {
	var namespacename string
	var ref string
	for _, mirror := range meta.PastMirrors {
		for _, blob := range mirror.Blobs {
			if blob.ID == layerDigest {
				namespacename = blob.NamespaceName
				break
			}
		}
	}
	if len(o.UserNamespace) != 0 {
		ref = fmt.Sprintf("%s/%s/%s", o.ToMirror, o.UserNamespace, namespacename)
	} else {
		ref = fmt.Sprintf("%s/%s", o.ToMirror, namespacename)
	}
	return imagesource.ParseReference(ref)
}
