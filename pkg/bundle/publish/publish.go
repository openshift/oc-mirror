package publish

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openshift/oc/pkg/cli/image/mirror"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/RedHatGov/bundle/pkg/archive"
	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/image"
)

// mirror to registry
func mirrorToReg(rootDir string) {

	// TODO: implement non-local layer pulling, image reconstitution, and pushing to mirror.

	in, out, errout := os.Stdin, os.Stdout, os.Stderr

	iostreams := genericclioptions.IOStreams{In: in, Out: out, ErrOut: errout}

	imageOpts := mirror.NewMirrorImageOptions(iostreams)
	// oc image mirror -a dba-ps.json --from-dir=release/ "file://openshift/release:4.7.3*" registry.dbasparta.io:5020/openshift --insecure
	imageOpts.FromFileDir = rootDir
	//	imageOpts.Out = PublishOpts.ToMirror
	// imageOpts.Filenames = "file://openshift/release:4.7.3*"
	logrus.Info("Dry Run: ", imageOpts.DryRun)
	logrus.Info("From File Dir: ", imageOpts.FromFileDir)

}

func Publish(ctx context.Context, rootDir, archivePath, toMirror string) error {

	logrus.Infof("Publish image set from archive %q to registry %q", archivePath, toMirror)

	// Load tar archive made by `create`.
	if err := archive.NewArchiver().Unarchive(archivePath, rootDir); err != nil {
		return err
	}

	// Load image associations to find layers not present locally.
	assocs, err := readAssociations(rootDir)
	if err != nil {
		return err
	}

	// For each image association with layers, pull any manifest layer digests needed
	// to reconstitute mirrored images and push those images.
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
			for _, manifestDigest := range assoc.ManifestDigests {
				if _, hasManifest := assocs[manifestDigest]; !hasManifest {
					errs = append(errs, fmt.Errorf("image %q: expected associations to have manifest %s but was not found", imageName, manifestDigest))
				}
			}
			continue
		}

		for _, layerDigest := range assoc.LayerDigests {
			// Construct blob path, which is adjacent to the manifests path.
			// If a layer exists in the archive (err == nil), nothing needs to be done
			// since the layer is already in the expected location.
			blobPath := filepath.Join(assoc.Path, "blobs", layerDigest)
			if _, err := os.Stat(blobPath); err != nil && errors.Is(err, os.ErrNotExist) {
				// Image layer must exist in the mirror registry since it wasn't archived,
				// so GET the layer and place it in the blob dir so it can be mirrored by `oc`.

				// TODO: implement layer pulling.

			} else if err != nil {
				errs = append(errs, fmt.Errorf("access image %q blob %q at %s: %v", imageName, layerDigest, blobPath, err))
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

	// mirror to registry
	logrus.Info("mirroring not implemented")
	mirrorToReg(rootDir)

	// install imagecontentsourcepolicy
	logrus.Info("ICSP creation not implemented")

	// install catalogsource
	logrus.Info("CatalogSource creation not implemented")

	return nil
}

func readAssociations(rootDir string) (assocs image.Associations, err error) {
	assocPath := filepath.Join(rootDir, config.InternalDir, config.AssociationsFile)
	f, err := os.Open(assocPath)
	if err != nil {
		return assocs, fmt.Errorf("error opening image associations file: %v", err)
	}
	defer f.Close()

	return assocs, assocs.Decode(f)
}
