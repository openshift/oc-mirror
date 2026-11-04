package mirror

import (
	"context"
	"fmt"

	"github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"
)

// attestationFilter selects the entries of a manifest list that should be
// copied once the attestation manifests have been left out.
//
// Entries are selected by digest rather than by OS/architecture: a platform
// filter matches every entry sharing that platform, so an attestation that
// carries a real platform would be selected again by the very image it
// attests. Digests identify each retained entry exactly.
type attestationFilter struct {
	digests []digest.Digest
}

// empty reports whether the filter selects nothing at all, in which case the
// caller has no real entry to copy and should keep its original behavior.
func (f *attestationFilter) empty() bool {
	return len(f.digests) == 0
}

// filterAttestationInstances inspects a source manifest list and returns the
// entries to copy when attestation manifests are present. Attestation manifests
// cannot be copied from proxy registries like registry.connect.redhat.com.
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
	digests, err := AttestationExcludedInstances(list)
	if err != nil || digests == nil {
		return nil, err
	}
	return &attestationFilter{digests: digests}, nil
}

// AttestationExcludedInstances returns non-attestation instance digests when the
// list contains annotated attestations, or nil when no exclusion is needed.
// An empty non-nil selection means every instance is an attestation; callers
// must retain their original traversal when no non-attestation instance remains.
func AttestationExcludedInstances(list manifest.List) ([]digest.Digest, error) {
	hasAttestation := false
	digests := make([]digest.Digest, 0, len(list.Instances()))

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

		digests = append(digests, d)
	}

	if !hasAttestation {
		return nil, nil
	}

	return digests, nil
}

// isAttestationManifest returns true if a manifest list entry is an attestation
// manifest, identified by the "vnd.docker.reference.type" annotation that buildx
// sets on the descriptor (SLSA provenance).
//
// The annotation is the only signal trusted here: it states the entry's purpose
// outright, so an entry is never dropped from the copy on a guess.
func isAttestationManifest(info manifest.ListUpdate) bool {
	return info.ReadOnly.Annotations["vnd.docker.reference.type"] == "attestation-manifest"
}
