package archive

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/history"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/stretchr/testify/assert"
)

type mockBlobGatherer struct{}
type MockFileCreator struct {
	Buffer *bytes.Buffer
}

type nopCloser struct {
	io.Writer
}

func newMirrorArchiveWithMocks(testFolder string) (MirrorArchive, error) {
	ctx := context.Background()
	global := &mirror.GlobalOptions{
		TlsVerify:    false,
		SecurePolicy: false,
		Force:        true,
		WorkingDir:   "tests",
	}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()
	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
	}
	cfg := "../../tests/isc.yaml"

	ma, err := NewMirrorArchive(ctx, &opts, testFolder, cfg, "../../tests/working-dir-fake", "../../tests/cache-fake", clog.New("trace"))
	if err != nil {
		return MirrorArchive{}, err
	}

	ma, err = ma.WithFakes()
	return ma, err
}
func TestArchive_BuildArchive(t *testing.T) {
	// Create a temporary test folder
	testFolder := t.TempDir()
	ma, err := newMirrorArchiveWithMocks(testFolder)
	if err != nil {
		t.Fatal(err)
	}
	defer ma.Close()
	defer os.RemoveAll(testFolder)

	images := []v1alpha3.CopyImageSchema{
		{
			Source:      "docker://registry.redhat.io/ubi8/ubi:latest",
			Destination: "docker://localhost:5000/cfe969/ubi8/ubi:latest",
			Origin:      "docker://registry.redhat.io/ubi8/ubi:latest",
		},
	}
	archName, err := ma.BuildArchive(images)
	if err != nil {
		t.Fatal(err)
	}
	assert.FileExists(t, archName, "archive should exist")
}

func TestArchive_AddBlobsDiff(t *testing.T) {
	// Create a temporary test folder
	testFolder := t.TempDir()
	ma, err := newMirrorArchiveWithMocks(testFolder)
	if err != nil {
		t.Fatal(err)
	}
	defer ma.Close()
	defer os.RemoveAll(testFolder)

	collectedBlobs := map[string]string{
		"sha256:2e39d55595ea56337b5b788e96e6afdec3db09d2759d903cbe120468187c4644": "",
		"sha256:94343313ec1512ab02267e4bc3ce09eecb01fda5bf26c56e2f028ecc72e80b18": "",
		"sha256:4c0f6aace7053de3b9c1476b33c9a763e45a099c8c7ae9117773c9a8e5b8506b": "",
		"sha256:cfaa7496ab546c36ab14859f93fbd2d8a3588b344b18d5fbe74dd834e4a6f7eb": "",
		"sha256:53c56977ccd20c0d87df0ad52036c55b27201e1a63874c2644383d0c532f5aee": "",
		"sha256:e1bb0572465a9e03d7af5024abb36d7227b5bf133c448b54656d908982127874": "",
		"sha256:6e1ac33d11e06db5e850fec4a1ec07f6c2ab15f130c2fdf0f9d0d0a5c83651e7": "",
		"sha256:f992cb38fce665360a4d07f6f78db864a1f6e20a7ad304219f7f81d7fe608d97": "",
		"sha256:6376a0276facf61d87fdf7c6f21d761ee25ba8ceba934d64752d43e84fe0cb98": "",
		"sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed": "",
		"sha256:9b6fa335dba394d437930ad79e308e01da4f624328e49d00c0ff44775d2e4769": "",
		"sha256:e6c589cf5f402a60a83a01653304d7a8dcdd47b93a395a797b5622a18904bd66": "",
	}

	historyBlobs := map[string]string{
		"sha256:2e39d55595ea56337b5b788e96e6afdec3db09d2759d903cbe120468187c4644": "",
		"sha256:94343313ec1512ab02267e4bc3ce09eecb01fda5bf26c56e2f028ecc72e80b18": "",
		"sha256:4c0f6aace7053de3b9c1476b33c9a763e45a099c8c7ae9117773c9a8e5b8506b": "",
		"sha256:cfaa7496ab546c36ab14859f93fbd2d8a3588b344b18d5fbe74dd834e4a6f7eb": "",
		"sha256:53c56977ccd20c0d87df0ad52036c55b27201e1a63874c2644383d0c532f5aee": "",
	}

	expectedAddedBlobs := map[string]string{
		"sha256:e1bb0572465a9e03d7af5024abb36d7227b5bf133c448b54656d908982127874": "",
		"sha256:6e1ac33d11e06db5e850fec4a1ec07f6c2ab15f130c2fdf0f9d0d0a5c83651e7": "",
		"sha256:f992cb38fce665360a4d07f6f78db864a1f6e20a7ad304219f7f81d7fe608d97": "",
		"sha256:6376a0276facf61d87fdf7c6f21d761ee25ba8ceba934d64752d43e84fe0cb98": "",
		"sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed": "",
		"sha256:9b6fa335dba394d437930ad79e308e01da4f624328e49d00c0ff44775d2e4769": "",
		"sha256:e6c589cf5f402a60a83a01653304d7a8dcdd47b93a395a797b5622a18904bd66": "",
	}
	actualAddedBlobs, err := ma.addBlobsDiff(collectedBlobs, historyBlobs)
	assert.NoError(t, err, "call addBlobsDiff should not return an error")
	assert.Equal(t, expectedAddedBlobs, actualAddedBlobs)

}

// //////     Mocks       ////////
func (ma MirrorArchive) WithFakes() (MirrorArchive, error) {
	ma.blobGatherer = mockBlobGatherer{}
	h, err := history.NewHistory(ma.workingDir, time.Time{}, clog.New("trace"), MockFileCreator{})
	ma.history = h
	return ma, err
}

func (mbg mockBlobGatherer) GatherBlobs(imgRef string) (map[string]string, error) {
	blobs := map[string]string{
		"sha256:2e39d55595ea56337b5b788e96e6afdec3db09d2759d903cbe120468187c4644": "",
		"sha256:94343313ec1512ab02267e4bc3ce09eecb01fda5bf26c56e2f028ecc72e80b18": "",
		"sha256:4c0f6aace7053de3b9c1476b33c9a763e45a099c8c7ae9117773c9a8e5b8506b": "",
		"sha256:cfaa7496ab546c36ab14859f93fbd2d8a3588b344b18d5fbe74dd834e4a6f7eb": "",
		"sha256:53c56977ccd20c0d87df0ad52036c55b27201e1a63874c2644383d0c532f5aee": "",
		"sha256:e1bb0572465a9e03d7af5024abb36d7227b5bf133c448b54656d908982127874": "",
		"sha256:6e1ac33d11e06db5e850fec4a1ec07f6c2ab15f130c2fdf0f9d0d0a5c83651e7": "",
		"sha256:f992cb38fce665360a4d07f6f78db864a1f6e20a7ad304219f7f81d7fe608d97": "",
		"sha256:6376a0276facf61d87fdf7c6f21d761ee25ba8ceba934d64752d43e84fe0cb98": "",
		"sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed": "",
		"sha256:9b6fa335dba394d437930ad79e308e01da4f624328e49d00c0ff44775d2e4769": "",
		"sha256:e6c589cf5f402a60a83a01653304d7a8dcdd47b93a395a797b5622a18904bd66": "",
	}
	return blobs, nil
}

func (m MockFileCreator) Create(name string) (io.WriteCloser, error) {
	m.Buffer = new(bytes.Buffer)
	return nopCloser{m.Buffer}, nil
}

func (nopCloser) Close() error { return nil }
