package archive

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/history"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type (
	mockBlobGatherer struct{}
	mockHistory      struct{}
)

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
	"working-dir-fake/hold-release/ocp-release/v4.13.10/release-manifests/0000_50_installer_coreos-bootimages.yaml",
	"working-dir-fake/hold-release/ocp-release/v4.13.9/release-manifests/0000_50_installer_coreos-bootimages.yaml",
	"working-dir-fake/hold-release/ocp-release/4.14.1-x86_64/release-manifests/image-references",
	"working-dir-fake/hold-release/ocp-release/4.14.1-x86_64/release-manifests/release-metadata",
	"working-dir-fake/hold-release/ocp-release/4.14.1-x86_64/release-manifests/0000_50_installer_coreos-bootimages.yaml",
	"working-dir-fake/hold-release/cincinnati-graph-data/amd64-stable-4.13.json",
	"working-dir-fake/hold-operator/redhat-operator-index/v4.14/configs/node-observability-operator/catalog.json",
}

func TestArchive_BuildArchive(t *testing.T) {
	t.Run("use strict adder: pass", func(t *testing.T) {
		// Create a temporary test folder
		testFolder := t.TempDir()
		ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, false)
		if err != nil {
			t.Fatal(err)
		}

		images := []v2alpha1.CopyImageSchema{
			{
				Source:      consts.DockerProtocol + "registry.redhat.io/ubi8/ubi:latest",
				Destination: consts.DockerProtocol + "localhost:5000/cfe969/ubi8/ubi:latest",
				Origin:      consts.DockerProtocol + "registry.redhat.io/ubi8/ubi:latest",
			},
		}
		err = ma.BuildArchive(context.Background(), v2alpha1.CollectorSchema{AllImages: images}, nil)
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
		ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, true)
		if err != nil {
			t.Fatal(err)
		}

		images := []v2alpha1.CopyImageSchema{
			{
				Source:      consts.DockerProtocol + "registry.redhat.io/ubi8/ubi:latest",
				Destination: consts.DockerProtocol + "localhost:5000/cfe969/ubi8/ubi:latest",
				Origin:      consts.DockerProtocol + "registry.redhat.io/ubi8/ubi:latest",
			},
		}
		err = ma.BuildArchive(context.Background(), v2alpha1.CollectorSchema{AllImages: images}, nil)
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
	ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, false)
	if err != nil {
		t.Fatal(err)
	}

	images := []v2alpha1.CopyImageSchema{
		{
			Source:      consts.DockerProtocol + "registry.redhat.io/ubi8/ubi:latest",
			Destination: consts.DockerProtocol + "localhost:5000/cfe969/ubi8/ubi:latest",
			Origin:      consts.DockerProtocol + "registry.redhat.io/ubi8/ubi:latest",
		},
	}
	// force error for addAllFolder
	ma.cacheDir = "none"
	ma.workingDir = consts.TestFolder + "working-dir-fake"

	err = ma.BuildArchive(context.Background(), v2alpha1.CollectorSchema{AllImages: images}, nil)
	if err == nil {
		t.Fatal("should fail")
	}
}

func TestArchive_WorkingDirError(t *testing.T) {
	// Create a temporary test folder
	testFolder := t.TempDir()
	ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, false)
	if err != nil {
		t.Fatal(err)
	}

	images := []v2alpha1.CopyImageSchema{
		{
			Source:      consts.DockerProtocol + "registry.redhat.io/ubi8/ubi:latest",
			Destination: consts.DockerProtocol + "localhost:5000/cfe969/ubi8/ubi:latest",
			Origin:      consts.DockerProtocol + "registry.redhat.io/ubi8/ubi:latest",
		},
	}
	// force error for addAllFolder
	ma.cacheDir = consts.TestFolder + "cache-fake"
	ma.workingDir = "none"

	err = ma.BuildArchive(context.Background(), v2alpha1.CollectorSchema{AllImages: images}, nil)
	if err == nil {
		t.Fatal("should fail")
	}
}

func TestArchive_FileError(t *testing.T) {
	// Create a temporary test folder
	testFolder := t.TempDir()
	ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, false)
	if err != nil {
		t.Fatal(err)
	}

	images := []v2alpha1.CopyImageSchema{
		{
			Source:      consts.DockerProtocol + "registry.redhat.io/ubi8/ubi:latest",
			Destination: consts.DockerProtocol + "localhost:5000/cfe969/ubi8/ubi:latest",
			Origin:      consts.DockerProtocol + "registry.redhat.io/ubi8/ubi:latest",
		},
	}
	// force error for addFile
	ma.iscPath = "none"

	err = ma.BuildArchive(context.Background(), v2alpha1.CollectorSchema{AllImages: images}, nil)
	if err == nil {
		t.Fatal("should fail")
	}
}

