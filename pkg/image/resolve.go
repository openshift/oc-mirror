package image

import (
	"context"
	"strings"

	"github.com/containerd/containerd/remotes"
	imgreference "github.com/openshift/library-go/pkg/image/reference"
)

// ResolveToPin returns unresolvedImage's digest-pinned string representation.
func ResolveToPin(ctx context.Context, resolver remotes.Resolver, unresolvedImage string) (string, error) {

	// Add the digest to the Reference to use it's Stringer implementation
	// to get the full image pin.
	ref, err := imgreference.Parse(unresolvedImage)
	if err != nil {
		return "", err
	}
	ref = ref.DockerClientDefaults()

	// Get the image's registry-specific digest.
	_, desc, err := resolver.Resolve(ctx, ref.String())
	if err != nil {
		return "", err
	}

	ref.ID = desc.Digest.String()

	return ref.String(), nil
}

// IsImagePinned returns true if img looks canonical.
func IsImagePinned(img string) bool {
	return strings.Contains(img, "@")
}

// IsImageTagged returns true if img has a tag.
func IsImageTagged(img string) bool {
	return strings.Contains(img, ":")
}
