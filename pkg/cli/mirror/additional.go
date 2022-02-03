package mirror

import (
	"fmt"

	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
)

// TODO(jpower432): determine whether this can be
// deleted or whether it will implement an interface

type AdditionalOptions struct{}

func (o *AdditionalOptions) Plan(imageList []v1alpha1.AdditionalImages) (image.TypedImageMapping, error) {
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
