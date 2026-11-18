package artifactfilter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/docker/distribution/registry/api/errcode"
	v2 "github.com/docker/distribution/registry/api/v2"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/manifest"
)

func TestSelect(t *testing.T) {
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
	unknown.ArtifactType = "application/vnd.in-toto+json"
	// The same manifest referenced by an annotated and an unannotated descriptor.
	annotatedShared := attestation
	annotatedShared.Digest = runnable.Digest

	for _, test := range []struct {
		name         string
		mimeType     string
		instances    []specv1.Descriptor
		want         instanceSelection
		wantNarrowed bool
	}{
		{name: "clean", instances: []specv1.Descriptor{runnable}, want: instanceSelection{Retained: []digest.Digest{runnable.Digest}}},
		{name: "empty"},
		// Selecting by platform would also match an attestation declaring the image's platform.
		{name: "same platform attestation", instances: []specv1.Descriptor{runnable, attestation}, want: instanceSelection{Retained: []digest.Digest{runnable.Digest}, Excluded: []digest.Digest{attestation.Digest}}, wantNarrowed: true},
		// Only the attestation annotation excludes an entry: unknown platforms,
		// other reference types, artifact types and missing platforms are retained.
		{name: "unknown and platformless are retained", instances: []specv1.Descriptor{attestation, unknown, platformless}, want: instanceSelection{Retained: []digest.Digest{unknown.Digest, platformless.Digest}, Excluded: []digest.Digest{attestation.Digest}}, wantNarrowed: true},
		{name: "all attestations", instances: []specv1.Descriptor{attestation}, want: instanceSelection{Excluded: []digest.Digest{attestation.Digest}}},
		// A digest is excluded only when every descriptor referencing it is annotated.
		{name: "annotated duplicate of a runnable digest is retained", instances: []specv1.Descriptor{annotatedShared, runnable, attestation}, want: instanceSelection{Retained: []digest.Digest{runnable.Digest}, Excluded: []digest.Digest{attestation.Digest}}, wantNarrowed: true},
		{name: "runnable digest listed before its annotated duplicate is retained", instances: []specv1.Descriptor{runnable, annotatedShared}, want: instanceSelection{Retained: []digest.Digest{runnable.Digest}}},
		{name: "fully annotated duplicate is excluded", instances: []specv1.Descriptor{runnable, attestation, attestation}, want: instanceSelection{Retained: []digest.Digest{runnable.Digest}, Excluded: []digest.Digest{attestation.Digest}}, wantNarrowed: true},
		// The schema2 list parser drops descriptor annotations, so even an annotated entry is retained.
		{name: "docker schema2 list", mimeType: manifest.DockerV2ListMediaType, instances: []specv1.Descriptor{runnable, attestation}, want: instanceSelection{Retained: []digest.Digest{runnable.Digest, attestation.Digest}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			list := testList(t, test.mimeType, test.instances...)
			selected, err := selectInstances(list)
			require.NoError(t, err)
			require.Equal(t, test.want, selected)
			require.Equal(t, test.wantNarrowed, selected.narrowed())
		})
	}
}

type failingAttestationList struct {
	manifest.List
	failure error
}

