package artifactfilter

import (
	"context"
	"errors"
	"fmt"

	"github.com/opencontainers/go-digest"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

// RetryCopyWithoutAttestations retries a failed copy without attestations.
// Callers must require --remove-referrers opt-in; copyImage applies normal retries.
func RetryCopyWithoutAttestations(ctx context.Context, log clog.PluggableLoggerInterface, sourceName string, sourceRef types.ImageReference, options *copy.Options, copyErr error, copyImage func(types.ImageReference, *copy.Options) error) error {
	if options.ImageListSelection != copy.CopyAllImages || ctx.Err() != nil ||
		errors.Is(copyErr, context.Canceled) || errors.Is(copyErr, context.DeadlineExceeded) {
		return copyErr
	}

	if sourceRef.Transport().Name() != "docker" {
		return copyErr
	}
	selection, inspectedDigest, err := inspectAttestations(ctx, sourceRef, options.SourceCtx)
	if err != nil {
		log.Debug("attestation check skipped for %s: %v", sourceName, err)
		return fmt.Errorf("%w; attestation check failed: %w", copyErr, err)
	}
	if !selection.canSkipAttestations() {
		return copyErr
	}
	if ctx.Err() != nil {
		return fmt.Errorf("%w; attestation check canceled: %w", copyErr, ctx.Err())
	}

	log.Warn("copying %s failed: %v; retrying once without %d attestation manifest(s)", sourceName, copyErr, len(selection.toSkip))
	retryOptions := *options
	retryOptions.ImageListSelection = copy.CopySpecificImages
	retryOptions.Instances = selection.toCopy
	retryOptions.InstancePlatforms = nil
	retrySource := indexSnapshotReference{ImageReference: sourceRef, indexDigest: inspectedDigest}
	if err := copyImage(retrySource, &retryOptions); err != nil {
		return fmt.Errorf("%w; retrying without attestation manifests also failed: %w", copyErr, err)
	}
	return nil
}

func inspectAttestations(ctx context.Context, sourceRef types.ImageReference, sourceCtx *types.SystemContext) (selection attestationSelection, indexDigest digest.Digest, retErr error) {
	snapshot, err := openManifestSnapshot(ctx, sourceRef, sourceCtx)
	if err != nil {
		return attestationSelection{}, "", err
	}
	defer func() {
		retErr = errors.Join(retErr, snapshot.source.Close())
	}()

	if snapshot.mimeType != specv1.MediaTypeImageIndex {
		return attestationSelection{}, "", nil
	}

	index, err := manifest.OCI1IndexFromManifest(snapshot.manifestBytes)
	if err != nil {
		return attestationSelection{}, "", fmt.Errorf("parsing manifest list for attestation check: %w", err)
	}

	return classifyAttestations(index), snapshot.digest, nil
}

// Preserve tag-based policy/routing and the raw source's signature capabilities
// while rejecting index changes between inspection and fallback.
type indexSnapshotReference struct {
	types.ImageReference
	indexDigest digest.Digest
}

//nolint:ireturn // implements types.ImageReference and must return the unwrapped source
func (ref indexSnapshotReference) NewImageSource(ctx context.Context, sourceCtx *types.SystemContext) (types.ImageSource, error) {
	snapshot, err := openManifestSnapshot(ctx, ref.ImageReference, sourceCtx)
	if err != nil {
		return nil, err
	}
	if snapshot.digest != ref.indexDigest {
		err := fmt.Errorf("source index changed before attestation fallback: inspected %s, opened %s", ref.indexDigest, snapshot.digest)
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
		return manifestSnapshot{}, fmt.Errorf("attestation inspection canceled: %w", err)
	}
	source, err := sourceRef.NewImageSource(ctx, sourceCtx)
	if err != nil {
		return manifestSnapshot{}, fmt.Errorf("opening attestation source: %w", err)
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, source.Close())
		}
	}()
	manifestBytes, mimeType, err := source.GetManifest(ctx, nil)
	if err != nil {
		return manifestSnapshot{}, fmt.Errorf("reading attestation source manifest: %w", err)
	}
	manifestDigest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return manifestSnapshot{}, fmt.Errorf("digesting attestation source manifest: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return manifestSnapshot{}, fmt.Errorf("attestation inspection canceled: %w", err)
	}
	return manifestSnapshot{source: source, manifestBytes: manifestBytes, mimeType: mimeType, digest: manifestDigest}, nil
}