func TestArchive_AddBlobsDiff(t *testing.T) {
	// Create a temporary test folder
	testFolder := t.TempDir()
	ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, false)
	if err != nil {
		t.Fatal(err)
	}

	collectedBlobs := sets.New(
		"sha256:2e39d55595ea56337b5b788e96e6afdec3db09d2759d903cbe120468187c4644",
		"sha256:94343313ec1512ab02267e4bc3ce09eecb01fda5bf26c56e2f028ecc72e80b18",
		"sha256:4c0f6aace7053de3b9c1476b33c9a763e45a099c8c7ae9117773c9a8e5b8506b",
		"sha256:cfaa7496ab546c36ab14859f93fbd2d8a3588b344b18d5fbe74dd834e4a6f7eb",
		"sha256:53c56977ccd20c0d87df0ad52036c55b27201e1a63874c2644383d0c532f5aee",
		"sha256:e1bb0572465a9e03d7af5024abb36d7227b5bf133c448b54656d908982127874",
		"sha256:6e1ac33d11e06db5e850fec4a1ec07f6c2ab15f130c2fdf0f9d0d0a5c83651e7",
		"sha256:f992cb38fce665360a4d07f6f78db864a1f6e20a7ad304219f7f81d7fe608d97",
		"sha256:6376a0276facf61d87fdf7c6f21d761ee25ba8ceba934d64752d43e84fe0cb98",
		"sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
		"sha256:9b6fa335dba394d437930ad79e308e01da4f624328e49d00c0ff44775d2e4769",
		"sha256:e6c589cf5f402a60a83a01653304d7a8dcdd47b93a395a797b5622a18904bd66",
	)

	historyBlobs := sets.New(
		"sha256:2e39d55595ea56337b5b788e96e6afdec3db09d2759d903cbe120468187c4644",
		"sha256:94343313ec1512ab02267e4bc3ce09eecb01fda5bf26c56e2f028ecc72e80b18",
		"sha256:4c0f6aace7053de3b9c1476b33c9a763e45a099c8c7ae9117773c9a8e5b8506b",
		"sha256:cfaa7496ab546c36ab14859f93fbd2d8a3588b344b18d5fbe74dd834e4a6f7eb",
		"sha256:53c56977ccd20c0d87df0ad52036c55b27201e1a63874c2644383d0c532f5aee",
	)

	expectedAddedBlobs := sets.New(
		"sha256:e1bb0572465a9e03d7af5024abb36d7227b5bf133c448b54656d908982127874",
		"sha256:6e1ac33d11e06db5e850fec4a1ec07f6c2ab15f130c2fdf0f9d0d0a5c83651e7",
		"sha256:f992cb38fce665360a4d07f6f78db864a1f6e20a7ad304219f7f81d7fe608d97",
		"sha256:6376a0276facf61d87fdf7c6f21d761ee25ba8ceba934d64752d43e84fe0cb98",
		"sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
		"sha256:9b6fa335dba394d437930ad79e308e01da4f624328e49d00c0ff44775d2e4769",
		"sha256:e6c589cf5f402a60a83a01653304d7a8dcdd47b93a395a797b5622a18904bd66",
	)
	actualAddedBlobs, err := ma.addBlobsDiff(collectedBlobs, historyBlobs, sets.New[string]())
	assert.NoError(t, err, "call addBlobsDiff should not return an error")
	assert.Equal(t, sets.List(expectedAddedBlobs), sets.List(actualAddedBlobs))
}

func TestArchive_RemovePastMirrors(t *testing.T) {
	type testCase struct {
		caseName      string
		destination   string
		expectedError string
	}

	testFolder1 := t.TempDir()
	testFolder2 := t.TempDir()

	file, err := os.Create(filepath.Join(testFolder2, "mirror_000001.tar"))
	if err != nil {
		t.Fatalf("should not fail : %v", err)
	}
	defer file.Close()

	testCases := []testCase{
		{
			caseName:      "destination exists, nothing to remove: pass",
			destination:   testFolder1,
			expectedError: "",
		},
		{
			caseName:      "destination doesnt exist: pass",
			destination:   "testFolder3",
			expectedError: "",
		},
		{
			caseName:      "destination exists and contains files: pass",
			destination:   testFolder2,
			expectedError: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			err := RemovePastArchives(tc.destination)
			if err != nil {
				assert.Equal(t, tc.expectedError, err.Error())
			} else if tc.expectedError != "" {
				t.Fatal("should fail but passed instead")
			}
			if _, err := os.Stat(tc.destination); err == nil {
				entries, err := os.ReadDir(tc.destination)
				if err != nil {
					t.Fatal("should not fail")
				}
				assert.Empty(t, entries)
			}
		})
	}
}

