package mirror

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/distribution/registry/api/errcode"
	errcodev2 "github.com/docker/distribution/registry/api/v2"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/common/pkg/retry"
	"go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/signature"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
)

func TestMirrorCopy(t *testing.T) {
	testFolder := t.TempDir()
	testFile := testFolder + "/testDigest.txt"

	global := &GlobalOptions{SecurePolicy: false}

	_, sharedOpts := SharedImageFlags()
	_, deprecatedTLSVerifyOpt := DeprecatedTLSVerifyFlags()
	_, srcOpts := ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := RetryFlags()

	opts := CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "oci:test",
		Dev:                 false,
		Mode:                MirrorToDisk,
		MultiArch:           "all",
		Format:              "oci",
		SignPassphraseFile:  "test-digest",
		DigestFile:          testFile,
	}

	mm := &mockMirrorCopy{}
	md := &mockMirrorDelete{}
	m := New(mm, md)

	t.Run("Testing Mirror : copy should pass", func(t *testing.T) {
		err := m.Run(context.Background(), consts.DockerProtocol+"localhost.localdomain:5000/test", "oci:test", "copy", &opts)
		if err != nil {
			t.Fatal("should pass")
		}
	})

	t.Run("Testing Mirror : copy should fail", func(t *testing.T) {
		err := m.Run(context.Background(), "broken", "oci:test", "copy", &opts)
		assert.Equal(t, "invalid source name broken: Invalid image name \"broken\", expected colon-separated transport:reference", err.Error())
	})

	t.Run("Testing Mirror : copy should fail", func(t *testing.T) {
		err := m.Run(context.Background(), consts.DockerProtocol+"localhost.localdomain:5000/tes", "broken", "copy", &opts)
		assert.Equal(t, "invalid destination name broken: Invalid image name \"broken\", expected colon-separated transport:reference", err.Error())
	})

	opts.MultiArch = "other"
	t.Run("Testing Mirror : copy should fail", func(t *testing.T) {
		err := m.Run(context.Background(), consts.DockerProtocol+"localhost.localdomain:5000/tes", "oci:test", "copy", &opts)
		assert.Contains(t, err.Error(), "unknown multi-arch option")
	})

	opts.All = true
	t.Run("Testing Mirror : copy should fail", func(t *testing.T) {
		err := m.Run(context.Background(), consts.DockerProtocol+"localhost.localdomain:5000/tes", "oci:test", "copy", &opts)
		assert.Equal(t, "MultiArch and All options cannot be used together", err.Error())
	})

	opts.All = true
	opts.MultiArch = ""
	opts.EncryptionKeys = []string{"test"}
	opts.DecryptionKeys = []string{"test"}
	t.Run("Testing Mirror : copy should fail", func(t *testing.T) {
		err := m.Run(context.Background(), consts.DockerProtocol+"localhost.localdomain:5000/tes", "oci:test", "copy", &opts)
		assert.Equal(t, "--encryption-key and --decryption-key cannot be specified together", err.Error())
	})

	opts.All = true
	opts.MultiArch = ""
	opts.EncryptionKeys = nil
	opts.DecryptionKeys = nil
	opts.SignPassphraseFile = "test"
	opts.SignByFingerprint = "test"
	opts.SignBySigstorePrivateKey = "test"
	t.Run("Testing Mirror : copy should fail", func(t *testing.T) {
		err := m.Run(context.Background(), consts.DockerProtocol+"localhost.localdomain:5000/tes", "oci:test", "copy", &opts)
		assert.Equal(t, "only one of --sign-by and sign-by-sigstore-private-key can be used with sign-passphrase-file", err.Error())
	})
}

