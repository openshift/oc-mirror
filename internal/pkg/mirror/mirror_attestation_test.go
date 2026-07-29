package mirror

import (
	"context"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/transports/alltransports"
	podmanTypes "go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
)

// pushMultiArchWithAttestation creates a manifest list with two real platform
// images (linux/amd64, linux/arm64) and one attestation entry
// (unknown/unknown), then pushes it to the given registry.
func pushMultiArchWithAttestation(t *testing.T, host string) string {
	t.Helper()

	ref := host + "/test/attestation:latest"
	tag, err := name.ParseReference(ref, name.Insecure)
	require.NoError(t, err)

	amd64, err := random.Image(256, 1)
	require.NoError(t, err)

	arm64, err := random.Image(256, 1)
	require.NoError(t, err)

	attestation, err := random.Image(64, 1)
	require.NoError(t, err)

	amd64Desc, err := partial_desc(amd64, "linux", "amd64")
	require.NoError(t, err)

	arm64Desc, err := partial_desc(arm64, "linux", "arm64")
	require.NoError(t, err)

	attestDesc, err := partial_desc(attestation, "unknown", "unknown")
	require.NoError(t, err)
	attestDesc.Annotations = map[string]string{
		"vnd.docker.reference.type": "attestation-manifest",
	}

	idx := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{Add: amd64, Descriptor: amd64Desc},
		mutate.IndexAddendum{Add: arm64, Descriptor: arm64Desc},
		mutate.IndexAddendum{Add: attestation, Descriptor: attestDesc},
	)

	err = remote.WriteIndex(tag, idx)
	require.NoError(t, err)

	return ref
}

// pushMultiArchClean creates a manifest list with two real platform images
// and no attestation entries.
func pushMultiArchClean(t *testing.T, host string) string {
	t.Helper()

	ref := host + "/test/clean:latest"
	tag, err := name.ParseReference(ref, name.Insecure)
	require.NoError(t, err)

	amd64, err := random.Image(256, 1)
	require.NoError(t, err)

	arm64, err := random.Image(256, 1)
	require.NoError(t, err)

	amd64Desc, err := partial_desc(amd64, "linux", "amd64")
	require.NoError(t, err)

	arm64Desc, err := partial_desc(arm64, "linux", "arm64")
	require.NoError(t, err)

	idx := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{Add: amd64, Descriptor: amd64Desc},
		mutate.IndexAddendum{Add: arm64, Descriptor: arm64Desc},
	)

	err = remote.WriteIndex(tag, idx)
	require.NoError(t, err)

	return ref
}

// pushSingleImage creates a single-arch image (not a manifest list).
func pushSingleImage(t *testing.T, host string) string {
	t.Helper()

	ref := host + "/test/single:latest"
	tag, err := name.ParseReference(ref, name.Insecure)
	require.NoError(t, err)

	img, err := random.Image(256, 1)
	require.NoError(t, err)

	err = remote.Write(tag, img)
	require.NoError(t, err)

	return ref
}

func partial_desc(img v1.Image, os, arch string) (v1.Descriptor, error) {
	mt, err := img.MediaType()
	if err != nil {
		return v1.Descriptor{}, err
	}
	digest, err := img.Digest()
	if err != nil {
		return v1.Descriptor{}, err
	}
	size, err := img.Size()
	if err != nil {
		return v1.Descriptor{}, err
	}
	return v1.Descriptor{
		MediaType: mt,
		Digest:    digest,
		Size:      size,
		Platform: &v1.Platform{
			OS:           os,
			Architecture: arch,
		},
	}, nil
}

func TestFilterAttestationPlatforms(t *testing.T) {
	s := httptest.NewServer(registry.New())
	defer s.Close()
	u, err := url.Parse(s.URL)
	require.NoError(t, err)
	host := u.Host

	sysCtx := &podmanTypes.SystemContext{
		DockerInsecureSkipTLSVerify: podmanTypes.OptionalBoolTrue,
	}

	t.Run("manifest list with attestation returns only real platforms", func(t *testing.T) {
		ref := pushMultiArchWithAttestation(t, host)
		srcRef, err := alltransports.ParseImageName(consts.DockerProtocol + ref)
		require.NoError(t, err)

		platforms, err := filterAttestationPlatforms(context.Background(), srcRef, sysCtx)
		require.NoError(t, err)
		require.NotNil(t, platforms)
		assert.Len(t, platforms, 2)
		assert.Contains(t, platforms, copy.InstancePlatformFilter{OS: "linux", Architecture: "amd64"})
		assert.Contains(t, platforms, copy.InstancePlatformFilter{OS: "linux", Architecture: "arm64"})
	})

	t.Run("manifest list without attestation returns nil", func(t *testing.T) {
		ref := pushMultiArchClean(t, host)
		srcRef, err := alltransports.ParseImageName(consts.DockerProtocol + ref)
		require.NoError(t, err)

		platforms, err := filterAttestationPlatforms(context.Background(), srcRef, sysCtx)
		require.NoError(t, err)
		assert.Nil(t, platforms)
	})

	t.Run("single arch image returns nil", func(t *testing.T) {
		ref := pushSingleImage(t, host)
		srcRef, err := alltransports.ParseImageName(consts.DockerProtocol + ref)
		require.NoError(t, err)

		platforms, err := filterAttestationPlatforms(context.Background(), srcRef, sysCtx)
		require.NoError(t, err)
		assert.Nil(t, platforms)
	})

	t.Run("invalid image reference returns error", func(t *testing.T) {
		srcRef, err := alltransports.ParseImageName(consts.DockerProtocol + host + "/nonexistent/image:missing")
		require.NoError(t, err)

		_, err = filterAttestationPlatforms(context.Background(), srcRef, sysCtx)
		assert.Error(t, err)
	})
}