func TestHandleSignatureErrors(t *testing.T) {
	testCases := []struct {
		name     string
		img      v2alpha1.CopyImageSchema
		inputErr error
		want     error
	}{
		{
			name: "nil error should return nil",
			img: v2alpha1.CopyImageSchema{
				Type: v2alpha1.TypeOperatorCatalog,
			},
			inputErr: nil,
			want:     nil,
		},
		{
			name: "non-signature error should return nil",
			img: v2alpha1.CopyImageSchema{
				Type: v2alpha1.TypeOperatorCatalog,
			},
			inputErr: fmt.Errorf("some other error"),
			want:     nil,
		},
		{
			name: "original operator catalog image should return ArchiveError{OperatorErr: signature error}",
			img: v2alpha1.CopyImageSchema{
				Type: v2alpha1.TypeOperatorCatalog,
			},
			inputErr: &SignatureBlobGathererError{SigError: fmt.Errorf("signature error")},
			want:     &ArchiveError{OperatorErr: fmt.Errorf("signature error")},
		},
		{
			name: "rebuilt operator catalog image should return nil",
			img: v2alpha1.CopyImageSchema{
				Type:       v2alpha1.TypeOperatorCatalog,
				RebuiltTag: "rebuilt-catalog",
			},
			inputErr: &SignatureBlobGathererError{SigError: fmt.Errorf("signature error")},
			want:     nil,
		},
		{
			name: "release image should return ArchiveError{ReleaseErr: signature error}",
			img: v2alpha1.CopyImageSchema{
				Type: v2alpha1.TypeOCPRelease,
			},
			inputErr: &SignatureBlobGathererError{SigError: fmt.Errorf("signature error")},
			want:     &ArchiveError{ReleaseErr: fmt.Errorf("signature error")},
		},
		{
			name: "graph image should return nil",
			img: v2alpha1.CopyImageSchema{
				Type: v2alpha1.TypeCincinnatiGraph,
			},
			inputErr: &SignatureBlobGathererError{SigError: fmt.Errorf("signature error")},
			want:     nil,
		},
		{
			name: "additional image should return ArchiveError{AdditionalImgErr: signature error} signature error",
			img: v2alpha1.CopyImageSchema{
				Type: v2alpha1.TypeGeneric,
			},
			inputErr: &SignatureBlobGathererError{SigError: fmt.Errorf("signature error")},
			want:     &ArchiveError{AdditionalImgErr: fmt.Errorf("signature error")},
		},
		{
			name: "helm image should return ArchiveError{AdditionalImgErr: signature error} signature errorsignature error",
			img: v2alpha1.CopyImageSchema{
				Type: v2alpha1.TypeHelmImage,
			},
			inputErr: &SignatureBlobGathererError{SigError: fmt.Errorf("signature error")},
			want:     &ArchiveError{HelmErr: fmt.Errorf("signature error")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := handleSignatureErrors(tc.img, tc.inputErr)
			if tc.want == nil {
				assert.Nil(t, got)
				return
			}
			assert.Equal(t, tc.want.Error(), got.Error())

			var wantSigErr *ArchiveError
			var gotSigErr *ArchiveError
			if !errors.As(tc.want, &wantSigErr) || !errors.As(got, &gotSigErr) {
				t.Fatal("error type mismatch")
			}

			switch {
			case tc.img.Type.IsOperator():
				assert.Equal(t, wantSigErr.OperatorErr, gotSigErr.OperatorErr)
			case tc.img.Type.IsRelease():
				assert.Equal(t, wantSigErr.ReleaseErr, gotSigErr.ReleaseErr)
			case tc.img.Type.IsAdditionalImage():
				assert.Equal(t, wantSigErr.AdditionalImgErr, gotSigErr.AdditionalImgErr)
			case tc.img.Type.IsHelmImage():
				assert.Equal(t, wantSigErr.HelmErr, gotSigErr.HelmErr)
			}
		})
	}
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

func TestArchive_AddImagesDiff_IgnoreReleaseSignature(t *testing.T) {
	// sigErr simulates a missing cosign signature for an unsigned release image
	// (e.g. a CI nightly build that has no cosign .sig blob in the local cache).
	sigErr := &SignatureBlobGathererError{SigError: fmt.Errorf("manifest unknown")}

	releaseImg := v2alpha1.CopyImageSchema{
		Source:      consts.DockerProtocol + "registry.build05.ci.openshift.org/ci-op/release:latest",
		Destination: consts.DockerProtocol + "localhost:5000/openshift/release:latest",
		Type:        v2alpha1.TypeOCPRelease,
	}
	schema := v2alpha1.CollectorSchema{AllImages: []v2alpha1.CopyImageSchema{releaseImg}}

	t.Run("ignoreReleaseSignature=false: release sig error is fatal", func(t *testing.T) {
		testFolder := t.TempDir()
		ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, false)
		if err != nil {
			t.Fatal(err)
		}
		ma.blobGatherer = funcBlobGatherer(func(_ context.Context, _ string, _ []string) (sets.Set[string], error) {
			return sets.New[string](), sigErr
		})
		ma.ignoreReleaseSignature = false

		_, err = ma.addImagesDiff(context.Background(), schema, sets.New[string]())
		var archErr *ArchiveError
		assert.ErrorAs(t, err, &archErr, "error should be ArchiveError with ReleaseErr set")
		assert.NotNil(t, archErr.ReleaseErr)
	})

	t.Run("ignoreReleaseSignature=true: release sig error is non-fatal", func(t *testing.T) {
		testFolder := t.TempDir()
		ma, err := newMirrorArchiveWithMocks(testFolder, defaultSegSize*segMultiplier, false)
		if err != nil {
			t.Fatal(err)
		}
		ma.blobGatherer = funcBlobGatherer(func(_ context.Context, _ string, _ []string) (sets.Set[string], error) {
			return sets.New[string](), sigErr
		})
		ma.ignoreReleaseSignature = true

		_, err = ma.addImagesDiff(context.Background(), schema, sets.New[string]())
		assert.NoError(t, err, "release signature error should be non-fatal when ignoreReleaseSignature=true")
	})
}

