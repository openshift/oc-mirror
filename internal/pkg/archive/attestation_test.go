package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/types"
	"k8s.io/apimachinery/pkg/util/sets"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	ocmirrormanifest "github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
)

type attestationManifestSource struct {
	ocmirrormanifest.ManifestInterface
	manifests map[digest.Digest][]byte
	requested []digest.Digest
}

func (source *attestationManifestSource) ImageManifest(_ context.Context, _ *types.SystemContext, _ string, instance *digest.Digest) ([]byte, string, error) {
	if instance == nil {
		return nil, "", fmt.Errorf("signature not present")
	}
	source.requested = append(source.requested, *instance)
	data, present := source.manifests[*instance]
	if !present {
		return nil, "", fmt.Errorf("missing manifest %s", *instance)
	}
	return data, specv1.MediaTypeImageManifest, nil
}

func TestMultiArchBlobsAttestationSelection(t *testing.T) {
	t.Parallel()
	config := specv1.Descriptor{MediaType: specv1.MediaTypeImageConfig, Digest: digest.FromString("config")}
	layer := specv1.Descriptor{MediaType: specv1.MediaTypeImageLayer, Digest: digest.FromString("layer")}
	childBytes, err := json.Marshal(specv1.Manifest{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: specv1.MediaTypeImageManifest, Config: config, Layers: []specv1.Descriptor{layer}})
	require.NoError(t, err)
	runnable := specv1.Descriptor{MediaType: specv1.MediaTypeImageManifest, Digest: digest.FromBytes(childBytes), Platform: &specv1.Platform{OS: "linux", Architecture: "amd64"}}
	attestation := runnable
	attestation.Digest = digest.FromString("attestation")
	attestation.Annotations = map[string]string{"vnd.docker.reference.type": "attestation-manifest"}
	other := runnable
	other.Digest = digest.FromString("other")
	other.Platform = &specv1.Platform{OS: "linux", Architecture: "arm64"}
	platformless := other
	platformless.Platform = nil
	unknown := other
	unknown.Platform = &specv1.Platform{OS: "unknown", Architecture: "unknown"}

	for _, test := range []struct {
		name           string
		instances      []specv1.Descriptor
		present        []digest.Digest
		allowed        []string
		wantRequested  []digest.Digest
		wantError      string
		copySignatures bool
	}{
		{name: "missing attestation omitted", instances: []specv1.Descriptor{runnable, attestation}, present: []digest.Digest{runnable.Digest}, wantRequested: []digest.Digest{runnable.Digest}},
		{name: "available attestation omitted", instances: []specv1.Descriptor{runnable, attestation}, present: []digest.Digest{runnable.Digest, attestation.Digest}, wantRequested: []digest.Digest{runnable.Digest}},
		{name: "missing runnable fails", instances: []specv1.Descriptor{runnable, attestation}, wantRequested: []digest.Digest{runnable.Digest}, wantError: "missing manifest"},
		{name: "clean missing runnable fails", instances: []specv1.Descriptor{runnable}, wantRequested: []digest.Digest{runnable.Digest}, wantError: "missing manifest"},
		{name: "explicit platform precedence", instances: []specv1.Descriptor{runnable, attestation}, present: []digest.Digest{runnable.Digest}, allowed: []string{"linux/amd64"}, wantRequested: []digest.Digest{runnable.Digest, attestation.Digest}, wantError: "missing manifest"},
		{name: "excluded runnable is sparse", instances: []specv1.Descriptor{runnable, other}, present: []digest.Digest{runnable.Digest}, allowed: []string{"linux/amd64"}, wantRequested: []digest.Digest{runnable.Digest}},
		{name: "explicit platform retains platformless", instances: []specv1.Descriptor{runnable, platformless}, present: []digest.Digest{runnable.Digest}, allowed: []string{"linux/amd64"}, wantRequested: []digest.Digest{runnable.Digest, platformless.Digest}, wantError: "missing manifest"},
		{name: "explicit platform excludes unknown", instances: []specv1.Descriptor{runnable, unknown}, present: []digest.Digest{runnable.Digest}, allowed: []string{"linux/amd64"}, wantRequested: []digest.Digest{runnable.Digest}},
		{name: "included runnable missing fails", instances: []specv1.Descriptor{runnable, other}, present: []digest.Digest{runnable.Digest}, allowed: []string{"linux/amd64", "linux/arm64"}, wantRequested: []digest.Digest{runnable.Digest, other.Digest}, wantError: "missing manifest"},
		{name: "platformless is not an attestation", instances: []specv1.Descriptor{runnable, platformless, attestation}, present: []digest.Digest{runnable.Digest}, wantRequested: []digest.Digest{runnable.Digest, platformless.Digest}, wantError: "missing manifest"},
		{name: "unknown is not an attestation", instances: []specv1.Descriptor{runnable, unknown, attestation}, present: []digest.Digest{runnable.Digest}, wantRequested: []digest.Digest{runnable.Digest, unknown.Digest}, wantError: "missing manifest"},
		{name: "all attestation missing fails", instances: []specv1.Descriptor{attestation}, wantRequested: []digest.Digest{attestation.Digest}, wantError: "missing manifest"},
		{name: "all attestation present retains traversal", instances: []specv1.Descriptor{attestation}, present: []digest.Digest{attestation.Digest}, wantRequested: []digest.Digest{attestation.Digest}},
		{name: "runnable signature errors retained", instances: []specv1.Descriptor{runnable, attestation}, present: []digest.Digest{runnable.Digest}, wantRequested: []digest.Digest{runnable.Digest}, copySignatures: true, wantError: "signature"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			indexBytes, err := json.Marshal(specv1.Index{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: specv1.MediaTypeImageIndex, Manifests: test.instances})
			require.NoError(t, err)
			source := &attestationManifestSource{manifests: map[digest.Digest][]byte{}}
			for _, instance := range test.present {
				source.manifests[instance] = childBytes
			}
			gatherer := &ImageBlobGatherer{ocmirrormanifest: source, log: clog.New("error")}
			blobs, err := gatherer.multiArchBlobs(context.Background(), internalImageBlobGatherer{imgRef: "docker://localhost/synthetic:latest", manifestBytes: indexBytes, mimeType: specv1.MediaTypeImageIndex, digest: digest.FromBytes(indexBytes), allowedPlatforms: test.allowed, copySignatures: test.copySignatures})
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				if test.copySignatures {
					var signatureError *SignatureBlobGathererError
					require.ErrorAs(t, err, &signatureError)
					require.Equal(t, sets.New(digest.FromBytes(indexBytes).String(), runnable.Digest.String(), config.Digest.String(), layer.Digest.String()), blobs)
				}
			} else {
				require.NoError(t, err)
				expected := sets.New(digest.FromBytes(indexBytes).String(), config.Digest.String(), layer.Digest.String())
				for _, instance := range test.wantRequested {
					expected.Insert(instance.String())
				}
				require.Equal(t, expected, blobs)
			}
			require.Equal(t, test.wantRequested, source.requested)
		})
	}
}

func TestArchiveMissingRunnableBlob(t *testing.T) {
	t.Parallel()
	for _, strict := range []bool{false, true} {
		for _, missing := range []string{"config", "layer"} {
			t.Run(fmt.Sprintf("strict=%t/missing=%s", strict, missing), func(t *testing.T) {
				t.Parallel()
				cache := t.TempDir()
				collected := sets.New[string]()
				for _, content := range []string{"manifest", "config", "layer"} {
					hash := digest.FromString(content)
					collected.Insert(hash.String())
					if content == missing {
						continue
					}
					path := filepath.Join(cache, cacheBlobsDir, "sha256", hash.Encoded()[:2], hash.Encoded(), "data")
					require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
					require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
				}
				archive := &MirrorArchive{cacheDir: cache, destination: t.TempDir(), strictArchiving: strict, logger: clog.New("error")}
				require.NoError(t, archive.createTarball())
				defer archive.adder.close()
				_, err := archive.addBlobsDiff(collected, sets.New[string](), sets.New[string]())
				require.ErrorIs(t, err, os.ErrNotExist)
			})
		}
	}
}
