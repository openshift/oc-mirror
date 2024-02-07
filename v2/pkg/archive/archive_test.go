package archive

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/stretchr/testify/assert"
)

type mockBlobGatherer struct{}
type mockHistory struct{}

var expectedTarContents = []string{
	// is in history // "docker/registry/v2/blobs/sha256/2e/2e39d55595ea56337b5b788e96e6afdec3db09d2759d903cbe120468187c4644/data",
	// is in history // "docker/registry/v2/blobs/sha256/4c/4c0f6aace7053de3b9c1476b33c9a763e45a099c8c7ae9117773c9a8e5b8506b/data",
	// is in history // "docker/registry/v2/blobs/sha256/53/53c56977ccd20c0d87df0ad52036c55b27201e1a63874c2644383d0c532f5aee/data",
	"docker/registry/v2/blobs/sha256/63/6376a0276facf61d87fdf7c6f21d761ee25ba8ceba934d64752d43e84fe0cb98/data",
	"docker/registry/v2/blobs/sha256/6e/6e1ac33d11e06db5e850fec4a1ec07f6c2ab15f130c2fdf0f9d0d0a5c83651e7/data",
	// is in history // "docker/registry/v2/blobs/sha256/94/94343313ec1512ab02267e4bc3ce09eecb01fda5bf26c56e2f028ecc72e80b18/data",
	"docker/registry/v2/blobs/sha256/9b/9b6fa335dba394d437930ad79e308e01da4f624328e49d00c0ff44775d2e4769/data",
	// is in history // "docker/registry/v2/blobs/sha256/cf/cfaa7496ab546c36ab14859f93fbd2d8a3588b344b18d5fbe74dd834e4a6f7eb/data",
	"docker/registry/v2/blobs/sha256/db/db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed/data",
	"docker/registry/v2/blobs/sha256/e1/e1bb0572465a9e03d7af5024abb36d7227b5bf133c448b54656d908982127874/data",
	"docker/registry/v2/blobs/sha256/e6/e6c589cf5f402a60a83a01653304d7a8dcdd47b93a395a797b5622a18904bd66/data",
	"docker/registry/v2/blobs/sha256/f9/f992cb38fce665360a4d07f6f78db864a1f6e20a7ad304219f7f81d7fe608d97/data",
	"docker/registry/v2/repositories/ubi8/ubi/_layers/sha256/2e39d55595ea56337b5b788e96e6afdec3db09d2759d903cbe120468187c4644/link",
	"docker/registry/v2/repositories/ubi8/ubi/_layers/sha256/4c0f6aace7053de3b9c1476b33c9a763e45a099c8c7ae9117773c9a8e5b8506b/link",
	"docker/registry/v2/repositories/ubi8/ubi/_layers/sha256/53c56977ccd20c0d87df0ad52036c55b27201e1a63874c2644383d0c532f5aee/link",
	"docker/registry/v2/repositories/ubi8/ubi/_layers/sha256/6e1ac33d11e06db5e850fec4a1ec07f6c2ab15f130c2fdf0f9d0d0a5c83651e7/link",
	"docker/registry/v2/repositories/ubi8/ubi/_layers/sha256/94343313ec1512ab02267e4bc3ce09eecb01fda5bf26c56e2f028ecc72e80b18/link",
	"docker/registry/v2/repositories/ubi8/ubi/_layers/sha256/cfaa7496ab546c36ab14859f93fbd2d8a3588b344b18d5fbe74dd834e4a6f7eb/link",
	"docker/registry/v2/repositories/ubi8/ubi/_layers/sha256/e1bb0572465a9e03d7af5024abb36d7227b5bf133c448b54656d908982127874/link",
	"docker/registry/v2/repositories/ubi8/ubi/_layers/sha256/f992cb38fce665360a4d07f6f78db864a1f6e20a7ad304219f7f81d7fe608d97/link",
	"docker/registry/v2/repositories/ubi8/ubi/_manifests/revisions/sha256/6376a0276facf61d87fdf7c6f21d761ee25ba8ceba934d64752d43e84fe0cb98/link",
	"docker/registry/v2/repositories/ubi8/ubi/_manifests/revisions/sha256/9b6fa335dba394d437930ad79e308e01da4f624328e49d00c0ff44775d2e4769/link",
	"docker/registry/v2/repositories/ubi8/ubi/_manifests/revisions/sha256/db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed/link",
	"docker/registry/v2/repositories/ubi8/ubi/_manifests/revisions/sha256/e6c589cf5f402a60a83a01653304d7a8dcdd47b93a395a797b5622a18904bd66/link",
	"docker/registry/v2/repositories/ubi8/ubi/_manifests/tags/latest/current/link",
	"docker/registry/v2/repositories/ubi8/ubi/_manifests/tags/latest/index/sha256/db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed/link",
	"docker/registry/v2/repositories/ubi8/ubi/_uploads/97e2891e-4cb2-4289-a87a-e9b8cd006d20/data",
	"docker/registry/v2/repositories/ubi8/ubi/_uploads/97e2891e-4cb2-4289-a87a-e9b8cd006d20/hashstates/sha256/0",
	"docker/registry/v2/repositories/ubi8/ubi/_uploads/97e2891e-4cb2-4289-a87a-e9b8cd006d20/startedat",
	"isc",
	"working-dir-fake/hold-release/ocp-release/4.14.1-x86_64/release-manifests/image-references",
	"working-dir-fake/hold-release/ocp-release/4.14.1-x86_64/release-manifests/release-metadata",
	"working-dir-fake/release-filters/690d64d2ebd41f1834c80e632f041b5b",
	"working-dir-fake/hold-operator/redhat-operator-index/v4.14/configs/node-observability-operator/catalog.json",
}

