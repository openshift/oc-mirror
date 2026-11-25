package artifactfilter

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/opencontainers/go-digest"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"
)

// PrepareCopy excludes artifact digests from options without changing the index.
//
//nolint:ireturn // the image library accepts transport-specific references through this interface
func PrepareCopy(ctx context.Context, sourceRef types.ImageReference, options *copy.Options) (types.ImageReference, error) {
	index, inspectedDigest, err := inspectIndex(ctx, sourceRef, options.SourceCtx)
	if err != nil {
		return nil, err
	}
	if index != nil {
		if err := excludeArtifactDigests(index, options); err != nil {
			return nil, err
		}
	}
	return manifestSnapshotReference{ImageReference: sourceRef, manifestDigest: inspectedDigest}, nil
}

func excludeArtifactDigests(index *manifest.OCI1Index, options *copy.Options) error {
	selection := classifyArtifacts(index)
	if len(selection.toSkip) == 0 {
		return nil
	}
	requested := index.Instances()
	switch options.ImageListSelection {
	case copy.CopyAllImages:
	case copy.CopySystemImage:
		instanceDigest, err := index.ChooseInstance(options.SourceCtx)
		if err != nil {
			return fmt.Errorf("selecting system image: %w", err)
		}
		requested = []digest.Digest{instanceDigest}
	case copy.CopySpecificImages:
		requested = nil
		for _, filter := range options.InstancePlatforms {
			instances, err := platformDigests(index, filter)
			if err != nil {
				return err
			}
			requested = append(requested, instances...)
		}
	}
	requestedCount := len(requested)
	toCopy := slices.DeleteFunc(requested, func(instanceDigest digest.Digest) bool {
		return selection.toSkip[instanceDigest]
	})
	if len(toCopy) == requestedCount {
		return nil
	}
	if len(toCopy) == 0 {
		return fmt.Errorf("cannot remove OCI artifacts: no non-artifact manifests match the copy selection")
	}
	options.ImageListSelection = copy.CopySpecificImages
	options.Instances = toCopy
	options.InstancePlatforms = nil
	options.PreserveDigests = true
	return nil
}

func platformDigests(index *manifest.OCI1Index, filter copy.InstancePlatformFilter) ([]digest.Digest, error) {
	var instances []digest.Digest
	for _, instanceDigest := range index.Instances() {
		info, err := index.Instance(instanceDigest)
		if err != nil {
			return nil, fmt.Errorf("reading instance %s: %w", instanceDigest, err)
		}
		platform := info.ReadOnly.Platform
		if platform == nil {
			platform = &specv1.Platform{}
		}
		if platform.OS == filter.OS && platform.Architecture == filter.Architecture {
			instances = append(instances, instanceDigest)
		}
	}
	if len(instances) == 0 {
		return nil, fmt.Errorf("no instances found for platform %s/%s", filter.OS, filter.Architecture)
	}
	return instances, nil
}

func inspectIndex(ctx context.Context, sourceRef types.ImageReference, sourceCtx *types.SystemContext) (index *manifest.OCI1Index, manifestDigest digest.Digest, retErr error) {
	snapshot, err := openManifestSnapshot(ctx, sourceRef, sourceCtx)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		retErr = errors.Join(retErr, snapshot.source.Close())
	}()

	if snapshot.mimeType != specv1.MediaTypeImageIndex {
		return nil, snapshot.digest, nil
	}

	index, err = manifest.OCI1IndexFromManifest(snapshot.manifestBytes)
	if err != nil {
		return nil, "", fmt.Errorf("parsing manifest list for artifact check: %w", err)
	}

	return index, snapshot.digest, nil
}

// Preserve tag-based policy/routing and the raw source's signature capabilities
// while rejecting manifest changes between inspection and copying.
type manifestSnapshotReference struct {
	types.ImageReference
	manifestDigest digest.Digest
}

//nolint:ireturn // implements types.ImageReference and must return the unwrapped source
func (ref manifestSnapshotReference) NewImageSource(ctx context.Context, sourceCtx *types.SystemContext) (types.ImageSource, error) {
	snapshot, err := openManifestSnapshot(ctx, ref.ImageReference, sourceCtx)
	if err != nil {
		return nil, err
	}
	if snapshot.digest != ref.manifestDigest {
		err := fmt.Errorf("source manifest changed before copying without OCI artifacts: inspected %s, opened %s", ref.manifestDigest, snapshot.digest)
		return nil, errors.Join(err, snapshot.source.Close())
	}
	return snapshot.source, nil
}

type manifestSnapshot struct {
	source        types.ImageSource
	manifestBytes []byte
	mimeType      string
	digest        digest.Digest
}

// The caller owns the source on success.
func openManifestSnapshot(ctx context.Context, sourceRef types.ImageReference, sourceCtx *types.SystemContext) (_ manifestSnapshot, retErr error) {
	if err := ctx.Err(); err != nil {
		return manifestSnapshot{}, fmt.Errorf("artifact inspection canceled: %w", err)
	}
	source, err := sourceRef.NewImageSource(ctx, sourceCtx)
	if err != nil {
		return manifestSnapshot{}, fmt.Errorf("opening artifact source: %w", err)
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, source.Close())
		}
	}()
	manifestBytes, mimeType, err := source.GetManifest(ctx, nil)
	if err != nil {
		return manifestSnapshot{}, fmt.Errorf("reading artifact source manifest: %w", err)
	}
	manifestDigest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return manifestSnapshot{}, fmt.Errorf("digesting artifact source manifest: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return manifestSnapshot{}, fmt.Errorf("artifact inspection canceled: %w", err)
	}
	return manifestSnapshot{source: source, manifestBytes: manifestBytes, mimeType: mimeType, digest: manifestDigest}, nil
}