func TestMirrorCheck(t *testing.T) {
	global := &GlobalOptions{SecurePolicy: false}

	_, sharedOpts := SharedImageFlags()
	_, deprecatedTLSVerifyOpt := DeprecatedTLSVerifyFlags()
	srcFlags, srcOpts := ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	dstFlags, destOpts := ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := RetryFlags()

	_ = srcFlags.Set("src-tls-verify", "false")
	_ = dstFlags.Set("dest-tls-verify", "false")
	opts := CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "oci:test",
		Dev:                 false,
		Mode:                MirrorToDisk,
		MultiArch:           "all",
		Format:              "oci",
		SignPassphraseFile:  "test-digest",
	}

	mm := &mockMirrorCopy{}
	md := &mockMirrorDelete{}
	m := New(mm, md)

	// Set up a fake registry.
	s := httptest.NewServer(registry.New())
	defer s.Close()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	imageAbsolutePath, err := filepath.Abs(consts.TestFolder + "albo-bundle-image")
	if err != nil {
		t.Fatal(err)
	}

	src := consts.DirProtocol + imageAbsolutePath
	dest := consts.DockerProtocol + u.Host + "/albo-test:latest"
	err = New(NewMirrorCopy(), NewMirrorDelete()).Run(context.Background(), src, dest, "copy", &opts)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Testing Mirror : check should pass", func(t *testing.T) {
		_, err := m.Check(context.Background(), dest, &opts, false)
		if err != nil {
			t.Fatal("should pass")
		}
	})

	t.Run("Testing Mirror : check should pass", func(t *testing.T) {
		_, err := m.Check(context.Background(), "broken", &opts, false)
		assert.Equal(t, "invalid source name broken: Invalid image name \"broken\", expected colon-separated transport:reference", err.Error())
	})
}

// TestMirrorDelete
func TestMirrorDelete(t *testing.T) {
	global := &GlobalOptions{SecurePolicy: false}

	_, sharedOpts := SharedImageFlags()
	_, deprecatedTLSVerifyOpt := DeprecatedTLSVerifyFlags()
	srcFlags, srcOpts := ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	dstFlags, destOpts := ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := RetryFlags()

	_ = srcFlags.Set("src-tls-verify", "false")
	_ = dstFlags.Set("dest-tls-verify", "false")

	opts := CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "oci:test",
		Dev:                 false,
		Mode:                MirrorToDisk,
		MultiArch:           "all",
		Format:              "oci",
		SignPassphraseFile:  "test-digest",
	}

	// Set up a fake registry.
	s := httptest.NewServer(registry.New())
	defer s.Close()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	imageAbsolutePath, err := filepath.Abs(consts.TestFolder + "albo-bundle-image")
	if err != nil {
		t.Fatal(err)
	}

	src := consts.DirProtocol + imageAbsolutePath
	dest := consts.DockerProtocol + u.Host + "/albo-test:latest"
	err = New(NewMirrorCopy(), NewMirrorDelete()).Run(context.Background(), src, dest, "copy", &opts)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Testing Mirror : delete should pass", func(t *testing.T) {
		err = New(NewMirrorCopy(), NewMirrorDelete()).Run(context.Background(), src, dest, "delete", &opts)
		if err != nil {
			t.Fatal("should not fail")
		}
	})

	t.Run("Testing Mirror : delete should fail", func(t *testing.T) {
		err = New(NewMirrorCopy(), NewMirrorDelete()).Run(context.Background(), src, "broken", "delete", &opts)
		assert.Equal(t, "invalid source name broken: Invalid image name \"broken\", expected colon-separated transport:reference", err.Error())
	})

	t.Run("Testing Mirror : delete should fail", func(t *testing.T) {
		err = New(NewMirrorCopy(), NewMirrorDelete()).Run(context.Background(), src, src, "delete", &opts)
		assert.Equal(t, "Deleting images not implemented for dir: images", err.Error())
	})
}