// funcBlobGatherer is a BlobsGatherer backed by a plain function, for use in tests.
type funcBlobGatherer func(ctx context.Context, imgRef string, allowedPlatforms []string) (sets.Set[string], error)

func (f funcBlobGatherer) GatherBlobs(ctx context.Context, imgRef string, allowedPlatforms []string) (sets.Set[string], error) {
	return f(ctx, imgRef, allowedPlatforms)
}

// //////     Mocks       ////////

func newMirrorArchiveWithMocks(testFolder string, maxArchiveSize int64, permissive bool) (*MirrorArchive, error) {
	global := &mirror.GlobalOptions{
		SecurePolicy:    false,
		Force:           true,
		WorkingDir:      "tests",
		StrictArchiving: permissive,
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
	cfg := consts.TestFolder + "isc.yaml"
	var ma *MirrorArchive
	ma, err := NewMirrorArchive(&opts, testFolder, cfg, consts.TestFolder+"working-dir-fake", consts.TestFolder+"cache-fake", 0, clog.New("trace"))
	if err != nil {
		return &MirrorArchive{}, err
	}
	if err := ma.createTarball(); err != nil {
		return &MirrorArchive{}, err
	}
	ma, err = ma.WithFakes(maxArchiveSize)
	return ma, err
}

func (ma *MirrorArchive) WithFakes(maxArchiveSize int64) (*MirrorArchive, error) {
	ma.blobGatherer = mockBlobGatherer{}
	ma.history = mockHistory{}
	return ma, nil
}

func (mbg mockBlobGatherer) GatherBlobs(ctx context.Context, imgRef string, allowedPlatforms []string) (sets.Set[string], error) {
	blobs := sets.New(
		"sha256:2e39d55595ea56337b5b788e96e6afdec3db09d2759d903cbe120468187c4644",
		"sha256:94343313ec1512ab02267e4bc3ce09eecb01fda5bf26c56e2f028ecc72e80b18",
		"sha256:4c0f6aace7053de3b9c1476b33c9a763e45a099c8c7ae9117773c9a8e5b8506b",
		"sha256:cfaa7496ab546c36ab14859f93fbd2d8a3588b344b18d5fbe74dd834e4a6f7eb",
		"sha256:53c56977ccd20c0d87df0ad52036c55b27201e1a63874c2644383d0c532f5aee",
		"sha256:e1bb0572465a9e03d7af5024abb36d7227b5bf133c448b54656d908982127874",
		"sha256:6e1ac33d11e06db5e850fec4a1ec07f6c2ab15f130c2fdf0f9d0d0a5c83651e7",
		"sha256:f992cb38fce665360a4d07f6f78db864a1f6e20a7ad304219f7f81d7fe608d97",
		"sha256:6376a0276facf61d87fdf7c6f21d761ee25ba8ceba934d64752d43e84fe0cb98",
		"sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
		"sha256:9b6fa335dba394d437930ad79e308e01da4f624328e49d00c0ff44775d2e4769",
		"sha256:e6c589cf5f402a60a83a01653304d7a8dcdd47b93a395a797b5622a18904bd66",
	)
	return blobs, nil
}

func (m mockHistory) Read() (sets.Set[string], error) {
	historyMap := sets.New(
		"sha256:2e39d55595ea56337b5b788e96e6afdec3db09d2759d903cbe120468187c4644",
		"sha256:94343313ec1512ab02267e4bc3ce09eecb01fda5bf26c56e2f028ecc72e80b18",
		"sha256:4c0f6aace7053de3b9c1476b33c9a763e45a099c8c7ae9117773c9a8e5b8506b",
		"sha256:cfaa7496ab546c36ab14859f93fbd2d8a3588b344b18d5fbe74dd834e4a6f7eb",
		"sha256:53c56977ccd20c0d87df0ad52036c55b27201e1a63874c2644383d0c532f5aee",
	)
	return historyMap, nil
}

func (m mockHistory) Append(inputMap sets.Set[string]) (sets.Set[string], error) {
	historyMap := sets.New(
		"sha256:2e39d55595ea56337b5b788e96e6afdec3db09d2759d903cbe120468187c4644",
		"sha256:94343313ec1512ab02267e4bc3ce09eecb01fda5bf26c56e2f028ecc72e80b18",
		"sha256:4c0f6aace7053de3b9c1476b33c9a763e45a099c8c7ae9117773c9a8e5b8506b",
		"sha256:cfaa7496ab546c36ab14859f93fbd2d8a3588b344b18d5fbe74dd834e4a6f7eb",
		"sha256:53c56977ccd20c0d87df0ad52036c55b27201e1a63874c2644383d0c532f5aee",
	)
	historyMap = historyMap.Union(inputMap)
	return historyMap, nil
}

func TestArchiveSizeEstimateMatchesArchive(t *testing.T) {
	root := t.TempDir()
	working := filepath.Join(root, "working-dir")
	cache := filepath.Join(root, "cache")
	write := func(path string, n int64) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", int(n))), 0600))
	}
	write(filepath.Join(working, "operator-catalogs", "catalog", "config.json"), 713)
	write(filepath.Join(root, "isc.yaml"), 33)
	old := types.BlobInfo{Digest: digest.FromString("old"), Size: 600}
	newBlob := types.BlobInfo{Digest: digest.FromString("new"), Size: 900}
	for _, blob := range []types.BlobInfo{old, newBlob} {
		path, err := BlobPath(cache, blob.Digest)
		require.NoError(t, err)
		write(path, blob.Size)
	}
	log := clog.New("error")
	h, err := history.NewHistory(working, time.Time{}, log, history.OSFileCreator{})
	require.NoError(t, err)
	_, err = h.Append(sets.New(old.Digest.String()))
	require.NoError(t, err)
	opts := &mirror.CopyOptions{Global: &mirror.GlobalOptions{WorkingDir: working, ConfigPath: filepath.Join(root, "isc.yaml")}}
	img := v2alpha1.CopyImageSchema{Source: "docker://source.example/ns/image:latest", Destination: "docker://localhost:55000/ns/image:latest"}
	schema := v2alpha1.CollectorSchema{AllImages: []v2alpha1.CopyImageSchema{img}}
	blobs := map[string][]types.BlobInfo{img.Source: {old, newBlob}}
	metadata, err := CacheMetadata(schema.AllImages, blobs)
	require.NoError(t, err)
	for path, size := range metadata {
		write(filepath.Join(cache, path), size)
	}
	extraLink := filepath.Join(cacheRepositoriesDir, "image", "_layers", "sha256", newBlob.Digest.Encoded(), "link")
	metadata[extraLink] = int64(len(newBlob.Digest.String()))
	estimated, err := EstimateSize(context.Background(), opts, cache, schema, blobs, metadata, 0, log)
	require.NoError(t, err)
	for name, laterBlobs := range map[string][]types.BlobInfo{"inspected source": {old}, "failed inspection": nil} {
		t.Run("repeated destination/"+name, func(t *testing.T) {
			repeated := schema
			repeated.AllImages = []v2alpha1.CopyImageSchema{img,
				{Source: "docker://source.example/ns/middle:latest", Destination: "docker://localhost:55000/ns/middle:latest"},
				{Source: "docker://source.example/ns/later:latest", Destination: img.Destination}}
			inspected := maps.Clone(blobs)
			inspected[repeated.AllImages[2].Source] = laterBlobs
			got, err := EstimateSize(context.Background(), opts, cache, repeated, inspected, metadata, 0, log)
			require.NoError(t, err)
			require.Equal(t, estimated, got, "later entries must preserve the first source's new blobs")
		})
	}
	write(filepath.Join(cache, extraLink), metadata[extraLink])
	// The history baseline is read only; the new blob has not been appended.
	previous, err := h.Read()
	require.NoError(t, err)
	require.False(t, previous.Has(newBlob.Digest.String()))
	archiver, err := NewMirrorArchive(opts, filepath.Join(root, "archives"), opts.Global.ConfigPath, working, cache, 0, log)
	require.NoError(t, err)
	archiver.blobGatherer = funcBlobGatherer(func(context.Context, string, []string) (sets.Set[string], error) {
		return sets.New(old.Digest.String(), newBlob.Digest.String()), nil
	})
	require.NoError(t, archiver.BuildArchive(context.Background(), schema, nil))
	require.Equal(t, archiveBytes(t, filepath.Join(root, "archives")), estimated)

}

