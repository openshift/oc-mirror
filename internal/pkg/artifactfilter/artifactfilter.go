// Package artifactfilter owns oc-mirror's temporary compatibility workaround
// for attestation entries of an image index that a registry may be unable to
// serve.
//
// Multi-arch images built with buildx may list attestation manifests (SLSA
// provenance) as index entries annotated with
// "vnd.docker.reference.type": "attestation-manifest", and proxy registries
// like registry.connect.redhat.com may not serve them. Copies and archives run
// normally first and keep available attestations; only after a failure are
// annotated entries left out. The annotation is the only signal trusted, and a
// digest is excluded only when every descriptor referencing it is annotated.
// The index bytes and digest are never edited, so a destination receives a
// sparse index when entries are left out. Explicit platform selection takes
// precedence, and an index made only of excluded entries is never narrowed.
//
//   - Image copies (RetryCopyWithoutExcluded): after a copy.CopyAllImages copy
//     from a docker source finally failed, unless the operation was cancelled or
//     timed out, the source index is inspected once and, if it lists excluded
//     entries, the copy is retried once with only the entries not excluded,
//     from the original reference after its manifest matches the inspected
//     digest. Failures keep the original error in the chain.
//   - Archive gathering (ArchiveSkipper): an entry may be left out only when a
//     verified child read carries the registry's typed manifest-unknown code,
//     the entry is excluded, a non-excluded entry remains, no explicit platform
//     filter applies and the operation was not cancelled. Every other error
//     still fails the archive.
//
// Removing the workaround means deleting this package, the
// RetryCopyWithoutExcluded call in internal/pkg/mirror/mirror.go and the
// ArchiveSkipper in internal/pkg/archive/image-blob-gatherer.go.
package artifactfilter

import (
	"context"
	"errors"
	"fmt"

	"github.com/docker/distribution/registry/api/errcode"
	v2 "github.com/docker/distribution/registry/api/v2"
	"github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/manifest"
)

const (
	// referenceTypeAnnotation is the index descriptor annotation buildx uses to
	// state the purpose of a non-image entry.
	referenceTypeAnnotation = "vnd.docker.reference.type"
	// attestationManifestReferenceType is the referenceTypeAnnotation value of
	// an attestation manifest entry.
	attestationManifestReferenceType = "attestation-manifest"
)

// instanceSelection is the outcome of applying the enabled exclusion rule to the
// instances of one image index. Both slices keep index order.
type instanceSelection struct {
	// Retained holds the instances that are not identified as skippable artifacts.
	Retained []digest.Digest
	// Excluded holds the instances identified as skippable artifacts.
	Excluded []digest.Digest
}

// narrowed reports whether Retained should replace a traversal of every
// instance: at least one entry was excluded and at least one entry remains.
// Otherwise (nothing excluded, or nothing left to copy) callers keep their
// original full traversal.
func (s instanceSelection) narrowed() bool {
	return len(s.Excluded) > 0 && len(s.Retained) > 0
}

// selectInstances classifies every instance of list as retained or excluded.
//
// Entries are selected by digest rather than by OS/architecture: a platform
// filter matches every entry sharing that platform, so an attestation that
// carries a real platform would be selected again by the very image it
// attests. Digests identify each retained entry exactly.
func selectInstances(list manifest.List) (instanceSelection, error) {
	var selection instanceSelection
	if index, ok := list.(*manifest.OCI1Index); ok && index != nil {
		return selectOCIInstances(index), nil
	}
	seen := make(map[digest.Digest]bool)
	for _, d := range list.Instances() {
		_, err := list.Instance(d)
		if err != nil {
			// Bail out rather than returning a partial selection, which would
			// silently drop a real entry from the copy.
			return instanceSelection{}, fmt.Errorf("reading manifest list instance %s for attestation check: %w", d, err)
		}

		if !seen[d] {
			selection.Retained = append(selection.Retained, d)
			seen[d] = true
		}
	}
	return selection, nil
}

func selectOCIInstances(index *manifest.OCI1Index) instanceSelection {
	var selection instanceSelection
	order := make([]digest.Digest, 0, len(index.Manifests))
	allAnnotated := make(map[digest.Digest]bool, len(index.Manifests))
	for _, descriptor := range index.Manifests {
		previous, seen := allAnnotated[descriptor.Digest]
		if !seen {
			order = append(order, descriptor.Digest)
		}
		annotated := isAttestationManifest(descriptor.Annotations)
		allAnnotated[descriptor.Digest] = annotated && (!seen || previous)
	}
	for _, instance := range order {
		if allAnnotated[instance] {
			selection.Excluded = append(selection.Excluded, instance)
		} else {
			selection.Retained = append(selection.Retained, instance)
		}
	}
	return selection
}

// isAttestationManifest returns true if a manifest list entry is an attestation
// manifest, identified by the "vnd.docker.reference.type" annotation that buildx
// sets on the descriptor (SLSA provenance).
//
// The annotation is the only signal trusted here: it states the entry's purpose
// outright, so an entry is never dropped from the copy on a guess.
func isAttestationManifest(annotations map[string]string) bool {
	return annotations[referenceTypeAnnotation] == attestationManifestReferenceType
}

// ArchiveSkipper decides which entries of one index archive gathering may leave
// out after reading their manifest failed. Creating it does no work; the index
// is classified at most once, on the first eligible failure. It is meant for the
// single goroutine that gathers one index and is not safe for concurrent use.
type ArchiveSkipper struct {
	list              manifest.List
	explicitPlatforms bool
	classified        bool
	excluded          map[digest.Digest]bool
}

// NewArchiveSkipper returns an ArchiveSkipper for list. explicitPlatforms reports
// whether an explicit platform filter applies, in which case nothing is skipped.
func NewArchiveSkipper(list manifest.List, explicitPlatforms bool) *ArchiveSkipper {
	return &ArchiveSkipper{list: list, explicitPlatforms: explicitPlatforms}
}

// Skip reports whether archive gathering may leave out instance after reading
// its manifest failed with readErr, under the conditions in the package
// documentation. The caller must verify child-read provenance before passing
// readErr and must add nothing from a skipped entry.
func (s *ArchiveSkipper) Skip(ctx context.Context, instance digest.Digest, readErr error) bool {
	if s.explicitPlatforms || ctx.Err() != nil || errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) || !isManifestUnknown(readErr) {
		return false
	}
	if !s.classified {
		s.classified = true
		if selection, err := selectInstances(s.list); err == nil && selection.narrowed() {
			s.excluded = make(map[digest.Digest]bool, len(selection.Excluded))
			for _, d := range selection.Excluded {
				s.excluded[d] = true
			}
		}
	}
	return s.excluded[instance]
}

// isManifestUnknown reports whether err carries the registry error code for an
// unknown manifest. Transport, server and authentication errors do not. The
// docker/distribution v2 error types are required because the container image
// library reports registry errors with them.
func isManifestUnknown(err error) bool {
	for current := err; current != nil; current = errors.Unwrap(current) {
		if _, multiple := current.(interface{ Unwrap() []error }); multiple {
			return false
		}
	}
	var coder errcode.ErrorCoder
	return errors.As(err, &coder) && coder.ErrorCode() == v2.ErrorCodeManifestUnknown
}