// TestMirrorParseMultiArch
func TestMirrorParseMultiArch(t *testing.T) {
	res, platforms, err := parseMultiArch("system")
	assert.NoError(t, err)
	assert.Equal(t, copy.ImageListSelection(0), res)
	assert.Nil(t, platforms)

	res, platforms, err = parseMultiArch("all")
	assert.NoError(t, err)
	assert.Equal(t, copy.ImageListSelection(1), res)
	assert.Nil(t, platforms)

	res, platforms, err = parseMultiArch("index-only")
	assert.NoError(t, err)
	assert.Equal(t, copy.ImageListSelection(2), res)
	assert.Nil(t, platforms)

	// Platform strings are no longer handled by parseMultiArch — they go through
	// determinePlatformSelection via InstancePlatforms. Any unknown string is an error.
	_, _, err = parseMultiArch("linux/amd64,linux/arm64")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown multi-arch option")

	_, _, err = parseMultiArch("other")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown multi-arch option")
}

func TestDeterminePlatformSelection(t *testing.T) {
	t.Run("Priority 1: valid InstancePlatforms returns CopySpecificImages", func(t *testing.T) {
		opts := &CopyOptions{InstancePlatforms: []string{"linux/amd64", "linux/arm64"}}
		sel, platforms, err := determinePlatformSelection(opts)
		assert.NoError(t, err)
		assert.Equal(t, copy.CopySpecificImages, sel)
		assert.Equal(t, []copy.InstancePlatformFilter{
			{OS: "linux", Architecture: "amd64"},
			{OS: "linux", Architecture: "arm64"},
		}, platforms)
	})

	t.Run("Priority 1: invalid platform string returns error", func(t *testing.T) {
		opts := &CopyOptions{InstancePlatforms: []string{"invalid"}}
		_, _, err := determinePlatformSelection(opts)
		assert.Error(t, err)
	})

	t.Run("Priority 2: MultiArch=all returns CopyAllImages", func(t *testing.T) {
		opts := &CopyOptions{MultiArch: "all"}
		sel, platforms, err := determinePlatformSelection(opts)
		assert.NoError(t, err)
		assert.Equal(t, copy.CopyAllImages, sel)
		assert.Nil(t, platforms)
	})

	t.Run("Priority 2: MultiArch=system returns CopySystemImage", func(t *testing.T) {
		opts := &CopyOptions{MultiArch: "system"}
		sel, platforms, err := determinePlatformSelection(opts)
		assert.NoError(t, err)
		assert.Equal(t, copy.CopySystemImage, sel)
		assert.Nil(t, platforms)
	})

	t.Run("Priority 2: All=true returns CopyAllImages", func(t *testing.T) {
		opts := &CopyOptions{All: true}
		sel, platforms, err := determinePlatformSelection(opts)
		assert.NoError(t, err)
		assert.Equal(t, copy.CopyAllImages, sel)
		assert.Nil(t, platforms)
	})

	t.Run("Priority 2: MultiArch and All together return error", func(t *testing.T) {
		opts := &CopyOptions{MultiArch: "all", All: true}
		_, _, err := determinePlatformSelection(opts)
		assert.Error(t, err)
	})

	t.Run("empty options returns CopySystemImage", func(t *testing.T) {
		opts := &CopyOptions{}
		sel, platforms, err := determinePlatformSelection(opts)
		assert.NoError(t, err)
		assert.Equal(t, copy.CopySystemImage, sel)
		assert.Nil(t, platforms)
	})
}