func TestSizeAdderMatchesChunkWriters(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(map[bool]string{false: "permissive", true: "strict"}[strict], func(t *testing.T) {
			root := t.TempDir()
			log := clog.New("error")
			var actual archiveAdder
			var err error
			if strict {
				actual, err = newStrictAdder(1000, root, log)
			} else {
				actual, err = newPermissiveAdder(1000, root, log)
			}
			require.NoError(t, err)
			estimate := &sizeAdder{maxSize: 1000, strict: strict, total: 1024, files: map[string]bool{}}
			for i, size := range []int{0, 510, 515, 1600} {
				name := string(rune('a' + i))
				file := filepath.Join(t.TempDir(), "data")
				require.NoError(t, os.WriteFile(file, make([]byte, size), 0600))
				err := actual.addFile(file, name)
				estimateErr := estimate.addFile(file, name)
				if strict && size > 1000 {
					require.Error(t, err)
					require.Error(t, estimateErr)
					continue
				}
				require.NoError(t, err)
				require.NoError(t, estimateErr)
			}
			require.NoError(t, actual.close())
			require.Equal(t, archiveBytes(t, root), estimate.total)
		})
	}
}

func TestSizeAdderPendingFolders(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "blobs", "first")
	neighbor := filepath.Join(root, "blobs", "first-other")
	pending := map[string]int64{
		filepath.Join(folder, "data"):   1,
		filepath.Join(folder, "extra"):  513,
		filepath.Join(neighbor, "data"): 1025,
	}
	counter := &sizeAdder{maxSize: 1024 * 1024, total: 1024, files: map[string]bool{},
		pending: pending, pendingPaths: slices.Sorted(maps.Keys(pending))}
	require.NoError(t, counter.addAllFolder(folder, root))
	require.Equal(t, map[string]bool{filepath.Join("blobs", "first", "data"): true,
		filepath.Join("blobs", "first", "extra"): true}, counter.files)
	firstSize := counter.total
	require.NoError(t, counter.addAllFolder(folder, root))
	require.Equal(t, firstSize, counter.total, "repeated folders must not be counted twice")
	require.NoError(t, counter.addAllFolder(neighbor, root))
	require.Len(t, counter.files, 3)
	require.Greater(t, counter.total, firstSize)
}

func archiveBytes(t *testing.T, directory string) int64 {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(directory, "mirror_*.tar"))
	require.NoError(t, err)
	var total int64
	for _, file := range files {
		info, err := os.Stat(file)
		require.NoError(t, err)
		total += info.Size()
	}
	return total
}