func TestArchive_BuildArchive(t *testing.T) {
	t.Run("use strict adder: pass", func(t *testing.T) {
		// Create a temporary test folder
		testFolder := t.TempDir()
		defer os.RemoveAll(testFolder)
		ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, false)
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(testFolder)

		images := []v1alpha3.CopyImageSchema{
			{
				Source:      "docker://registry.redhat.io/ubi8/ubi:latest",
				Destination: "docker://localhost:5000/cfe969/ubi8/ubi:latest",
				Origin:      "docker://registry.redhat.io/ubi8/ubi:latest",
			},
		}
		err = ma.BuildArchive(context.Background(), images)
		if err != nil {
			t.Fatal(err)
		}
		archName := filepath.Join(testFolder, "mirror_000001.tar")
		assert.FileExists(t, archName, "archive should exist")
		assertContents(t, archName, expectedTarContents)
	})
	t.Run("use permissive adder: pass", func(t *testing.T) {
		// Create a temporary test folder
		testFolder := t.TempDir()
		defer os.RemoveAll(testFolder)
		ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, true)
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(testFolder)

		images := []v1alpha3.CopyImageSchema{
			{
				Source:      "docker://registry.redhat.io/ubi8/ubi:latest",
				Destination: "docker://localhost:5000/cfe969/ubi8/ubi:latest",
				Origin:      "docker://registry.redhat.io/ubi8/ubi:latest",
			},
		}
		err = ma.BuildArchive(context.Background(), images)
		if err != nil {
			t.Fatal(err)
		}
		archName := filepath.Join(testFolder, "mirror_000001.tar")
		assert.FileExists(t, archName, "archive should exist")
		assertContents(t, archName, expectedTarContents)
	})
}

func TestArchive_CacheDirError(t *testing.T) {
	// Create a temporary test folder
	testFolder := t.TempDir()
	defer os.RemoveAll(testFolder)
	ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, false)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testFolder)

	images := []v1alpha3.CopyImageSchema{
		{
			Source:      "docker://registry.redhat.io/ubi8/ubi:latest",
			Destination: "docker://localhost:5000/cfe969/ubi8/ubi:latest",
			Origin:      "docker://registry.redhat.io/ubi8/ubi:latest",
		},
	}
	// force error for addAllFolder
	ma.cacheDir = "none"
	ma.workingDir = "../../tests/working-dir-fake"

	err = ma.BuildArchive(context.Background(), images)
	if err == nil {
		t.Fatal("should fail")
	}
}

func TestArchive_WorkingDirError(t *testing.T) {
	// Create a temporary test folder
	testFolder := t.TempDir()
	defer os.RemoveAll(testFolder)
	ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, false)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testFolder)

	images := []v1alpha3.CopyImageSchema{
		{
			Source:      "docker://registry.redhat.io/ubi8/ubi:latest",
			Destination: "docker://localhost:5000/cfe969/ubi8/ubi:latest",
			Origin:      "docker://registry.redhat.io/ubi8/ubi:latest",
		},
	}
	// force error for addAllFolder
	ma.cacheDir = "../../tests/cache-fake"
	ma.workingDir = "none"

	err = ma.BuildArchive(context.Background(), images)
	if err == nil {
		t.Fatal("should fail")
	}
}

