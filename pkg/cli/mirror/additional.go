package mirror

import (
	"context"
	"errors"
	"fmt"

	"github.com/containerd/containerd/errdefs"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/bundle"
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
func (o *AdditionalOptions) Plan(ctx context.Context, imageList []v1alpha2.Image) (image.TypedImageMapping, error) {
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

		// Instead of returning an error, just log it.
		isSkipErr := func(err error) bool {
			return o.ContinueOnError || (o.SkipMissing && errors.Is(err, errdefs.ErrNotFound))
		}

		srcImage, err := bundle.PinImages(ctx, srcRef.Ref.Exact(), "", o.SourceSkipTLS, o.SourcePlainHTTP)
		if err != nil {
			if !isSkipErr(err) {
				return nil, err
			}
			klog.Warning(err)
			continue
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
		// The registry component is not included in the final path.
		dstRef.Ref.Registry = ""

		mmappings.Add(srcRef, dstRef, v1alpha2.TypeGeneric)
	}

	return mmappings, nil
}

func setLatest(img imagesource.TypedImageReference) bool {
	return len(img.Ref.ID) == 0 && len(img.Ref.Tag) == 0
}
