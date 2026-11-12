package mirror

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/manifest"
)

func TestAttestationExcludedInstances(t *testing.T) {
	t.Parallel()
	runnable := specv1.Descriptor{MediaType: specv1.MediaTypeImageManifest, Digest: digest.FromString("runnable"), Platform: &specv1.Platform{OS: "linux", Architecture: "amd64"}}
	attestation := runnable
	attestation.Digest = digest.FromString("attestation")
	attestation.Annotations = map[string]string{"vnd.docker.reference.type": "attestation-manifest"}
	platformless := runnable
	platformless.Digest = digest.FromString("platformless")
	platformless.Platform = nil
	unknown := runnable
	unknown.Digest = digest.FromString("unknown")
	unknown.Platform = &specv1.Platform{OS: "unknown", Architecture: "unknown"}
	unknown.Annotations = map[string]string{"vnd.docker.reference.type": "other"}

	for _, test := range []struct {
		name      string
		instances []specv1.Descriptor
		want      []digest.Digest
	}{
		{name: "clean", instances: []specv1.Descriptor{runnable}},
		{name: "empty"},
		{name: "same platform attestation", instances: []specv1.Descriptor{runnable, attestation}, want: []digest.Digest{runnable.Digest}},
		{name: "unknown and platformless are retained", instances: []specv1.Descriptor{attestation, unknown, platformless}, want: []digest.Digest{unknown.Digest, platformless.Digest}},
		{name: "all attestations", instances: []specv1.Descriptor{attestation}, want: []digest.Digest{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(specv1.Index{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: specv1.MediaTypeImageIndex, Manifests: test.instances})
			require.NoError(t, err)
			list, err := manifest.ListFromBlob(data, specv1.MediaTypeImageIndex)
			require.NoError(t, err)
			selected, err := AttestationExcludedInstances(list)
			require.NoError(t, err)
			require.Equal(t, test.want, selected)
		})
	}
}

type failingAttestationList struct {
	manifest.List
	failure error
}

func (list failingAttestationList) Instances() []digest.Digest {
	return []digest.Digest{digest.FromString("first"), digest.FromString("second")}
}

func (list failingAttestationList) Instance(instance digest.Digest) (manifest.ListUpdate, error) {
	if instance == digest.FromString("second") {
		return manifest.ListUpdate{}, list.failure
	}
	return manifest.ListUpdate{}, nil
}

func TestAttestationExcludedInstancesRejectsPartialSelection(t *testing.T) {
	t.Parallel()
	failure := errors.New("unreadable descriptor")
	selected, err := AttestationExcludedInstances(failingAttestationList{failure: failure})
	require.ErrorIs(t, err, failure)
	require.Nil(t, selected)
}
