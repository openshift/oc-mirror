package manifest

import (
	"context"
	"fmt"

	digest "github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/transports/alltransports"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func (o Manifest) ImageManifest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, instanceDigest *digest.Digest) ([]byte, string, error) {
	srcRef, err := alltransports.ParseImageName(imgRef)
	if err != nil {
		return nil, "", fmt.Errorf("invalid source name %s: %w", imgRef, err)
	}

	img, err := srcRef.NewImageSource(ctx, sourceCtx)
	if err != nil {
		return nil, "", fmt.Errorf("error when creating a new image source: %w", err)
	}
	defer img.Close()

	bytesManifest, mime, err := img.GetManifest(ctx, instanceDigest)
	if err != nil {
		return nil, "", fmt.Errorf("error to get the image manifest and mime type %w", err)
	}

	return bytesManifest, mime, nil
}

func (o Manifest) ImageDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	if err := mirror.ReexecIfNecessaryForImages(imgRef); err != nil {
		return "", fmt.Errorf("reexec mirror: %w", err)
	}

	manifestBytes, _, err := o.ImageManifest(ctx, sourceCtx, imgRef, nil)
	if err != nil {
		return "", fmt.Errorf("get manifest: %w", err)
	}

	digest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return "", fmt.Errorf("get manifest digest: %w", err)
	}

	return digest.Encoded(), nil
}
