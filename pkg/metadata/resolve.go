package metadata

import (
	"context"

	"github.com/containerd/containerd/remotes"
	imgreference "github.com/openshift/library-go/pkg/image/reference"
)

// ResolveToPin returns unresolvedImage's digest-pinned string representation.
func ResolveToPin(ctx context.Context, resolver remotes.Resolver, unresolvedImage string) (string, error) {
	// Get the image's registry-specific digest.
	_, desc, err := resolver.Resolve(ctx, unresolvedImage)
	if err != nil {
		return "", err
	}

	// Add the digest to the Reference to use it's Stringer implementation
	// to get the full image pin.
	ref, err := imgreference.Parse(unresolvedImage)
	if err != nil {
		return "", err
	}
	ref = ref.DockerClientDefaults()
	ref.ID = desc.Digest.String()

	return ref.String(), nil
}
