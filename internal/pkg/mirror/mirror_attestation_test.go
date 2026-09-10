package mirror

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
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

// pushAttestationOnly creates a manifest list holding nothing but an
// attestation entry.
func pushAttestationOnly(t *testing.T, host string) string {
	t.Helper()

	ref := host + "/test/attestation-only:latest"
	tag, err := name.ParseReference(ref, name.Insecure)
	require.NoError(t, err)

	attestation, err := random.Image(64, 1)
	require.NoError(t, err)

	attestDesc, err := partial_desc(attestation, "unknown", "unknown")
	require.NoError(t, err)
	attestDesc.Annotations = map[string]string{
		"vnd.docker.reference.type": "attestation-manifest",
	}

	idx := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{Add: attestation, Descriptor: attestDesc},
	)

	err = remote.WriteIndex(tag, idx)
	require.NoError(t, err)

	return ref
}

// pushMultiArchWithPlatformlessEntry creates a manifest list with one real
// platform image, one real entry whose descriptor carries no platform, and one
// attestation entry.
func pushMultiArchWithPlatformlessEntry(t *testing.T, host string) (string, string) {
	t.Helper()

	ref := host + "/test/platformless:latest"
	tag, err := name.ParseReference(ref, name.Insecure)
	require.NoError(t, err)

	amd64, err := random.Image(256, 1)
	require.NoError(t, err)

	platformless, err := random.Image(256, 1)
	require.NoError(t, err)

	attestation, err := random.Image(64, 1)
	require.NoError(t, err)

	amd64Desc, err := partial_desc(amd64, "linux", "amd64")
	require.NoError(t, err)

	platformlessDesc, err := partial_desc(platformless, "", "")
	require.NoError(t, err)
	platformlessDesc.Platform = nil

	attestDesc, err := partial_desc(attestation, "unknown", "unknown")
	require.NoError(t, err)
	attestDesc.Annotations = map[string]string{
		"vnd.docker.reference.type": "attestation-manifest",
	}

	idx := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{Add: amd64, Descriptor: amd64Desc},
		mutate.IndexAddendum{Add: platformless, Descriptor: platformlessDesc},
		mutate.IndexAddendum{Add: attestation, Descriptor: attestDesc},
	)

	err = remote.WriteIndex(tag, idx)
	require.NoError(t, err)

	return ref, platformlessDesc.Digest.String()
}

func partial_desc(img v1.Image, os, arch string) (v1.Descriptor, error) {
	mt, err := img.MediaType()
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("getting image media type: %w", err)
	}
	digest, err := img.Digest()
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("getting image digest: %w", err)
	}
	size, err := img.Size()
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("getting image size: %w", err)
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

		filter, err := filterAttestationInstances(context.Background(), srcRef, sysCtx)
		require.NoError(t, err)
		require.NotNil(t, filter)
		assert.False(t, filter.empty())
		assert.Len(t, filter.platforms, 2)
		assert.Contains(t, filter.platforms, copy.InstancePlatformFilter{OS: "linux", Architecture: "amd64"})
		assert.Contains(t, filter.platforms, copy.InstancePlatformFilter{OS: "linux", Architecture: "arm64"})
		assert.Empty(t, filter.digests)
	})

	t.Run("platform-less entry is kept by digest", func(t *testing.T) {
		ref, platformlessDigest := pushMultiArchWithPlatformlessEntry(t, host)
		srcRef, err := alltransports.ParseImageName(consts.DockerProtocol + ref)
		require.NoError(t, err)

		filter, err := filterAttestationInstances(context.Background(), srcRef, sysCtx)
		require.NoError(t, err)
		require.NotNil(t, filter)
		assert.False(t, filter.empty())
		assert.Equal(t, []copy.InstancePlatformFilter{{OS: "linux", Architecture: "amd64"}}, filter.platforms)
		require.Len(t, filter.digests, 1)
		assert.Equal(t, platformlessDigest, filter.digests[0].String())
	})

	t.Run("attestation-only manifest list selects nothing", func(t *testing.T) {
		ref := pushAttestationOnly(t, host)
		srcRef, err := alltransports.ParseImageName(consts.DockerProtocol + ref)
		require.NoError(t, err)

		filter, err := filterAttestationInstances(context.Background(), srcRef, sysCtx)
		require.NoError(t, err)
		require.NotNil(t, filter)
		// Nothing real to copy, so the caller keeps CopyAllImages.
		assert.True(t, filter.empty())
	})

	t.Run("manifest list without attestation returns nil", func(t *testing.T) {
		ref := pushMultiArchClean(t, host)
		srcRef, err := alltransports.ParseImageName(consts.DockerProtocol + ref)
		require.NoError(t, err)

		filter, err := filterAttestationInstances(context.Background(), srcRef, sysCtx)
		require.NoError(t, err)
		assert.Nil(t, filter)
	})

	t.Run("single arch image returns nil", func(t *testing.T) {
		ref := pushSingleImage(t, host)
		srcRef, err := alltransports.ParseImageName(consts.DockerProtocol + ref)
		require.NoError(t, err)

		filter, err := filterAttestationInstances(context.Background(), srcRef, sysCtx)
		require.NoError(t, err)
		assert.Nil(t, filter)
	})

	t.Run("invalid image reference returns error", func(t *testing.T) {
		srcRef, err := alltransports.ParseImageName(consts.DockerProtocol + host + "/nonexistent/image:missing")
		require.NoError(t, err)

		_, err = filterAttestationInstances(context.Background(), srcRef, sysCtx)
		assert.Error(t, err)
	})
}
