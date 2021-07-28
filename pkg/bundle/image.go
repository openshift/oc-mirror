package bundle

import (
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/openshift/library-go/pkg/image/reference"
)

// IsBlocked will return a boolean value on whether an image
// is specified as blocked in the BundleSpec
func IsBlocked(cfg v1alpha1.ImageSetConfiguration, imgRef reference.DockerImageReference) bool {

	for _, block := range cfg.Mirror.BlockedImages {
		if imgRef.Exact() == block.Name {
			return true
		}
	}
	return false
}
