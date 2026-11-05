package manifest

import (
	"context"
	"fmt"

	digest "github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/transports/alltransports"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type cachedManifest struct {
	bytes []byte
	mime  string
}

func manifestCacheKey(imgRef string, instanceDigest *digest.Digest) string {
	if instanceDigest == nil {
		return imgRef + "|*"
	}
	return imgRef + "|" + instanceDigest.String()
}

func (o Manifest) ImageManifest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, instanceDigest *digest.Digest) ([]byte, string, error) {
	cacheKey := manifestCacheKey(imgRef, instanceDigest)
	if o.fetchCache != nil {
		if v, ok := o.fetchCache.Load(cacheKey); ok {
			cached := v.(cachedManifest)
			return cached.bytes, cached.mime, nil
		}
	}

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

	if o.fetchCache != nil {
		o.fetchCache.Store(cacheKey, cachedManifest{bytes: bytesManifest, mime: mime})
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

// parseBlobSizes parses a single-architecture manifest and returns the digest+size of every blob (layers + config).
// Blobs with Size <= 0 (e.g. schema1 unknown size -1) are skipped; their count is returned so callers can warn.
func parseBlobSizes(manifestBytes []byte, mimeType string) ([]v2alpha1.BlobSizeInfo, int, error) {
	parsed, err := manifest.FromBlob(manifestBytes, mimeType)
	if err != nil {
		return nil, 0, fmt.Errorf("error parsing manifest: %w", err)
	}
	var blobs []v2alpha1.BlobSizeInfo
	var unknownSizeBlobs int
	for _, layer := range parsed.LayerInfos() {
		if layer.Size <= 0 {
			unknownSizeBlobs++
			continue
		}
		blobs = append(blobs, v2alpha1.BlobSizeInfo{
			Digest: layer.Digest.String(),
			Size:   layer.Size,
		})
	}
	cfg := parsed.ConfigInfo()
	switch {
	case cfg.Digest == "":
		// No config blob present.
	case cfg.Size <= 0:
		unknownSizeBlobs++
	default:
		blobs = append(blobs, v2alpha1.BlobSizeInfo{
			Digest: cfg.Digest.String(),
			Size:   cfg.Size,
		})
	}
	return blobs, unknownSizeBlobs, nil
}

func (o Manifest) warnUnknownSizeBlobs(count int) {
	if count <= 0 || o.Log == nil {
		return
	}
	o.Log.Warn("%d blob(s) have unknown size and were excluded from the size estimate", count)
}

// ImageBlobSizes fetches the manifest for imgRef and returns the digest+size of every blob.
// For manifest lists, sub-manifests are resolved per platform and attestation manifests are skipped.
func (o Manifest) ImageBlobSizes(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, platformFilters []v2alpha1.InstancePlatformFilter) ([]v2alpha1.BlobSizeInfo, error) {
	manifestBytes, mimeType, err := o.ImageManifest(ctx, sourceCtx, imgRef, nil)
	if err != nil {
		return nil, fmt.Errorf("error fetching manifest for %s: %w", imgRef, err)
	}

	if !manifest.MIMETypeIsMultiImage(mimeType) {
		blobs, unknown, parseErr := parseBlobSizes(manifestBytes, mimeType)
		if parseErr != nil {
			return nil, parseErr
		}
		o.warnUnknownSizeBlobs(unknown)
		return blobs, nil
	}

	list, err := manifest.ListFromBlob(manifestBytes, mimeType)
	if err != nil {
		return nil, fmt.Errorf("error parsing manifest list for %s: %w", imgRef, err)
	}

	return o.blobSizesFromManifestList(ctx, sourceCtx, imgRef, list, platformFilters)
}

func shouldSizeManifestInstance(inst manifest.ListUpdate, instErr error, allowedPlatforms map[string]bool) bool {
	if instErr == nil && inst.ReadOnly.Annotations["vnd.docker.reference.type"] == "attestation-manifest" {
		return false
	}
	if len(allowedPlatforms) == 0 {
		return true
	}
	// Match copy.InstancePlatforms: nil platform does not match a concrete os/arch.
	if instErr != nil || inst.ReadOnly.Platform == nil {
		return false
	}
	return allowedPlatforms[inst.ReadOnly.Platform.OS+"/"+inst.ReadOnly.Platform.Architecture]
}

func (o Manifest) blobSizesFromManifestList(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, list manifest.List, platformFilters []v2alpha1.InstancePlatformFilter) ([]v2alpha1.BlobSizeInfo, error) {
	allowedPlatforms := make(map[string]bool, len(platformFilters))
	for _, pf := range platformFilters {
		allowedPlatforms[pf.String()] = true
	}

	seen := make(map[string]bool)
	var allBlobs []v2alpha1.BlobSizeInfo
	var skippedInstances, candidateInstances, unknownSizeBlobs int

	for _, instanceDigest := range list.Instances() {
		inst, instErr := list.Instance(instanceDigest)
		if !shouldSizeManifestInstance(inst, instErr, allowedPlatforms) {
			continue
		}

		candidateInstances++
		d := instanceDigest
		subBytes, subMime, subErr := o.ImageManifest(ctx, sourceCtx, imgRef, &d)
		if subErr != nil {
			skippedInstances++
			continue
		}

		subBlobs, unknown, parseErr := parseBlobSizes(subBytes, subMime)
		if parseErr != nil {
			skippedInstances++
			continue
		}
		unknownSizeBlobs += unknown

		for _, blob := range subBlobs {
			if !seen[blob.Digest] {
				seen[blob.Digest] = true
				allBlobs = append(allBlobs, blob)
			}
		}
	}

	return o.finishManifestListBlobSizes(allBlobs, len(allowedPlatforms) > 0, candidateInstances, skippedInstances, unknownSizeBlobs)
}

func (o Manifest) finishManifestListBlobSizes(blobs []v2alpha1.BlobSizeInfo, filtered bool, candidates, skipped, unknownSizeBlobs int) ([]v2alpha1.BlobSizeInfo, error) {
	if filtered && candidates == 0 {
		return nil, fmt.Errorf("no instances matched the requested platforms")
	}
	if skipped > 0 {
		return nil, fmt.Errorf("%d of %d sub-manifests could not be sized", skipped, candidates)
	}
	o.warnUnknownSizeBlobs(unknownSizeBlobs)
	return blobs, nil
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
