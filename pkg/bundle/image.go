package bundle

import (
	"github.com/openshift/library-go/pkg/image/reference"
)

// IsBlocked will return a boolean value on whether an image
// is specified as blocked in the BundleSpec
func IsBlocked(cfg *BundleSpec, imgRef reference.DockerImageReference) bool {

	for _, block := range cfg.BlockedImages {
		if imgRef.Exact() == block {
			return true
		}
	}
	return false
}
