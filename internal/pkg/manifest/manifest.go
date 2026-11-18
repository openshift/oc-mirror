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
	return readManifest(ctx, sourceCtx, imgRef, instanceDigest, srcRef)
}

func readManifest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, instanceDigest *digest.Digest, srcRef types.ImageReference) ([]byte, string, error) {
	var requested digest.Digest
	if instanceDigest != nil {
		requested = *instanceDigest
	}
	img, err := srcRef.NewImageSource(ctx, sourceCtx)
	if err != nil {
		return nil, "", fmt.Errorf("error when creating a new image source: %w", closeFailedSource(img, err))
	}
	if !sourceIsLive(img) {
		return nil, "", fmt.Errorf("opening image source returned no source")
	}

	bytesManifest, mime, err := img.GetManifest(ctx, instanceDigest)
	closeErr := img.Close()
	if err != nil {
		if closeErr != nil {
			return nil, "", fmt.Errorf("%w; closing image source: %w", err, closeErr)
		}
		if srcRef.Transport().Name() == "docker" && instanceDigest != nil && requested.Validate() == nil {
			err = &InstanceManifestError{Source: imgRef, Instance: requested, Err: err}
		}
		return nil, "", fmt.Errorf("error to get the image manifest and mime type %w", err)
	}
	if closeErr != nil {
		return nil, "", fmt.Errorf("closing image source: %w", closeErr)
	}

	return bytesManifest, mime, nil
}

// GetManifestListDigests inspects the source image to check if it's a manifest list
// and returns the sub-manifest digests.
func (o Manifest) GetManifestListDigests(ctx context.Context, sourceCtx *types.SystemContext, source string) ([]string, error) {
	manifestBytes, manifestType, err := o.ImageManifest(ctx, sourceCtx, source, nil)
	if err != nil {
		return nil, err
	}

	if !manifest.MIMETypeIsMultiImage(manifestType) {
		return nil, nil
	}

	list, err := manifest.ListFromBlob(manifestBytes, manifestType)
	if err != nil {
		return nil, fmt.Errorf("error parsing manifest list for %s: %w", source, err)
	}

	instances := list.Instances()
	digests := make([]string, 0, len(instances))
	for _, instance := range instances {
		digests = append(digests, instance.String())
	}
	return digests, nil
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
