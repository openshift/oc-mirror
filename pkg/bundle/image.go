package bundle

import (
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/sirupsen/logrus"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

// IsBlocked will return a boolean value on whether an image
// is specified as blocked in the BundleSpec
func IsBlocked(cfg v1alpha1.ImageSetConfiguration, imgRef reference.DockerImageReference) bool {

	for _, block := range cfg.Mirror.BlockedImages {

		logrus.Debugf("Checking if image %s is blocked", imgRef.Exact())

		if imgRef.Name == block.Name {
			return true
		}
	}
	return false
}