func TestIsErrorRetryable(t *testing.T) {
	tooManyRequests := docker.UnexpectedHTTPStatusError{StatusCode: http.StatusTooManyRequests}

	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "nil error is not retryable",
			err:       nil,
			retryable: false,
		},
		{
			name:      "429 too many requests is retryable",
			err:       tooManyRequests,
			retryable: true,
		},
		{
			name:      "wrapped 429 too many requests is retryable",
			err:       fmt.Errorf("wrapped: %w", tooManyRequests),
			retryable: true,
		},
		{
			name:      "500 internal server error is retryable",
			err:       docker.UnexpectedHTTPStatusError{StatusCode: http.StatusInternalServerError},
			retryable: true,
		},
		{
			name:      "502 bad gateway is retryable",
			err:       docker.UnexpectedHTTPStatusError{StatusCode: http.StatusBadGateway},
			retryable: true,
		},
		{
			name:      "504 gateway timeout is retryable",
			err:       docker.UnexpectedHTTPStatusError{StatusCode: http.StatusGatewayTimeout},
			retryable: true,
		},
		{
			name:      "400 bad request is not retryable",
			err:       docker.UnexpectedHTTPStatusError{StatusCode: http.StatusBadRequest},
			retryable: false,
		},
		{
			name:      "401 unauthorized is not retryable",
			err:       docker.UnexpectedHTTPStatusError{StatusCode: http.StatusUnauthorized},
			retryable: false,
		},
		{
			name:      "404 not found is not retryable",
			err:       docker.UnexpectedHTTPStatusError{StatusCode: http.StatusNotFound},
			retryable: false,
		},
		{
			name:      "505 http version not supported is not retryable",
			err:       docker.UnexpectedHTTPStatusError{StatusCode: http.StatusHTTPVersionNotSupported},
			retryable: false,
		},
		{
			name:      "context deadline exceeded is retryable",
			err:       context.DeadlineExceeded,
			retryable: true,
		},
		{
			name:      "context canceled is not retryable",
			err:       context.Canceled,
			retryable: false,
		},
		{
			name:      "bare ErrTooManyRequests sentinel is retryable",
			err:       docker.ErrTooManyRequests,
			retryable: true,
		},
		{
			name:      "wrapped ErrTooManyRequests sentinel is retryable",
			err:       fmt.Errorf("wrapped: %w", docker.ErrTooManyRequests),
			retryable: true,
		},
		{
			// Delegated to go.podman.io/common, which retries every errcode
			// except unauthorized/denied/nameunknown/manifestunknown.
			name:      "errcode TOOMANYREQUESTS is retryable",
			err:       errcode.Error{Code: errcode.ErrorCodeTooManyRequests, Message: "Rate exceeded"},
			retryable: true,
		},
		{
			name:      "errcode MANIFESTUNKNOWN is not retryable",
			err:       errcode.Error{Code: errcodev2.ErrorCodeManifestUnknown},
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.retryable, IsErrorRetryable(tt.err))
		})
	}
}

func TestRetryOptionsFrom(t *testing.T) {
	t.Run("nil RetryOpts yields usable options rather than a nil dereference", func(t *testing.T) {
		ro := retryOptionsFrom(&CopyOptions{})
		require.NotNil(t, ro)
		assert.NotNil(t, ro.IsErrorRetryable)
		assert.Equal(t, 0, ro.MaxRetry)
	})

	t.Run("caller options are copied, not mutated", func(t *testing.T) {
		shared := &retry.Options{MaxRetry: 5, Delay: 2 * time.Second}
		ro := retryOptionsFrom(&CopyOptions{RetryOpts: shared})
		require.NotNil(t, ro)
		assert.NotSame(t, shared, ro)
		assert.Equal(t, 5, ro.MaxRetry)
		assert.Equal(t, 2*time.Second, ro.Delay)
		assert.NotNil(t, ro.IsErrorRetryable)
		assert.Nil(t, shared.IsErrorRetryable, "opts.RetryOpts is shared across images and must not be mutated")
	})

	t.Run("installed classifier retries a 429", func(t *testing.T) {
		ro := retryOptionsFrom(&CopyOptions{RetryOpts: &retry.Options{MaxRetry: 5}})
		require.NotNil(t, ro.IsErrorRetryable)
		assert.True(t, ro.IsErrorRetryable(docker.UnexpectedHTTPStatusError{StatusCode: http.StatusTooManyRequests}))
		assert.True(t, ro.IsErrorRetryable(docker.ErrTooManyRequests))
	})
}

type (
	mockMirrorCopy   struct{}
	mockMirrorDelete struct{}
)

func (o *mockMirrorCopy) CopyImage(ctx context.Context, pc *signature.PolicyContext, destRef, srcRef types.ImageReference, opts *copy.Options) ([]byte, error) {
	return []byte("test"), nil
}

func (o *mockMirrorDelete) DeleteImage(ctx context.Context, dest string, opts *CopyOptions) error {
	return nil
}
