// Package artifactfilter implements --remove-oci-artifacts without rewriting indexes.
package artifactfilter

import (
	"fmt"

	"github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/manifest"
)

const (
	referenceTypeAnnotation          = "vnd.docker.reference.type"
	attestationManifestReferenceType = "attestation-manifest"
)

type artifactSelection struct {
	toCopy []digest.Digest
	toSkip map[digest.Digest]bool
}

// Use digests: artifacts can declare the same platform as runnable images.
func classifyArtifacts(index manifest.List) artifactSelection {
	selection := artifactSelection{toSkip: make(map[digest.Digest]bool)}
	ociIndex, ok := index.(*manifest.OCI1Index)
	if !ok {
		return artifactSelection{toCopy: index.Instances()}
	}
	digests := make([]digest.Digest, 0, len(ociIndex.Manifests))
	retainDigest := make(map[digest.Digest]bool, len(ociIndex.Manifests))
	for _, descriptor := range ociIndex.Manifests {
		retained, seen := retainDigest[descriptor.Digest]
		if !seen {
			digests = append(digests, descriptor.Digest)
		}
		retainDigest[descriptor.Digest] = retained || !isSkippableArtifact(descriptor.Annotations)
	}
	for _, instanceDigest := range digests {
		if retainDigest[instanceDigest] {
			selection.toCopy = append(selection.toCopy, instanceDigest)
		} else {
			selection.toSkip[instanceDigest] = true
		}
	}
	return selection
}

// Only annotated attestation manifests are currently eligible for omission.
func isSkippableArtifact(annotations map[string]string) bool {
	return annotations[referenceTypeAnnotation] == attestationManifestReferenceType
}

func ArtifactsToSkip(index manifest.List) (map[digest.Digest]bool, error) {
	selection := classifyArtifacts(index)
	if len(selection.toSkip) > 0 && len(selection.toCopy) == 0 {
		return nil, fmt.Errorf("cannot remove OCI artifacts: index contains no non-artifact manifests")
	}
	return selection.toSkip, nil
}
