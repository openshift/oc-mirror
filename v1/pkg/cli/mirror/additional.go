package mirror

import (
	"context"
	"errors"
	"fmt"

	"github.com/containerd/containerd/errdefs"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
)

type AdditionalOptions struct {
	*MirrorOptions
}

type ErrorImage struct {
	image  v1alpha2.Image
	reason string
}

func NewAdditionalOptions(mo *MirrorOptions) *AdditionalOptions {
	opts := &AdditionalOptions{MirrorOptions: mo}
	return opts
}

// Plan provides an image mapping with source and destination for provided AdditionalImages
func (o *AdditionalOptions) Plan(ctx context.Context, imageList []v1alpha2.Image) (image.TypedImageMapping, error) {
	mmappings := make(image.TypedImageMapping, len(imageList))
	sysContext := image.NewSystemContext(o.SourceSkipTLS || o.SourcePlainHTTP, o.OCIRegistriesConfig)

	var errorImageList []ErrorImage
	for _, img := range imageList {
		// Get source image information
		srcRef, err := image.ParseReference(img.Name)
		if err != nil {
			return mmappings, fmt.Errorf("error parsing source image %s: %v", img.Name, err)
		}
		srcRef.Ref = srcRef.Ref.DockerClientDefaults()

		// Instead of returning an error, just log it.
		isSkipErr := func(err error) bool {
			return o.ContinueOnError || (o.SkipMissing && errors.Is(err, errdefs.ErrNotFound))
		}

		ref := srcRef.Ref.Exact()
		if !image.IsImagePinned(ref) {
			srcImage, err := image.ResolveToPin(ctx, sysContext, ref)
			if err != nil {
				if !isSkipErr(err) {
					return mmappings, err
				}
				errorImageList = append(errorImageList, ErrorImage{image: img, reason: err.Error()})
				klog.Warning(err)
				continue
			}
			pinnedRef, err := image.ParseReference(srcImage)
			if err != nil {
				return mmappings, fmt.Errorf("error parsing source image %s: %v", img.Name, err)
			}
			srcRef.Ref.ID = pinnedRef.Ref.ID
		}

		// Set destination image information as file by default
		dstRef := srcRef
		dstRef.Type = imagesource.DestinationFile
		// The registry component is not included in the final path.
		dstRef.Ref.Registry = ""

		mmappings.Add(srcRef, dstRef, v1alpha2.TypeGeneric)
	}
	// Create a new tar archive for writing
	klog.Infof("error image list %s", errorImageList)
	return mmappings, nil
}
