package mirror

import (
	"bufio"
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/types"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"
)

func TestMirrorCopy(t *testing.T) {

	testFolder := t.TempDir()
	testFile := testFolder + "/testDigest.txt"
	defer os.RemoveAll(testFolder)

	global := &GlobalOptions{TlsVerify: false, SecurePolicy: false}

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

	writer := bufio.NewWriter(os.Stdout)
	t.Run("Testing Mirror : copy should pass", func(t *testing.T) {
		err := m.Run(context.Background(), "docker://localhost.localdomain:5000/test", "oci:test", "copy", &opts, *writer)
		if err != nil {
			t.Fatal("should pass")
		}
	})

	t.Run("Testing Mirror : copy should fail", func(t *testing.T) {
		err := m.Run(context.Background(), "broken", "oci:test", "copy", &opts, *writer)
		assert.Equal(t, "Invalid source name broken: Invalid image name \"broken\", expected colon-separated transport:reference", err.Error())
	})

	t.Run("Testing Mirror : copy should fail", func(t *testing.T) {
		err := m.Run(context.Background(), "docker://localhost.localdomain:5000/tes", "broken", "copy", &opts, *writer)
		assert.Equal(t, "Invalid destination name broken: Invalid image name \"broken\", expected colon-separated transport:reference", err.Error())
	})

	opts.MultiArch = "other"
	t.Run("Testing Mirror : copy should fail", func(t *testing.T) {
		err := m.Run(context.Background(), "docker://localhost.localdomain:5000/tes", "oci:test", "copy", &opts, *writer)
		assert.Equal(t, "unknown multi-arch option \"other\". Choose one of the supported options: 'system', 'all', or 'index-only'", err.Error())
	})

	opts.All = true
	t.Run("Testing Mirror : copy should fail", func(t *testing.T) {
		err := m.Run(context.Background(), "docker://localhost.localdomain:5000/tes", "oci:test", "copy", &opts, *writer)
		assert.Equal(t, "Cannot use --all and --multi-arch flags together", err.Error())
	})

	opts.All = true
	opts.MultiArch = ""
	opts.EncryptionKeys = []string{"test"}
	opts.DecryptionKeys = []string{"test"}
	t.Run("Testing Mirror : copy should fail", func(t *testing.T) {
		err := m.Run(context.Background(), "docker://localhost.localdomain:5000/tes", "oci:test", "copy", &opts, *writer)
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
		err := m.Run(context.Background(), "docker://localhost.localdomain:5000/tes", "oci:test", "copy", &opts, *writer)
		assert.Equal(t, "Only one of --sign-by and sign-by-sigstore-private-key can be used with sign-passphrase-file", err.Error())
	})
}

func TestMirrorCheck(t *testing.T) {

	global := &GlobalOptions{TlsVerify: false, SecurePolicy: false}

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
	imageAbsolutePath, err := filepath.Abs("../../tests/albo-bundle-image")
	if err != nil {
		t.Fatal(err)
	}

	src := "dir://" + imageAbsolutePath
	dest := "docker://" + u.Host + "/albo-test:latest"
	err = New(NewMirrorCopy(), NewMirrorDelete()).Run(context.Background(), src, dest, "copy", &opts, *bufio.NewWriter(os.Stdout))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Testing Mirror : check should pass", func(t *testing.T) {
		_, err := m.Check(context.Background(), dest, &opts)
		if err != nil {
			t.Fatal("should pass")
		}
	})

	t.Run("Testing Mirror : check should pass", func(t *testing.T) {
		_, err := m.Check(context.Background(), "broken", &opts)
		assert.Equal(t, "invalid source name broken: Invalid image name \"broken\", expected colon-separated transport:reference", err.Error())
	})
}

// TestMirrorDelete
func TestMirrorDelete(t *testing.T) {

	global := &GlobalOptions{TlsVerify: false, SecurePolicy: false}

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
	}

	// Set up a fake registry.
	s := httptest.NewServer(registry.New())
	defer s.Close()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	imageAbsolutePath, err := filepath.Abs("../../tests/albo-bundle-image")
	if err != nil {
		t.Fatal(err)
	}

	src := "dir://" + imageAbsolutePath
	dest := "docker://" + u.Host + "/albo-test:latest"
	err = New(NewMirrorCopy(), NewMirrorDelete()).Run(context.Background(), src, dest, "copy", &opts, *bufio.NewWriter(os.Stdout))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Testing Mirror : delete should pass", func(t *testing.T) {
		err = New(NewMirrorCopy(), NewMirrorDelete()).Run(context.Background(), src, dest, "delete", &opts, *bufio.NewWriter(os.Stdout))
		if err != nil {
			t.Fatal("should not fail")
		}
	})

	t.Run("Testing Mirror : delete should fail", func(t *testing.T) {
		err = New(NewMirrorCopy(), NewMirrorDelete()).Run(context.Background(), src, "broken", "delete", &opts, *bufio.NewWriter(os.Stdout))
		assert.Equal(t, "Invalid source name broken: Invalid image name \"broken\", expected colon-separated transport:reference", err.Error())
	})

	t.Run("Testing Mirror : delete should fail", func(t *testing.T) {
		err = New(NewMirrorCopy(), NewMirrorDelete()).Run(context.Background(), src, src, "delete", &opts, *bufio.NewWriter(os.Stdout))
		assert.Equal(t, "Deleting images not implemented for dir: images", err.Error())
	})
}

// TestMirrorParseMultiArch
func TestMirrorParseMultiArch(t *testing.T) {
	res, _ := parseMultiArch("system")
	assert.Equal(t, copy.ImageListSelection(0), res)

	res, _ = parseMultiArch("all")
	assert.Equal(t, copy.ImageListSelection(1), res)

	res, _ = parseMultiArch("index-only")
	assert.Equal(t, copy.ImageListSelection(2), res)

	_, err := parseMultiArch("other")
	assert.Equal(t, "unknown multi-arch option \"other\". Choose one of the supported options: 'system', 'all', or 'index-only'", err.Error())
}

// mock

type mockMirrorCopy struct{}
type mockMirrorDelete struct{}

func (o *mockMirrorCopy) CopyImage(ctx context.Context, pc *signature.PolicyContext, destRef, srcRef types.ImageReference, opts *copy.Options) ([]byte, error) {
	return []byte("test"), nil
}

func (o *mockMirrorDelete) DeleteImage(ctx context.Context, dest string, opts *CopyOptions) error {
	return nil
}
