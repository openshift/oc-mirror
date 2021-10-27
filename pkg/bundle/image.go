package bundle

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"github.com/sirupsen/logrus"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/RedHatGov/bundle/pkg/image"
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

func pinImages(ctx context.Context, ref, resolverConfigPath string, insecure bool) (string, error) {
	resolver, err := containerdregistry.NewResolver(resolverConfigPath, insecure, nil)
	if err != nil {
		return "", fmt.Errorf("error creating image resolver: %v", err)
	}

	if !image.IsImagePinned(ref) {
		return image.ResolveToPin(ctx, resolver, ref)
	}

	return ref, nil
}
