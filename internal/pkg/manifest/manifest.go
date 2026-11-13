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

// parseBlobSizes parses a single-architecture manifest and returns the digest+size of every blob (layers + config).
// Blobs with Size <= 0 (e.g. schema1 unknown size -1) are skipped; their count is returned so callers can warn.
func parseBlobSizes(manifestBytes []byte, mimeType string) ([]types.BlobInfo, int, error) {
	parsed, err := manifest.FromBlob(manifestBytes, mimeType)
	if err != nil {
		return nil, 0, fmt.Errorf("error parsing manifest: %w", err)
	}
	var blobs []types.BlobInfo
	var unknownSizeBlobs int
	for _, layer := range parsed.LayerInfos() {
		if layer.Size <= 0 {
			unknownSizeBlobs++
			continue
		}
		blobs = append(blobs, layer.BlobInfo)
	}
	cfg := parsed.ConfigInfo()
	switch {
	case cfg.Digest == "":
		// No config blob present.
	case cfg.Size <= 0:
		unknownSizeBlobs++
	default:
		blobs = append(blobs, cfg)
	}
	return blobs, unknownSizeBlobs, nil
}

func (o Manifest) warnUnknownSizeBlobs(count int) {
	if count <= 0 || o.Log == nil {
		return
	}
	o.Log.Warn("%d blob(s) have unknown size and were excluded from the size estimate", count)
}

func (o Manifest) InspectImage(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, platformFilters []v2alpha1.InstancePlatformFilter, includeSizes bool) ([]string, []types.BlobInfo, error) {
	srcRef, err := alltransports.ParseImageName(imgRef)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid source name %s: %w", imgRef, err)
	}
	source, err := srcRef.NewImageSource(ctx, sourceCtx)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating image source for %s: %w", imgRef, err)
	}
	defer source.Close()
	manifestBytes, mimeType, err := source.GetManifest(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("error fetching manifest for %s: %w", imgRef, err)
	}
	if !manifest.MIMETypeIsMultiImage(mimeType) {
		if !includeSizes {
			return nil, nil, nil
		}
		blobs, parseErr := o.singleManifestBlobSizes(manifestBytes, mimeType)
		return nil, append(blobs, manifestBlobInfo(manifestBytes, mimeType)), parseErr
	}
	list, err := manifest.ListFromBlob(manifestBytes, mimeType)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing manifest list for %s: %w", imgRef, err)
	}
	digests := make([]string, 0, len(list.Instances()))
	for _, instance := range list.Instances() {
		digests = append(digests, instance.String())
	}
	if !includeSizes {
		return digests, nil, nil
	}
	blobs, err := o.blobSizesFromManifestList(ctx, source, list, platformFilters)
	if err != nil {
		return digests, nil, err
	}
	return digests, append(blobs, manifestBlobInfo(manifestBytes, mimeType)), nil
}

func (o Manifest) singleManifestBlobSizes(data []byte, mimeType string) ([]types.BlobInfo, error) {
	blobs, unknown, err := parseBlobSizes(data, mimeType)
	o.warnUnknownSizeBlobs(unknown)
	if err == nil && unknown > 0 {
		err = fmt.Errorf("%d blobs have unknown sizes", unknown)
	}
	return blobs, err
}

// Keep the top-level manifest last so registry tag metadata can identify it.
func manifestBlobInfo(data []byte, mimeType string) types.BlobInfo {
	return types.BlobInfo{Digest: digest.FromBytes(data), Size: int64(len(data)), MediaType: mimeType}
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

func (o Manifest) blobSizesFromManifestList(ctx context.Context, source types.ImageSource, list manifest.List, platformFilters []v2alpha1.InstancePlatformFilter) ([]types.BlobInfo, error) {
	allowedPlatforms := make(map[string]bool, len(platformFilters))
	for _, pf := range platformFilters {
		allowedPlatforms[pf.String()] = true
	}

	var allBlobs []types.BlobInfo
	var skippedInstances, candidateInstances, unknownSizeBlobs int

	for _, instanceDigest := range list.Instances() {
		inst, instErr := list.Instance(instanceDigest)
		if !shouldSizeManifestInstance(inst, instErr, allowedPlatforms) {
			continue
		}

		candidateInstances++
		d := instanceDigest
		subBytes, subMime, subErr := source.GetManifest(ctx, &d)
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
		allBlobs = append(allBlobs, subBlobs...)
		allBlobs = append(allBlobs, manifestBlobInfo(subBytes, subMime))
	}

	return o.finishManifestListBlobSizes(allBlobs, len(allowedPlatforms) > 0, candidateInstances, skippedInstances, unknownSizeBlobs)
}

func (o Manifest) finishManifestListBlobSizes(blobs []types.BlobInfo, filtered bool, candidates, skipped, unknownSizeBlobs int) ([]types.BlobInfo, error) {
	if filtered && candidates == 0 {
		return nil, fmt.Errorf("no instances matched the requested platforms")
	}
	if skipped > 0 {
		return nil, fmt.Errorf("%d of %d sub-manifests could not be sized", skipped, candidates)
	}
	o.warnUnknownSizeBlobs(unknownSizeBlobs)
	if unknownSizeBlobs > 0 {
		return nil, fmt.Errorf("%d blobs have unknown sizes", unknownSizeBlobs)
	}
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