func TestArchive_FileError(t *testing.T) {
	// Create a temporary test folder
	testFolder := t.TempDir()
	defer os.RemoveAll(testFolder)
	ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, false)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testFolder)

	images := []v1alpha3.CopyImageSchema{
		{
			Source:      "docker://registry.redhat.io/ubi8/ubi:latest",
			Destination: "docker://localhost:5000/cfe969/ubi8/ubi:latest",
			Origin:      "docker://registry.redhat.io/ubi8/ubi:latest",
		},
	}
	// force error for addFile
	ma.iscPath = "none"

	err = ma.BuildArchive(context.Background(), images)
	if err == nil {
		t.Fatal("should fail")
	}
}

func TestArchive_AddBlobsDiff(t *testing.T) {
	// Create a temporary test folder
	testFolder := t.TempDir()
	defer os.RemoveAll(testFolder)
	ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, false)
	if err != nil {
		t.Fatal(err)
	}
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
	actualAddedBlobs, err := ma.addBlobsDiff(collectedBlobs, historyBlobs, map[string]string{})
	assert.NoError(t, err, "call addBlobsDiff should not return an error")
	assert.Equal(t, expectedAddedBlobs, actualAddedBlobs)

}

func assertContents(t *testing.T, archiveFile string, expectedTarContents []string) bool {
	actualTarContents := []string{}
	chunkFile, err := os.Open(archiveFile)
	if err != nil {
		t.Errorf("generated archive %s cannot be open", archiveFile)
	}
	reader := tar.NewReader(chunkFile)
	for {
		header, err := reader.Next()
		// break the infinite loop when EOF
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Errorf("error reading archive %s: %v", archiveFile, err)
		}

		if header == nil {
			continue
		}
		if header.Typeflag == tar.TypeReg {
			if strings.HasPrefix(header.Name, "isc") {
				actualTarContents = append(actualTarContents, "isc")
			} else {
				actualTarContents = append(actualTarContents, header.Name)
			}
		}
	}
	return assert.ElementsMatch(t, expectedTarContents, actualTarContents)

}

// //////     Mocks       ////////

func newMirrorArchiveWithMocks(testFolder string, maxArchiveSize int64, permissive bool) (*MirrorArchive, error) {
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
	var ma *MirrorArchive
	if permissive {
		m, err := NewPermissiveMirrorArchive(&opts, testFolder, cfg, "../../tests/working-dir-fake", "../../tests/cache-fake", 0, clog.New("trace"))
		if err != nil {
			return &MirrorArchive{}, err
		}
		ma = m
	} else {
		m, err := NewMirrorArchive(&opts, testFolder, cfg, "../../tests/working-dir-fake", "../../tests/cache-fake", 0, clog.New("trace"))
		if err != nil {
			return &MirrorArchive{}, err
		}
		ma = m
	}
	ma, err := ma.WithFakes(maxArchiveSize)
	return ma, err
}

func (ma *MirrorArchive) WithFakes(maxArchiveSize int64) (*MirrorArchive, error) {
	ma.blobGatherer = mockBlobGatherer{}
	ma.history = mockHistory{}
	return ma, nil
}

func (mbg mockBlobGatherer) GatherBlobs(ctx context.Context, imgRef string) (map[string]string, error) {
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

func (m mockHistory) Read() (map[string]string, error) {
	historyMap := map[string]string{

		"sha256:2e39d55595ea56337b5b788e96e6afdec3db09d2759d903cbe120468187c4644": "",
		"sha256:94343313ec1512ab02267e4bc3ce09eecb01fda5bf26c56e2f028ecc72e80b18": "",
		"sha256:4c0f6aace7053de3b9c1476b33c9a763e45a099c8c7ae9117773c9a8e5b8506b": "",
		"sha256:cfaa7496ab546c36ab14859f93fbd2d8a3588b344b18d5fbe74dd834e4a6f7eb": "",
		"sha256:53c56977ccd20c0d87df0ad52036c55b27201e1a63874c2644383d0c532f5aee": "",
	}
	return historyMap, nil
}

func (m mockHistory) Append(inputMap map[string]string) (map[string]string, error) {
	historyMap := map[string]string{

		"sha256:2e39d55595ea56337b5b788e96e6afdec3db09d2759d903cbe120468187c4644": "",
		"sha256:94343313ec1512ab02267e4bc3ce09eecb01fda5bf26c56e2f028ecc72e80b18": "",
		"sha256:4c0f6aace7053de3b9c1476b33c9a763e45a099c8c7ae9117773c9a8e5b8506b": "",
		"sha256:cfaa7496ab546c36ab14859f93fbd2d8a3588b344b18d5fbe74dd834e4a6f7eb": "",
		"sha256:53c56977ccd20c0d87df0ad52036c55b27201e1a63874c2644383d0c532f5aee": "",
	}
	for k, v := range inputMap {
		historyMap[k] = v
	}
	return historyMap, nil
}
