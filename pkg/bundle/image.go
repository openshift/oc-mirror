package bundle

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"github.com/sirupsen/logrus"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
)

type ErrBlocked struct {
	image string
}

func (e ErrBlocked) Error() string {
	return fmt.Sprintf("image %s blocked", e.image)
}

// IsBlocked will return a boolean value on whether an image
// is specified as blocked in the ImageSetConfigSpec
func IsBlocked(blocked []v1alpha2.Image, imgRef reference.DockerImageReference) bool {

	for _, block := range blocked {

		logrus.Debugf("Checking if image %s is blocked", imgRef.Exact())

		if imgRef.Name == block.Name {
			return true
		}
	}
	return false
}

func PinImages(ctx context.Context, ref, resolverConfigPath string, skipTLSVerify, plainHTTP bool) (string, error) {
	resolver, err := containerdregistry.NewResolver(resolverConfigPath, skipTLSVerify, plainHTTP, nil)
	if err != nil {
		return "", fmt.Errorf("error creating image resolver: %v", err)
	}
	if !image.IsImagePinned(ref) {
		return image.ResolveToPin(ctx, resolver, ref)
	}
	return ref, nil
}
