package mirror

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/openshift/oc/pkg/cli/image/imagesource"
	imagemanifest "github.com/openshift/oc/pkg/cli/image/manifest"
	"github.com/openshift/oc/pkg/cli/image/mirror"
	"github.com/sirupsen/logrus"

	"github.com/openshift/oc-mirror/pkg/bundle"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc-mirror/pkg/image"
)

type AdditionalOptions struct {
	*MirrorOptions
}

func NewAdditionalOptions(mo *MirrorOptions) *AdditionalOptions {
	return &AdditionalOptions{MirrorOptions: mo}
}

type ErrBlocked struct {
	image string
}

func (e ErrBlocked) Error() string {
	return fmt.Sprintf("additional image %s also specified as blocked, remove the image one config field or the other", e.image)
}

// GetAdditional downloads specified images in the imageset-config.yaml under mirror.additonalImages
func (o *AdditionalOptions) GetAdditional(cfg v1alpha1.ImageSetConfiguration, imageList []v1alpha1.AdditionalImages) (assocs image.AssociationSet, err error) {

	opts := mirror.NewMirrorImageOptions(o.IOStreams)
	opts.DryRun = o.DryRun
	opts.SecurityOptions.Insecure = o.SourceSkipTLS
	opts.SecurityOptions.SkipVerification = o.SkipVerification
	opts.FileDir = filepath.Join(o.Dir, config.SourceDir)
	opts.FilterOptions = imagemanifest.FilterOptions{FilterByOS: ".*"}

	logrus.Infof("Downloading %d image(s) to %s", len(imageList), opts.FileDir)

	var mappings []mirror.Mapping
	images := make([]string, len(imageList))
	assocMappings := make(map[string]string, len(imageList))
	for i, img := range imageList {

		regctx, err := config.CreateDefaultContext(o.SourceSkipTLS)
		if err != nil {
			return nil, fmt.Errorf("error creating registry context: %v", err)
		}
		opts.SecurityOptions.CachedContext = regctx
		// Get source image information
		srcRef, err := imagesource.ParseReference(img.Name)
		if err != nil {
			return nil, fmt.Errorf("error parsing source image %s: %v", img.Name, err)
		}

		if setLatest(srcRef) {
			srcRef.Ref.Tag = "latest"
		}

		// Set destination image information
		dstRef := srcRef
		dstRef.Type = imagesource.DestinationFile
		dstRef.Ref = dstRef.Ref.DockerClientDefaults()

		// Check if image is specified as a blocked image
		if bundle.IsBlocked(cfg, srcRef.Ref) {
			return nil, ErrBlocked{img.Name}
		}
		// Create mapping from source and destination images
		mappings = append(mappings, mirror.Mapping{
			Source:      srcRef,
			Destination: dstRef,
			Name:        srcRef.Ref.Name,
		})

		// Add mapping and image for image association.
		// The registry component is not included in the final path.
		srcImage, err := bundle.PinImages(context.TODO(), srcRef.Ref.Exact(), "", o.SourceSkipTLS)
		if err != nil {
			return nil, err
		}

		dstRef.Ref.Registry = ""
		assocMappings[srcImage] = dstRef.String()
		images[i] = srcImage
	}

	opts.Mappings = mappings

	if err := opts.Run(); err != nil {
		return nil, err
	}

	// Do not build associations on dry runs because there are no manifests
	if !o.DryRun {
		assocs, err = image.AssociateImageLayers(opts.FileDir, assocMappings, images, image.TypeGeneric)
		if err != nil {
			return nil, err
		}
	}

	return assocs, nil
}

func setLatest(img imagesource.TypedImageReference) bool {
	return len(img.Ref.ID) == 0 && len(img.Ref.Tag) == 0
}
