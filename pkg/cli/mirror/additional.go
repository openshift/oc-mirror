package mirror

import (
	"context"
	"fmt"

	"github.com/openshift/oc/pkg/cli/image/imagesource"

	"github.com/openshift/oc-mirror/pkg/bundle"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
)

type AdditionalOptions struct {
	*MirrorOptions
}

func NewAdditionalOptions(mo *MirrorOptions) *AdditionalOptions {
	opts := &AdditionalOptions{MirrorOptions: mo}
	return opts
}

// Plan provides an image mapping with source and destination for provided AdditionalImages
func (o *AdditionalOptions) Plan(ctx context.Context, imageList []v1alpha2.AdditionalImages) (image.TypedImageMapping, error) {
	mmappings := make(image.TypedImageMapping, len(imageList))
	for _, img := range imageList {
		// Get source image information
		srcRef, err := imagesource.ParseReference(img.Name)
		if err != nil {
			return nil, fmt.Errorf("error parsing source image %s: %v", img.Name, err)
		}
		if setLatest(srcRef) {
			srcRef.Ref.Tag = "latest"
		}

		// The registry component is not included in the final path.
		srcImage, err := bundle.PinImages(ctx, srcRef.Ref.Exact(), "", o.SourceSkipTLS, o.SourcePlainHTTP)
		if err != nil {
			return nil, err
		}
		pinnedRef, err := imagesource.ParseReference(srcImage)
		if err != nil {
			return nil, fmt.Errorf("error parsing source image %s: %v", img.Name, err)
		}
		srcRef.Ref.ID = pinnedRef.Ref.ID

		// Set destination image information as file by default
		dstRef := srcRef
		dstRef.Type = imagesource.DestinationFile
		dstRef.Ref = dstRef.Ref.DockerClientDefaults()
		dstRef.Ref.Registry = ""

		mmappings.Add(srcRef, dstRef, image.TypeGeneric)
	}

	return mmappings, nil
}

func setLatest(img imagesource.TypedImageReference) bool {
	return len(img.Ref.ID) == 0 && len(img.Ref.Tag) == 0
}
