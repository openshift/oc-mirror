package mirror

import (
	"context"
	"fmt"

	"github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"
)

// attestationFilter selects the entries of a manifest list that should be
// copied once the attestation manifests have been left out.
type attestationFilter struct {
	// platforms selects entries by OS/architecture.
	platforms []copy.InstancePlatformFilter
	// digests selects entries whose descriptor carries no platform, which
	// platforms cannot match.
	digests []digest.Digest
}

// empty reports whether the filter selects nothing at all, in which case the
// caller has no real entry to copy and should keep its original behavior.
func (f *attestationFilter) empty() bool {
	return len(f.platforms) == 0 && len(f.digests) == 0
}

// filterAttestationInstances inspects a source manifest list and returns the
// entries to copy when attestation manifests are present. Attestation manifests
// use platform {architecture: "unknown", os: "unknown"} and cannot be copied
// from proxy registries like registry.connect.redhat.com.
//
// Returns (nil, nil) if the source is not a manifest list or contains no
// attestation entries — callers should preserve the original copy behavior.
func filterAttestationInstances(ctx context.Context, srcRef types.ImageReference, sysCtx *types.SystemContext) (filter *attestationFilter, retErr error) {
	src, err := srcRef.NewImageSource(ctx, sysCtx)
	if err != nil {
		return nil, fmt.Errorf("opening image source for attestation check: %w", err)
	}
	defer func() {
		if err := src.Close(); err != nil {
			retErr = NoteCloseFailure(retErr, "closing image source for attestation check", err)
		}
	}()

	manifestBytes, mimeType, err := src.GetManifest(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("reading manifest for attestation check: %w", err)
	}

	if !manifest.MIMETypeIsMultiImage(mimeType) {
		return nil, nil
	}

	list, err := manifest.ListFromBlob(manifestBytes, mimeType)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest list for attestation check: %w", err)
	}

	return realInstances(list)
}

// realInstances returns a filter selecting every non-attestation entry in list,
// or (nil, nil) when list holds no attestation entries at all.
func realInstances(list manifest.List) (*attestationFilter, error) {
	hasAttestation := false
	filter := &attestationFilter{}

	for _, d := range list.Instances() {
		info, err := list.Instance(d)
		if err != nil {
			// Bail out rather than returning a partial selection, which would
			// silently drop a real entry from the copy.
			return nil, fmt.Errorf("reading manifest list instance %s for attestation check: %w", d, err)
		}

		if isAttestationManifest(info) {
			hasAttestation = true
			continue
		}

		// A descriptor without a platform cannot be matched by OS/architecture,
		// so keep it in the copy by selecting it on its digest instead.
		if p := info.ReadOnly.Platform; p != nil {
			filter.platforms = append(filter.platforms, copy.InstancePlatformFilter{
				OS:           p.OS,
				Architecture: p.Architecture,
			})
		} else {
			filter.digests = append(filter.digests, d)
		}
	}

	if !hasAttestation {
		return nil, nil
	}

	return filter, nil
}

// isAttestationManifest returns true if a manifest list entry is an attestation
// manifest — identified by the "vnd.docker.reference.type" annotation or by
// having an unknown/unknown platform (common for buildx SLSA provenance).
func isAttestationManifest(info manifest.ListUpdate) bool {
	if info.ReadOnly.Annotations["vnd.docker.reference.type"] == "attestation-manifest" {
		return true
	}
	p := info.ReadOnly.Platform
	return p != nil && p.OS == "unknown" && p.Architecture == "unknown"
}