// MIMEType selects the per-instance classification path.
func (list failingAttestationList) MIMEType() string {
	return manifest.DockerV2ListMediaType
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

func TestSelectRejectsPartialSelection(t *testing.T) {
	t.Parallel()
	failure := errors.New("unreadable descriptor")
	selected, err := selectInstances(failingAttestationList{failure: failure})
	require.ErrorIs(t, err, failure)
	require.Equal(t, instanceSelection{}, selected)
	// Archive gathering never skips an entry it cannot classify.
	skipper := NewArchiveSkipper(failingAttestationList{failure: failure}, false)
	require.False(t, skipper.Skip(context.Background(), digest.FromString("second"), manifestUnknown()))
}

func TestArchiveSkipper(t *testing.T) {
	t.Parallel()
	runnable := specv1.Descriptor{MediaType: specv1.MediaTypeImageManifest, Digest: digest.FromString("runnable"), Platform: &specv1.Platform{OS: "linux", Architecture: "amd64"}}
	attestation := specv1.Descriptor{MediaType: specv1.MediaTypeImageManifest, Digest: digest.FromString("attestation"), Platform: &specv1.Platform{OS: "unknown", Architecture: "unknown"}, Annotations: map[string]string{"vnd.docker.reference.type": "attestation-manifest"}}

	for _, test := range []struct {
		name              string
		cancelled         bool
		instances         []specv1.Descriptor
		instance          digest.Digest
		readErr           error
		explicitPlatforms bool
		want              bool
	}{
		{name: "unknown attestation manifest with a runnable entry", instances: []specv1.Descriptor{runnable, attestation}, instance: attestation.Digest, readErr: manifestUnknown(), want: true},
		{name: "unavailable attestation manifest", instances: []specv1.Descriptor{runnable, attestation}, instance: attestation.Digest, readErr: errors.New("received unexpected HTTP status: 503 Service Unavailable")},
		{name: "unauthorized attestation manifest", instances: []specv1.Descriptor{runnable, attestation}, instance: attestation.Digest, readErr: fmt.Errorf("reading manifest: %w", errcode.ErrorCodeUnauthorized.WithMessage("authentication required"))},
		{name: "missing local file", instances: []specv1.Descriptor{runnable, attestation}, instance: attestation.Digest, readErr: &os.PathError{Op: "open", Path: "synthetic", Err: os.ErrNotExist}},
		{name: "joined causes", instances: []specv1.Descriptor{runnable, attestation}, instance: attestation.Digest, readErr: errors.Join(manifestUnknown(), errors.New("second cause"))},
		{name: "unknown runnable manifest", instances: []specv1.Descriptor{runnable, attestation}, instance: runnable.Digest, readErr: manifestUnknown()},
		{name: "all-attestation index", instances: []specv1.Descriptor{attestation}, instance: attestation.Digest, readErr: manifestUnknown()},
		{name: "explicit platforms", instances: []specv1.Descriptor{runnable, attestation}, instance: attestation.Digest, readErr: manifestUnknown(), explicitPlatforms: true},
		{name: "cancelled context", cancelled: true, instances: []specv1.Descriptor{runnable, attestation}, instance: attestation.Digest, readErr: manifestUnknown()},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.cancelled {
				cancel()
			}
			skipper := NewArchiveSkipper(testList(t, "", test.instances...), test.explicitPlatforms)
			require.Equal(t, test.want, skipper.Skip(ctx, test.instance, test.readErr))
		})
	}
}

// countingList counts how often the index is classified through Instances.
type countingList struct {
	manifest.List
	instances int
}

func (list *countingList) Instances() []digest.Digest {
	list.instances++
	return list.List.Instances()
}

func TestArchiveSkipperClassifiesOnce(t *testing.T) {
	t.Parallel()
	attestation := specv1.Descriptor{MediaType: specv1.MediaTypeImageManifest, Platform: &specv1.Platform{OS: "unknown", Architecture: "unknown"}, Annotations: map[string]string{"vnd.docker.reference.type": "attestation-manifest"}}
	first, second := attestation, attestation
	first.Digest, second.Digest = digest.FromString("first"), digest.FromString("second")
	runnable := specv1.Descriptor{MediaType: specv1.MediaTypeImageManifest, Digest: digest.FromString("runnable"), Platform: &specv1.Platform{OS: "linux", Architecture: "amd64"}}
	list := &countingList{List: testList(t, "", runnable, first, second)}

	skipper := NewArchiveSkipper(list, false)
	require.False(t, skipper.Skip(context.Background(), first.Digest, errors.New("unavailable")))
	require.Zero(t, list.instances, "nothing is classified before an eligible failure")
	require.False(t, skipper.Skip(context.Background(), first.Digest, manifestUnknown()))
	classified := list.instances
	require.Positive(t, classified)
	require.False(t, skipper.Skip(context.Background(), second.Digest, manifestUnknown()))
	require.Equal(t, classified, list.instances, "the index is classified only once")
}

// manifestUnknown returns a wrapped registry "manifest unknown" error.
func manifestUnknown() error {
	return fmt.Errorf("reading manifest: %w", v2.ErrorCodeManifestUnknown.WithMessage("manifest unknown"))
}

// testList builds a manifest list of mimeType (an OCI index by default) from instances.
//
//nolint:ireturn // c/image represents manifest lists as the manifest.List interface
func testList(t *testing.T, mimeType string, instances ...specv1.Descriptor) manifest.List {
	t.Helper()
	if mimeType == "" {
		mimeType = specv1.MediaTypeImageIndex
	}
	data, err := json.Marshal(specv1.Index{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: mimeType, Manifests: instances})
	require.NoError(t, err)
	var list manifest.List
	if mimeType == specv1.MediaTypeImageIndex {
		list, err = manifest.OCI1IndexFromManifest(data)
	} else {
		list, err = manifest.ListFromBlob(data, mimeType)
	}
	require.NoError(t, err)
	return list
}
