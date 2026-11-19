// Package artifactfilter provides the temporary --remove-referrers attestation fallback.
//
// TODO(OCPNODE-2018): Remove when containers-libs supports OCI 1.1 referrers.
package artifactfilter

import (
	"context"
	"errors"

	"github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/manifest"
)

const (
	referenceTypeAnnotation          = "vnd.docker.reference.type"
	attestationManifestReferenceType = "attestation-manifest"
)

type attestationSelection struct {
	toCopy []digest.Digest
	toSkip []digest.Digest
}

func (s attestationSelection) canSkipAttestations() bool {
	return len(s.toSkip) > 0 && len(s.toCopy) > 0
}

// Use digests: attestations can declare the same platform as runnable images.
func classifyAttestations(index manifest.List) attestationSelection {
	var selection attestationSelection
	ociIndex, ok := index.(*manifest.OCI1Index)
	if !ok {
		return attestationSelection{toCopy: index.Instances()}
	}
	digests := make([]digest.Digest, 0, len(ociIndex.Manifests))
	retainDigest := make(map[digest.Digest]bool, len(ociIndex.Manifests))
	for _, descriptor := range ociIndex.Manifests {
		retained, seen := retainDigest[descriptor.Digest]
		if !seen {
			digests = append(digests, descriptor.Digest)
		}
		retainDigest[descriptor.Digest] = retained || !isAttestationManifest(descriptor.Annotations)
	}
	for _, instanceDigest := range digests {
		if retainDigest[instanceDigest] {
			selection.toCopy = append(selection.toCopy, instanceDigest)
		} else {
			selection.toSkip = append(selection.toSkip, instanceDigest)
		}
	}
	return selection
}

func isAttestationManifest(annotations map[string]string) bool {
	return annotations[referenceTypeAnnotation] == attestationManifestReferenceType
}

// AttestationSkipper lazily classifies one index. It is not safe for concurrent use.
type AttestationSkipper struct {
	index             manifest.List
	hasPlatformFilter bool
	skippableDigests  map[digest.Digest]bool
}

func NewAttestationSkipper(index manifest.List, hasPlatformFilter bool) *AttestationSkipper {
	return &AttestationSkipper{index: index, hasPlatformFilter: hasPlatformFilter}
}

// CanSkip requires caller opt-in; a skipped entry must contribute no blobs.
func (s *AttestationSkipper) CanSkip(ctx context.Context, instanceDigest digest.Digest, readErr error) bool {
	if readErr == nil || s.hasPlatformFilter || ctx.Err() != nil ||
		errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
		return false
	}
	if s.skippableDigests == nil {
		s.skippableDigests = make(map[digest.Digest]bool)
		if selection := classifyAttestations(s.index); selection.canSkipAttestations() {
			for _, instanceDigest := range selection.toSkip {
				s.skippableDigests[instanceDigest] = true
			}
		}
	}
	return s.skippableDigests[instanceDigest]
}
