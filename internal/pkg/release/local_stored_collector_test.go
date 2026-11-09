package release

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/fake"
	"github.com/google/uuid"
	digest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/types"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/folder"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type MockMirror struct {
	Fail bool
}

type MockManifest struct {
	Log               clog.PluggableLoggerInterface
	FailImageIndex    bool
	FailImageManifest bool
	FailExtract       bool
}

type MockCincinnati struct {
	Config v2alpha1.ImageSetConfiguration
	Opts   mirror.CopyOptions
	Client Client
	Fail   bool
}

func TestReleaseLocalStoredCollector(t *testing.T) {
	log := clog.New("trace")
	tempDir := t.TempDir()

	// this test should cover over 80% M2D
	t.Run("Testing ReleaseImageCollector - Mirror to disk: should pass", func(t *testing.T) {
		ctx := context.Background()
		manifest := &MockManifest{Log: log}

		ex := setupCollector_MirrorToDisk(tempDir, log, manifest)

		err := copy.Copy(consts.TestFolder+"working-dir-fake/hold-release/ocp-release/4.14.1-x86_64", filepath.Join(ex.Opts.Global.WorkingDir, releaseImageExtractDir, "ocp-release/4.13.10-x86_64"))
		if err != nil {
			t.Fatalf("should not fail")
		}

		res, err := ex.ReleaseImageCollector(ctx)
		if err != nil {
			t.Fatalf("should not fail: %v", err)
		}
		// must contain 4 release component images
		// must contain 1 graph image
		// must contain 1 release image
		// must contain 1 kubevirt image
		expected := []v2alpha1.CopyImageSchema{
			{
				Source:      consts.DockerProtocol + "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e4182331e",
				Destination: consts.DockerProtocol + "localhost:9999/openshift/release:4.13.10-x86_64-agent-installer-api-server",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e4182331e",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Source:      consts.DockerProtocol + "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:955faaa822dc107f4dffa6a7e457f8d57a65d10949f74f6780ddd63c115e31e5",
				Destination: consts.DockerProtocol + "localhost:9999/openshift/release:4.13.10-x86_64-agent-installer-node-agent",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:955faaa822dc107f4dffa6a7e457f8d57a65d10949f74f6780ddd63c115e31e5",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Source:      consts.DockerProtocol + "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4949b93b3fd0f6b22197402ba22c2775eba408b53d30ac2e3ab2dda409314f5e",
				Destination: consts.DockerProtocol + "localhost:9999/openshift/release:4.13.10-x86_64-agent-installer-orchestrator",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4949b93b3fd0f6b22197402ba22c2775eba408b53d30ac2e3ab2dda409314f5e",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Source:      consts.DockerProtocol + "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:2a0dd75b1b327a0c5b17145fc71beb2bf805e6cc3b8fc3f672ce06772caddf21",
				Destination: consts.DockerProtocol + "localhost:9999/openshift/release:4.13.10-x86_64-apiserver-network-proxy",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:2a0dd75b1b327a0c5b17145fc71beb2bf805e6cc3b8fc3f672ce06772caddf21",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Type:        v2alpha1.TypeOCPReleaseContent,
				Source:      consts.DockerProtocol + "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:729265d5ef6ed6a45bcd55c46877e3acb9eae3f49c78cd795d5b53aa85e3775b",
				Destination: consts.DockerProtocol + "localhost:9999/openshift/release:4.13.10-x86_64-kube-virt-container",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:729265d5ef6ed6a45bcd55c46877e3acb9eae3f49c78cd795d5b53aa85e3775b",
			},
			{
				Source:      consts.DockerProtocol + "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
				Destination: consts.DockerProtocol + "localhost:9999/openshift/release-images:4.13.10-x86_64",
				Origin:      "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
				Type:        v2alpha1.TypeOCPRelease,
			},
			{
				Source:      consts.DockerProtocol + "localhost:9999/openshift/graph-image:latest",
				Destination: consts.DockerProtocol + "localhost:9999/openshift/graph-image:latest",
				Origin:      consts.DockerProtocol + "localhost:9999/openshift/graph-image:latest",
				Type:        v2alpha1.TypeCincinnatiGraph,
			},
		}
		assert.ElementsMatch(t, expected, res.AllImages)
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector - Disk to mirror : should pass", func(t *testing.T) {
		err := folder.RemoveFolders(
			filepath.Join(consts.TestFolder, "hold-release"),
			filepath.Join(consts.TestFolder, "release-images"),
			filepath.Join(consts.TestFolder, "tmp"),
		)
		assert.NoError(t, err)

		ex := setupCollector_DiskToMirror(tempDir, log)
		// copy tests/hold-test-fake to working-dir
		err = copy.Copy(
			filepath.Join(consts.TestFolder, "working-dir-fake", "hold-release", "ocp-release", "4.14.1-x86_64"),
			filepath.Join(ex.Opts.Global.WorkingDir, releaseImageExtractDir, "ocp-release/4.13.10-x86_64"),
		)
		if err != nil {
			t.Fatalf("should not fail")
		}

		res, err := ex.ReleaseImageCollector(context.Background())
		if err != nil {
			t.Fatalf("should not fail: %v", err)
		}
		if len(res.AllImages) == 0 {
			t.Fatalf("should contain at least 1 image")
		}
		if !strings.Contains(res.AllImages[0].Source, ex.LocalStorageFQDN) {
			t.Fatalf("source images should be from local storage")
		}
		// must contain 4 release component images
		// must contain 1 graph image
		// must contain 1 release image
		// must contain 1 kubevirt image
		expected := []v2alpha1.CopyImageSchema{
			{
				Source:      consts.DockerProtocol + "localhost:9999/openshift/release:4.13.10-x86_64-agent-installer-api-server",
				Destination: consts.DockerProtocol + "localhost:5000/test/openshift/release:4.13.10-x86_64-agent-installer-api-server",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e4182331e",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Source:      consts.DockerProtocol + "localhost:9999/openshift/release:4.13.10-x86_64-agent-installer-node-agent",
				Destination: consts.DockerProtocol + "localhost:5000/test/openshift/release:4.13.10-x86_64-agent-installer-node-agent",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:955faaa822dc107f4dffa6a7e457f8d57a65d10949f74f6780ddd63c115e31e5",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Source:      consts.DockerProtocol + "localhost:9999/openshift/release:4.13.10-x86_64-agent-installer-orchestrator",
				Destination: consts.DockerProtocol + "localhost:5000/test/openshift/release:4.13.10-x86_64-agent-installer-orchestrator",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4949b93b3fd0f6b22197402ba22c2775eba408b53d30ac2e3ab2dda409314f5e",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Source:      consts.DockerProtocol + "localhost:9999/openshift/release:4.13.10-x86_64-apiserver-network-proxy",
				Destination: consts.DockerProtocol + "localhost:5000/test/openshift/release:4.13.10-x86_64-apiserver-network-proxy",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:2a0dd75b1b327a0c5b17145fc71beb2bf805e6cc3b8fc3f672ce06772caddf21",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Type:        v2alpha1.TypeOCPReleaseContent,
				Source:      consts.DockerProtocol + "localhost:9999/openshift/release:4.13.10-x86_64-kube-virt-container",
				Destination: consts.DockerProtocol + "localhost:5000/test/openshift/release:4.13.10-x86_64-kube-virt-container",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:729265d5ef6ed6a45bcd55c46877e3acb9eae3f49c78cd795d5b53aa85e3775b",
			},
			{
				Source:      consts.DockerProtocol + "localhost:9999/openshift/release-images:4.13.10-x86_64",
				Destination: consts.DockerProtocol + "localhost:5000/test/openshift/release-images:4.13.10-x86_64",
				Origin:      consts.DockerProtocol + "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
				Type:        v2alpha1.TypeOCPRelease,
			},
			{
				Source:      consts.DockerProtocol + "localhost:9999/openshift/graph-image:latest",
				Destination: consts.DockerProtocol + "localhost:5000/test/openshift/graph-image:latest",
				Origin:      consts.DockerProtocol + "localhost:9999/openshift/graph-image:latest",
				Type:        v2alpha1.TypeCincinnatiGraph,
			},
		}
		assert.ElementsMatch(t, expected, res.AllImages)
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector with real GetReleaseReferenceImages - Disk to mirror : should pass", func(t *testing.T) {
		err := folder.RemoveFolders(
			filepath.Join(consts.TestFolder, "hold-release"),
			filepath.Join(consts.TestFolder, "release-images"),
			filepath.Join(consts.TestFolder, "tmp"),
		)
		assert.NoError(t, err)

		ex := setupCollector_DiskToMirror(tempDir, log)

		client := &ocpClient{}
		client.SetQueryParams(ex.Config.Mirror.Platform.Architectures[0], ex.Config.Mirror.Platform.Channels[0].Name, "")
		sig := MockCincinnati{}
		cn := NewCincinnati(ex.Log, nil, &ex.Config, ex.Opts, client, false, sig)

		ex.Cincinnati = cn

		ex.Opts.Global.WorkingDir = filepath.Join(consts.TestFolder, "working-dir-fake")

		res, err := ex.ReleaseImageCollector(context.Background())
		if err != nil {
			t.Fatalf("should not fail: %v", err)
		}
		if len(res.AllImages) == 0 {
			t.Fatalf("should contain at least 1 image")
		}
		if !strings.Contains(res.AllImages[0].Source, ex.LocalStorageFQDN) {
			t.Fatalf("source images should be from local storage")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail image index", func(t *testing.T) {
		manifest := &MockManifest{Log: log, FailImageIndex: true}

		ex := setupCollector_MirrorToDisk(tempDir, log, manifest)
		res, err := ex.ReleaseImageCollector(context.Background())
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail image manifest", func(t *testing.T) {
		manifest := &MockManifest{Log: log, FailImageManifest: true}
		ex := setupCollector_MirrorToDisk(tempDir, log, manifest)

		res, err := ex.ReleaseImageCollector(context.Background())
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail extract", func(t *testing.T) {
		manifest := &MockManifest{Log: log, FailExtract: true}
		ex := setupCollector_MirrorToDisk(tempDir, log, manifest)

		res, err := ex.ReleaseImageCollector(context.Background())
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})
}

func TestGraphImage(t *testing.T) {
	log := clog.New("trace")
	tempDir := t.TempDir()

	t.Run("Testing GraphImage : should fail", func(t *testing.T) {
		ex := setupCollector_DiskToMirror(tempDir, log)

		res, err := ex.GraphImage()
		if err != nil {
			t.Fatalf("should pass")
		}
		assert.Equal(t, ex.Opts.Destination+"/"+graphImageName+":latest", res)
	})
}

func TestReleaseImage(t *testing.T) {
	log := clog.New("trace")
	tempDir := t.TempDir()

	t.Run("Testing ReleaseImage : should pass", func(t *testing.T) {
		err := folder.RemoveFolders(
			filepath.Join(consts.TestFolder, "hold-release"),
			filepath.Join(consts.TestFolder, "release-images"),
			filepath.Join(consts.TestFolder, "tmp"),
		)
		assert.NoError(t, err)

		ex := setupCollector_DiskToMirror(tempDir, log)
		// copy tests/hold-test-fake to working-dir
		err = copy.Copy(
			filepath.Join(consts.TestFolder, "working-dir-fake", "hold-release", "ocp-release", "4.14.1-x86_64"),
			filepath.Join(ex.Opts.Global.WorkingDir, releaseImageExtractDir, "ocp-release", "4.13.9-x86_64"),
		)
		if err != nil {
			t.Fatalf("should not fail")
		}

		res, err := ex.ReleaseImage(context.Background())
		if err != nil {
			t.Fatalf("should pass: %v", err)
		}
		assert.Contains(t, res, "localhost:5000/test/openshift/release-images")
	})
}

func TestHandleGraphImage(t *testing.T) {
	type testCase struct {
		name              string
		mode              string
		updateURLOverride string
		imageInCache      bool
		imageInWorkingDir bool
		expectedError     bool
		expectedGraphCopy v2alpha1.CopyImageSchema
	}
	testCases := []testCase{
		{
			name:              "M2M - no UPDATE_URL_OVERRIDE: should pass",
			mode:              mirror.MirrorToMirror,
			updateURLOverride: "",
			imageInCache:      false,
			imageInWorkingDir: false,
			expectedError:     false,
			expectedGraphCopy: v2alpha1.CopyImageSchema{
				Source:      consts.DockerProtocol + "mymirror/openshift/graph-image:latest",
				Destination: consts.DockerProtocol + "mymirror/openshift/graph-image:latest",
				Origin:      consts.DockerProtocol + "mymirror/openshift/graph-image:latest",
				Type:        v2alpha1.TypeCincinnatiGraph,
			},
		},
		{
			name:              "M2D - no UPDATE_URL_OVERRIDE: should pass",
			mode:              mirror.MirrorToDisk,
			updateURLOverride: "",
			imageInCache:      false,
			imageInWorkingDir: false,
			expectedError:     false,
			expectedGraphCopy: v2alpha1.CopyImageSchema{
				Source:      consts.DockerProtocol + "localhost:9999/openshift/graph-image:latest",
				Destination: consts.DockerProtocol + "localhost:9999/openshift/graph-image:latest",
				Origin:      consts.DockerProtocol + "localhost:9999/openshift/graph-image:latest",
				Type:        v2alpha1.TypeCincinnatiGraph,
			},
		},
		{
			name:              "M2M - UPDATE_URL_OVERRIDE - image in cache: should pass",
			mode:              mirror.MirrorToMirror,
			updateURLOverride: "https://localhost.localdomain:3443/graph",
			imageInCache:      true,
			imageInWorkingDir: false,
			expectedError:     false,
			expectedGraphCopy: v2alpha1.CopyImageSchema{
				Source:      "docker://localhost:9999/openshift/graph-image:latest",
				Destination: "docker://mymirror/openshift/graph-image:latest",
				Origin:      "docker://localhost:9999/openshift/graph-image:latest",
				Type:        v2alpha1.TypeCincinnatiGraph,
			},
		},
		{
			name:              "M2M - UPDATE_URL_OVERRIDE - image in working-dir: should pass",
			mode:              mirror.MirrorToMirror,
			updateURLOverride: "https://localhost.localdomain:3443/graph",
			imageInCache:      false,
			imageInWorkingDir: true,
			expectedError:     false,
			expectedGraphCopy: v2alpha1.CopyImageSchema{
				Source:      consts.OciProtocol + "TEMPDIR/" + graphPreparationDir,
				Destination: "docker://mymirror/openshift/graph-image:latest",
				Origin:      consts.OciProtocol + "TEMPDIR/" + graphPreparationDir,
				Type:        v2alpha1.TypeCincinnatiGraph,
			},
		},
		{
			name:              "M2M - UPDATE_URL_OVERRIDE - image nowhere: should pass",
			mode:              mirror.MirrorToMirror,
			updateURLOverride: "https://localhost.localdomain:3443/graph",
			imageInCache:      false,
			imageInWorkingDir: false,
			expectedError:     true,
			expectedGraphCopy: v2alpha1.CopyImageSchema{},
		},
		{
			name:              "M2D - UPDATE_URL_OVERRIDE - image in cache: should pass",
			mode:              mirror.MirrorToDisk,
			updateURLOverride: "https://localhost.localdomain:3443/graph",
			imageInCache:      true,
			imageInWorkingDir: false,
			expectedError:     false,
			expectedGraphCopy: v2alpha1.CopyImageSchema{
				Source:      "docker://localhost:9999/openshift/graph-image:latest",
				Destination: "docker://localhost:9999/openshift/graph-image:latest",
				Origin:      "docker://localhost:9999/openshift/graph-image:latest",
				Type:        v2alpha1.TypeCincinnatiGraph,
			},
		},
		{
			name:              "M2D - UPDATE_URL_OVERRIDE - image in working-dir: should pass",
			mode:              mirror.MirrorToDisk,
			updateURLOverride: "https://localhost.localdomain:3443/graph",
			imageInCache:      false,
			imageInWorkingDir: true,
			expectedError:     false,
			expectedGraphCopy: v2alpha1.CopyImageSchema{
				Source:      consts.OciProtocol + "TEMPDIR/" + graphPreparationDir,
				Destination: "docker://localhost:9999/openshift/graph-image:latest",
				Origin:      consts.OciProtocol + "TEMPDIR/" + graphPreparationDir,
				Type:        v2alpha1.TypeCincinnatiGraph,
			},
		},
		{
			name:              "M2D - UPDATE_URL_OVERRIDE - image nowhere: should pass",
			mode:              mirror.MirrorToDisk,
			updateURLOverride: "https://localhost.localdomain:3443/graph",
			imageInCache:      false,
			imageInWorkingDir: false,
			expectedError:     true,
			expectedGraphCopy: v2alpha1.CopyImageSchema{},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tempDir := t.TempDir()
			globalOpts := &mirror.GlobalOptions{
				SecurePolicy: false,
				WorkingDir:   tempDir,
			}

			_, sharedOpts := mirror.SharedImageFlags()
			_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
			_, retryOpts := mirror.RetryFlags()
			_, srcOpts := mirror.ImageSrcFlags(globalOpts, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
			_, destOpts := mirror.ImageDestFlags(globalOpts, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")

			copyOpts := mirror.CopyOptions{
				Global:              globalOpts,
				DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
				SrcImage:            srcOpts,
				DestImage:           destOpts,
				RetryOpts:           retryOpts,
				Dev:                 false,
				Mode:                testCase.mode,
			}

			if testCase.mode == mirror.MirrorToDisk {
				copyOpts.Destination = consts.FileProtocol + tempDir
				copyOpts.Global.WorkingDir = tempDir
			}
			if testCase.mode == mirror.MirrorToMirror {
				copyOpts.Destination = "docker://mymirror"
				copyOpts.Global.WorkingDir = tempDir
			}

			log := clog.New("trace")
			manifestMock := new(ManifestMock)
			if testCase.imageInCache {
				manifestMock.On("ImageDigest", mock.Anything, mock.Anything, "docker://localhost:9999/openshift/graph-image:latest").Return("123456", nil)
			} else {
				manifestMock.On("ImageDigest", mock.Anything, mock.Anything, "docker://localhost:9999/openshift/graph-image:latest").Return("", fmt.Errorf("simulating image doesn't exist in cache"))
			}
			if testCase.imageInWorkingDir {
				manifestMock.On("ImageDigest", mock.Anything, mock.Anything, consts.OciProtocol+copyOpts.Global.WorkingDir+"/"+graphPreparationDir).Return("123456", nil)
			} else {
				manifestMock.On("ImageDigest", mock.Anything, mock.Anything, consts.OciProtocol+copyOpts.Global.WorkingDir+"/"+graphPreparationDir).Return("", fmt.Errorf("simulating image doesn't exist in cache"))
			}
			if testCase.updateURLOverride != "" {
				t.Setenv("UPDATE_URL_OVERRIDE", testCase.updateURLOverride)
			}
			ex := &LocalStorageCollector{
				Log:              log,
				Mirror:           &MockMirror{Fail: false},
				Opts:             copyOpts,
				Manifest:         manifestMock,
				LocalStorageFQDN: "localhost:9999",
				ImageBuilder:     &mockImageBuilder{},
				LogsDir:          "/tmp/",
			}
			graphImage, err := ex.handleGraphImage(context.Background())
			if testCase.expectedError && err == nil {
				t.Error("expecting test to fail with error, but no error returned")
			}
			if !testCase.expectedError && err != nil {
				t.Errorf("unexpected failure: %v", err)
			}
			testCase.expectedGraphCopy.Source = strings.Replace(testCase.expectedGraphCopy.Source, "TEMPDIR", copyOpts.Global.WorkingDir, 1)
			testCase.expectedGraphCopy.Origin = strings.Replace(testCase.expectedGraphCopy.Origin, "TEMPDIR", copyOpts.Global.WorkingDir, 1)
			testCase.expectedGraphCopy.Destination = strings.Replace(testCase.expectedGraphCopy.Destination, "TEMPDIR", copyOpts.Global.WorkingDir, 1)

			assert.Equal(t, testCase.expectedGraphCopy, graphImage)
		})
	}
}

func setupCollector_DiskToMirror(tempDir string, log clog.PluggableLoggerInterface) *LocalStorageCollector {
	manifest := &MockManifest{Log: log}

	globalD2M := &mirror.GlobalOptions{
		SecurePolicy: false,
		WorkingDir:   tempDir + "/working-dir",
		From:         tempDir,
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOptsD2M := mirror.ImageSrcFlags(globalD2M, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOptsD2M := mirror.ImageDestFlags(globalD2M, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	d2mOpts := mirror.CopyOptions{
		Global:              globalD2M,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOptsD2M,
		DestImage:           destOptsD2M,
		RetryOpts:           retryOpts,
		Destination:         "docker://localhost:5000/test",
		Dev:                 false,
		Mode:                mirror.DiskToMirror,
	}

	cfgd2m := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Platform: v2alpha1.Platform{
					Architectures: []string{"amd64"},
					Channels: []v2alpha1.ReleaseChannel{
						{
							Name:       "stable-4.13",
							MinVersion: "4.13.9",
							MaxVersion: "4.13.10",
						},
					},
					Graph:             true,
					KubeVirtContainer: true,
				},
			},
		},
	}

	cincinnati := &MockCincinnati{}
	client := &ocpClient{}
	client.SetQueryParams(cfgd2m.Mirror.Platform.Architectures[0], cfgd2m.Mirror.Platform.Channels[0].Name, "")
	cincinnati.Client = client

	ex := &LocalStorageCollector{
		Log:              log,
		Mirror:           &MockMirror{Fail: false},
		Config:           cfgd2m,
		Manifest:         manifest,
		Opts:             d2mOpts,
		Cincinnati:       cincinnati,
		LocalStorageFQDN: "localhost:9999",
		LogsDir:          "/tmp/",
	}

	return ex
}

func setupCollector_MirrorToDisk(tempDir string, log clog.PluggableLoggerInterface, manifest *MockManifest) *LocalStorageCollector {
	globalM2D := &mirror.GlobalOptions{
		SecurePolicy: false,
		WorkingDir:   tempDir,
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, retryOpts := mirror.RetryFlags()
	_, srcOptsM2D := mirror.ImageSrcFlags(globalM2D, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOptsM2D := mirror.ImageDestFlags(globalM2D, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")

	m2dOpts := mirror.CopyOptions{
		Global:              globalM2D,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOptsM2D,
		DestImage:           destOptsM2D,
		RetryOpts:           retryOpts,
		Destination:         consts.FileProtocol + "test",
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
	}

	cfgm2d := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Platform: v2alpha1.Platform{
					Channels: []v2alpha1.ReleaseChannel{
						{
							Name: "stable-4.7",
						},
						{
							Name:       "stable-4.6",
							MinVersion: "4.6.3",
							MaxVersion: "4.6.13",
						},
						{
							Name: "okd",
							Type: v2alpha1.TypeOKD,
						},
					},
					Graph:             true,
					KubeVirtContainer: true,
				},
				Operators: []v2alpha1.Operator{
					{
						Catalog: "redhat-operators:v4.7",
						Full:    true,
					},
					{
						Catalog: "certified-operators:v4.7",
						Full:    true,
						IncludeConfig: v2alpha1.IncludeConfig{
							Packages: []v2alpha1.IncludePackage{
								{Name: "couchbase-operator"},
								{
									Name: "mongodb-operator",
									IncludeBundle: v2alpha1.IncludeBundle{
										MinVersion: "1.4.0",
									},
								},
								{
									Name: "crunchy-postgresql-operator",
									Channels: []v2alpha1.IncludeChannel{
										{Name: "stable"},
									},
								},
							},
						},
					},
					{
						Catalog: "community-operators:v4.7",
					},
				},
				AdditionalImages: []v2alpha1.AdditionalImage{
					{Name: "registry.redhat.io/ubi8/ubi:latest"},
				},
				Helm: v2alpha1.Helm{
					Repositories: []v2alpha1.Repository{
						{
							URL:  "https://stefanprodan.github.io/podinfo",
							Name: "podinfo",
							Charts: []v2alpha1.Chart{
								{Name: "podinfo", Version: "5.0.0"},
							},
						},
					},
					Local: []v2alpha1.Chart{
						{Name: "podinfo", Path: "/test/podinfo-5.0.0.tar.gz"},
					},
				},
				BlockedImages: []v2alpha1.BlockedImage{
					{Name: "alpine"},
					{Name: "redis"},
				},
				Samples: []v2alpha1.SampleImage{
					{Name: "ruby"},
					{Name: "python"},
					{Name: "nginx"},
				},
			},
		},
	}

	cincinnati := &MockCincinnati{Config: cfgm2d, Opts: m2dOpts}
	ex := &LocalStorageCollector{
		Log:              log,
		Mirror:           &MockMirror{Fail: false},
		Config:           cfgm2d,
		Manifest:         manifest,
		Opts:             m2dOpts,
		Cincinnati:       cincinnati,
		LocalStorageFQDN: "localhost:9999",
		ImageBuilder:     &mockImageBuilder{},
		LogsDir:          "/tmp/",
	}
	return ex
}

func (o MockMirror) Run(ctx context.Context, src, dest string, mode mirror.Mode, opts *mirror.CopyOptions) error {
	if o.Fail {
		return fmt.Errorf("forced mirror run fail")
	}
	return nil
}

func (o MockMirror) Check(ctx context.Context, image string, opts *mirror.CopyOptions, asCopySrc bool) (bool, error) {
	return true, nil
}

func (o MockManifest) GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error) {
	return nil, nil
}

func (o MockManifest) GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error) {
	relatedImages := []v2alpha1.RelatedImage{
		{Name: "agent-installer-api-server", Type: v2alpha1.TypeOCPReleaseContent, Image: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e4182331e"},
		{Name: "agent-installer-node-agent", Type: v2alpha1.TypeOCPReleaseContent, Image: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:955faaa822dc107f4dffa6a7e457f8d57a65d10949f74f6780ddd63c115e31e5"},
		{Name: "agent-installer-orchestrator", Type: v2alpha1.TypeOCPReleaseContent, Image: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4949b93b3fd0f6b22197402ba22c2775eba408b53d30ac2e3ab2dda409314f5e"},
		{Name: "apiserver-network-proxy", Type: v2alpha1.TypeOCPReleaseContent, Image: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:2a0dd75b1b327a0c5b17145fc71beb2bf805e6cc3b8fc3f672ce06772caddf21"},
	}
	return relatedImages, nil
}

func (o MockManifest) GetOCIImageIndex(name string) (*specv1.Index, error) {
	if o.FailImageIndex {
		return nil, fmt.Errorf("forced error image index")
	}
	d, err := digest.Parse("sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419")
	if err != nil {
		return nil, fmt.Errorf("failed to parse digest: %w", err)
	}
	return &specv1.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Manifests: []specv1.Descriptor{
			{
				MediaType: specv1.MediaTypeImageManifest,
				Digest:    d,
				Size:      567,
			},
		},
	}, nil
}

func (o MockManifest) GetOCIImageManifest(name string) (*specv1.Manifest, error) {
	if o.FailImageManifest {
		return nil, fmt.Errorf("forced error image index")
	}

	d, err := digest.Parse("sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419")
	if err != nil {
		return nil, fmt.Errorf("failed to parse digest: %w", err)
	}
	return &specv1.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config: specv1.Descriptor{
			MediaType: specv1.MediaTypeImageManifest,
			Digest:    d,
			Size:      567,
		},
	}, nil
}

func (o MockManifest) GetOCIImageFromIndex(dir string) (gcrv1.Image, error) { //nolint:ireturn // interface is expected in this case
	if o.FailImageIndex {
		return nil, fmt.Errorf("forced error image index")
	}
	if o.FailImageManifest {
		return nil, fmt.Errorf("forced error image manifest")
	}
	return &fake.FakeImage{}, nil
}

func (o MockManifest) ExtractOCILayers(_ gcrv1.Image, toPath, label string) error {
	if o.FailExtract {
		return fmt.Errorf("forced extract oci fail")
	}
	return nil
}

func (o MockManifest) ConvertOCIIndexToSingleManifest(dir string, oci *specv1.Index) error {
	return nil
}

func (o MockManifest) ImageDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	return "3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419", nil
}

func (o MockManifest) ImageManifest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, instanceDigest *digest.Digest) ([]byte, string, error) {
	return nil, "", nil
}

func (o MockManifest) GetManifestListDigests(ctx context.Context, sourceCtx *types.SystemContext, source string) ([]string, error) {
	return nil, nil
}

func (o MockCincinnati) GetReleaseReferenceImages(ctx context.Context) ([]v2alpha1.CopyImageSchema, error) {
	var res []v2alpha1.CopyImageSchema
	res = append(res, v2alpha1.CopyImageSchema{Type: v2alpha1.TypeOCPRelease, Source: "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64", Origin: "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64"})
	return res, nil
}

func (o MockCincinnati) NewOCPClient(uuid uuid.UUID) (Client, error) {
	if o.Fail {
		return o.Client, fmt.Errorf("forced cincinnati client error")
	}
	return o.Client, nil
}

func (o MockCincinnati) NewOKDClient(uuid uuid.UUID) (Client, error) {
	return o.Client, nil
}

func (o MockCincinnati) GenerateReleaseSignatures(ctx context.Context, images []v2alpha1.CopyImageSchema) ([]v2alpha1.CopyImageSchema, error) {
	imagesByTag := make([]v2alpha1.CopyImageSchema, len(images))
	minor := 9
	for i, img := range images {
		digestIndex := strings.LastIndex(img.Source, "@sha256:")
		img.Origin = img.Source[:digestIndex] + ":v4.13." + fmt.Sprint(minor)
		img.Source = img.Source[:digestIndex] + ":v4.13." + fmt.Sprint(minor)
		img.Destination = ""
		img.Type = v2alpha1.TypeOCPRelease
		minor++
		imagesByTag[i] = img
	}
	return imagesByTag, nil
}

type ManifestMock struct {
	mock.Mock
}

func (o *ManifestMock) GetOCIImageIndex(dir string) (*specv1.Index, error) {
	return nil, nil
}

func (o *ManifestMock) GetOCIImageManifest(file string) (*specv1.Manifest, error) {
	return nil, nil
}

func (o *ManifestMock) GetOCIImageFromIndex(dir string) (gcrv1.Image, error) { //nolint:ireturn // as expected by go-containerregistry
	return nil, nil
}

func (o *ManifestMock) GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error) {
	return &v2alpha1.OperatorConfigSchema{}, nil
}

func (o *ManifestMock) ExtractOCILayers(_ gcrv1.Image, toPath, label string) error {
	return nil
}

func (o *ManifestMock) GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error) {
	return []v2alpha1.RelatedImage{}, nil
}

func (o *ManifestMock) ConvertOCIIndexToSingleManifest(dir string, oci *specv1.Index) error {
	return nil
}

func (o *ManifestMock) ImageDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	args := o.Called(ctx, sourceCtx, imgRef)
	return args.String(0), args.Error(1)
}

func (o *ManifestMock) ImageManifest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, instanceDigest *digest.Digest) ([]byte, string, error) {
	return nil, "", nil
}

func (o *ManifestMock) GetManifestListDigests(ctx context.Context, sourceCtx *types.SystemContext, source string) ([]string, error) {
	return nil, nil
}

// TestRequestedKubeVirtArchKeys verifies that the architecture-key resolution
// logic correctly maps user-facing architecture names to bootimages JSON keys
// and handles all three configuration paths (Platforms, Architectures, default).
// Only x86_64 (amd64) and s390x carry kubevirt images in the bootimages JSON.
func TestRequestedKubeVirtArchKeys(t *testing.T) {
	log := clog.New("trace")

	makeCollector := func(cfg v2alpha1.ImageSetConfiguration) LocalStorageCollector {
		return LocalStorageCollector{
			Log:    log,
			Config: cfg,
		}
	}

	t.Run("platform.platforms amd64+s390x maps to x86_64+s390x", func(t *testing.T) {
		cfg := v2alpha1.ImageSetConfiguration{
			ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform: v2alpha1.Platform{
						Platforms: []v2alpha1.InstancePlatformFilter{
							{OS: "linux", Architecture: "amd64"},
							{OS: "linux", Architecture: "s390x"},
						},
						KubeVirtContainer: true,
					},
				},
			},
		}
		keys := makeCollector(cfg).requestedKubeVirtArchKeys()
		assert.ElementsMatch(t, []string{"x86_64", "s390x"}, keys)
	})

	t.Run("platform.platforms with unsupported arch (arm64) is skipped", func(t *testing.T) {
		cfg := v2alpha1.ImageSetConfiguration{
			ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform: v2alpha1.Platform{
						Platforms: []v2alpha1.InstancePlatformFilter{
							{OS: "linux", Architecture: "arm64"},
						},
						KubeVirtContainer: true,
					},
				},
			},
		}
		// arm64 has no kubevirt entry; falls back to all known keys
		keys := makeCollector(cfg).requestedKubeVirtArchKeys()
		assert.ElementsMatch(t, allKubeVirtArchKeys, keys)
	})

	t.Run("deprecated platform.architectures amd64 maps to x86_64", func(t *testing.T) {
		cfg := v2alpha1.ImageSetConfiguration{
			ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform: v2alpha1.Platform{
						Architectures:     []string{"amd64"},
						KubeVirtContainer: true,
					},
				},
			},
		}
		keys := makeCollector(cfg).requestedKubeVirtArchKeys()
		assert.Equal(t, []string{"x86_64"}, keys)
	})

	t.Run("deprecated platform.architectures multi returns all kubevirt arch keys", func(t *testing.T) {
		cfg := v2alpha1.ImageSetConfiguration{
			ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform: v2alpha1.Platform{
						Architectures:     []string{"multi"},
						KubeVirtContainer: true,
					},
				},
			},
		}
		keys := makeCollector(cfg).requestedKubeVirtArchKeys()
		assert.ElementsMatch(t, allKubeVirtArchKeys, keys)
	})

	t.Run("no architecture config defaults to x86_64", func(t *testing.T) {
		cfg := v2alpha1.ImageSetConfiguration{
			ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform: v2alpha1.Platform{
						KubeVirtContainer: true,
					},
				},
			},
		}
		keys := makeCollector(cfg).requestedKubeVirtArchKeys()
		assert.Equal(t, []string{"x86_64"}, keys)
	})
}

// TestGetKubeVirtImages verifies that getKubeVirtImages returns one image per
// architecture that carries a kubevirt digest-ref in the bootimages ConfigMap.
// Per the upstream coreos-rhel-9.json, only x86_64 and s390x have kubevirt images.
func TestGetKubeVirtImages(t *testing.T) {
	log := clog.New("trace")

	// The testdata bootimages file mirrors the upstream shape:
	// x86_64 and s390x have kubevirt images; aarch64 and ppc64le do not.
	testdataDir := filepath.Join(consts.TestFolder, "working-dir-fake", "hold-release", "ocp-release", "4.14.1-x86_64")

	t.Run("default (no arch config) returns only x86_64 kubevirt image with legacy name", func(t *testing.T) {
		cfg := v2alpha1.ImageSetConfiguration{
			ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform: v2alpha1.Platform{KubeVirtContainer: true},
				},
			},
		}
		col := LocalStorageCollector{Log: log, Config: cfg}
		images, err := col.getKubeVirtImages(testdataDir)
		assert.NoError(t, err)
		assert.Len(t, images, 1)
		assert.Contains(t, images[0].Image, "sha256:729265d5ef6ed6a45bcd55c46877e3acb9eae3f49c78cd795d5b53aa85e3775b")
		// Backward compatibility: single-arch mirrors use the legacy name without suffix
		assert.Equal(t, "kube-virt-container", images[0].Name)
	})

	t.Run("platform.platforms amd64+s390x returns two kubevirt images with arch suffix", func(t *testing.T) {
		cfg := v2alpha1.ImageSetConfiguration{
			ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform: v2alpha1.Platform{
						Platforms: []v2alpha1.InstancePlatformFilter{
							{OS: "linux", Architecture: "amd64"},
							{OS: "linux", Architecture: "s390x"},
						},
						KubeVirtContainer: true,
					},
				},
			},
		}
		col := LocalStorageCollector{Log: log, Config: cfg}
		images, err := col.getKubeVirtImages(testdataDir)
		assert.NoError(t, err)
		assert.Len(t, images, 2)

		// Multi-arch: names include architecture suffix to avoid tag collision
		for _, img := range images {
			switch img.Name {
			case "kube-virt-container-x86_64":
				assert.Contains(t, img.Image, "sha256:729265d5ef6ed6a45bcd55c46877e3acb9eae3f49c78cd795d5b53aa85e3775b")
			case "kube-virt-container-s390x":
				assert.Contains(t, img.Image, "sha256:s390xs390xs390xs390xs390xs390xs390xs390xs390xs390xs390xs390xs390")
			default:
				t.Fatalf("unknown kubevirt image: %s", img.Name)
			}
		}
	})

	t.Run("platform.platforms with all arches returns only x86_64+s390x kubevirt images with arch suffix", func(t *testing.T) {
		// aarch64 and ppc64le have no kubevirt entry in the bootimages JSON; they are skipped.
		cfg := v2alpha1.ImageSetConfiguration{
			ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform: v2alpha1.Platform{
						Platforms: []v2alpha1.InstancePlatformFilter{
							{OS: "linux", Architecture: "amd64"},
							{OS: "linux", Architecture: "arm64"},
							{OS: "linux", Architecture: "ppc64le"},
							{OS: "linux", Architecture: "s390x"},
						},
						KubeVirtContainer: true,
					},
				},
			},
		}
		col := LocalStorageCollector{Log: log, Config: cfg}
		images, err := col.getKubeVirtImages(testdataDir)
		assert.NoError(t, err)
		// arm64 and ppc64le have no kubevirt mapping, so only x86_64 and s390x images are returned.
		assert.Len(t, images, 2)
		names := make([]string, len(images))
		for i, img := range images {
			assert.Equal(t, v2alpha1.TypeOCPReleaseContent, img.Type)
			assert.NotEmpty(t, img.Image)
			names[i] = img.Name
		}
		// Multi-arch: names include architecture suffix
		assert.ElementsMatch(t, []string{"kube-virt-container-x86_64", "kube-virt-container-s390x"}, names)
	})

	t.Run("deprecated platform.architectures multi returns x86_64+s390x kubevirt images with arch suffix", func(t *testing.T) {
		cfg := v2alpha1.ImageSetConfiguration{
			ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform: v2alpha1.Platform{
						Architectures:     []string{"multi"},
						KubeVirtContainer: true,
					},
				},
			},
		}
		col := LocalStorageCollector{Log: log, Config: cfg}
		images, err := col.getKubeVirtImages(testdataDir)
		assert.NoError(t, err)
		assert.Len(t, images, 2)
		for _, img := range images {
			switch img.Name {
			case "kube-virt-container-x86_64":
				assert.Contains(t, img.Image, "sha256:729265d5ef6ed6a45bcd55c46877e3acb9eae3f49c78cd795d5b53aa85e3775b")
			case "kube-virt-container-s390x":
				assert.Contains(t, img.Image, "sha256:s390xs390xs390xs390xs390xs390xs390xs390xs390xs390xs390xs390xs390")
			default:
				t.Fatalf("unknown kubevirt image %s", img.Name)
			}
		}
	})
}

// Synthetic kubevirt images and release used by the OCPBUGS-138157 tests.
const (
	// kubeVirtTestReleaseTag is the release directory name, used as release tag, of the
	// synthetic release written by writeKubeVirtTestBootImages.
	kubeVirtTestReleaseTag = "1.0.0-x86_64"
	kubeVirtTestRepo       = "example.com/coreos"
	kubeVirtTestDigest1    = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	kubeVirtTestDigest2    = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	kubeVirtTestDigest3    = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
	kubeVirtTestDigest4    = "sha256:4444444444444444444444444444444444444444444444444444444444444444"
)

// OCPBUGS-138157: bootimages ConfigMaps can hold a collection of CoreOS streams
// (data.streams) in addition to the legacy single stream (data.stream).
func TestGetKubeVirtImagesStreamCollection(t *testing.T) {
	// The legacy single stream is usually one of the streams of the collection.
	legacyStream := kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest1})
	// The stream names are intentionally not sorted in the JSON document.
	streamCollection := `{"rhel-9":` + kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest1, "s390x": kubeVirtTestDigest3}) +
		`,"rhel-10":` + kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest2}) + `}`

	defaultPlatform := v2alpha1.Platform{KubeVirtContainer: true}
	multiArchPlatform := v2alpha1.Platform{
		Platforms: []v2alpha1.InstancePlatformFilter{
			{OS: "linux", Architecture: "amd64"},
			{OS: "linux", Architecture: "s390x"},
		},
		KubeVirtContainer: true,
	}

	tests := []struct {
		name     string
		platform v2alpha1.Platform
		stream   string
		streams  string
		expected []v2alpha1.RelatedImage
		err      string
	}{
		{
			name:     "stream collection returns one image per stream sorted by stream name, without repeating the legacy stream image",
			platform: defaultPlatform,
			stream:   legacyStream,
			streams:  streamCollection,
			expected: []v2alpha1.RelatedImage{
				kubeVirtTestImage(kubeVirtTestDigest2, "kube-virt-container-rhel-10"),
				kubeVirtTestImage(kubeVirtTestDigest1, "kube-virt-container-rhel-9"),
			},
		},
		{
			name:     "stream collection without legacy stream",
			platform: defaultPlatform,
			streams:  streamCollection,
			expected: []v2alpha1.RelatedImage{
				kubeVirtTestImage(kubeVirtTestDigest2, "kube-virt-container-rhel-10"),
				kubeVirtTestImage(kubeVirtTestDigest1, "kube-virt-container-rhel-9"),
			},
		},
		{
			name:     "legacy stream image missing from the stream collection is kept with the legacy name",
			platform: defaultPlatform,
			stream:   kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest4}),
			streams:  streamCollection,
			expected: []v2alpha1.RelatedImage{
				kubeVirtTestImage(kubeVirtTestDigest2, "kube-virt-container-rhel-10"),
				kubeVirtTestImage(kubeVirtTestDigest1, "kube-virt-container-rhel-9"),
				kubeVirtTestImage(kubeVirtTestDigest4, "kube-virt-container"),
			},
		},
		{
			name:     "stream collection with multiple architectures skips streams without an image for an architecture",
			platform: multiArchPlatform,
			stream:   legacyStream,
			streams:  streamCollection,
			expected: []v2alpha1.RelatedImage{
				kubeVirtTestImage(kubeVirtTestDigest2, "kube-virt-container-x86_64-rhel-10"),
				kubeVirtTestImage(kubeVirtTestDigest1, "kube-virt-container-x86_64-rhel-9"),
				kubeVirtTestImage(kubeVirtTestDigest3, "kube-virt-container-s390x-rhel-9"),
			},
		},
		{
			name:     "multiple architectures keep only the legacy stream images missing from the stream collection",
			platform: multiArchPlatform,
			stream:   kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest1, "s390x": kubeVirtTestDigest4}),
			streams:  `{"rhel-9":` + kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest1}) + `}`,
			expected: []v2alpha1.RelatedImage{
				kubeVirtTestImage(kubeVirtTestDigest1, "kube-virt-container-x86_64-rhel-9"),
				kubeVirtTestImage(kubeVirtTestDigest4, "kube-virt-container-s390x"),
			},
		},
		{
			name:     "invalid legacy stream json next to a stream collection",
			platform: defaultPlatform,
			stream:   `{"architectures":`,
			streams:  streamCollection,
			err:      "parsing json from kubevirt configmap data",
		},
		{
			name:     "legacy stream only keeps the legacy name",
			platform: defaultPlatform,
			stream:   legacyStream,
			expected: []v2alpha1.RelatedImage{kubeVirtTestImage(kubeVirtTestDigest1, "kube-virt-container")},
		},
		{
			name:     "empty stream collection falls back to the legacy stream",
			platform: defaultPlatform,
			stream:   legacyStream,
			streams:  `{}`,
			expected: []v2alpha1.RelatedImage{kubeVirtTestImage(kubeVirtTestDigest1, "kube-virt-container")},
		},
		{
			name:     "null stream collection falls back to the legacy stream",
			platform: defaultPlatform,
			stream:   legacyStream,
			streams:  `null`,
			expected: []v2alpha1.RelatedImage{kubeVirtTestImage(kubeVirtTestDigest1, "kube-virt-container")},
		},
		{
			name:     "blank stream collection falls back to the legacy stream",
			platform: defaultPlatform,
			stream:   legacyStream,
			streams:  "  \n",
			expected: []v2alpha1.RelatedImage{kubeVirtTestImage(kubeVirtTestDigest1, "kube-virt-container")},
		},
		{
			name:     "empty stream collection without legacy stream",
			platform: defaultPlatform,
			streams:  `{}`,
			err:      "parsing json from kubevirt configmap data",
		},
		{
			name:     "invalid stream collection json",
			platform: defaultPlatform,
			stream:   legacyStream,
			streams:  `{"rhel-9":`,
			err:      "parsing json from kubevirt configmap streams data",
		},
		{
			name:     "stream name that cannot be used in an image tag",
			platform: defaultPlatform,
			streams:  `{"rhel/9":` + kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest1}) + `}`,
			err:      `invalid stream name "rhel/9" in kubevirt configmap streams data`,
		},
		{
			name:     "stream collection without kubevirt images",
			platform: defaultPlatform,
			streams:  `{"rhel-9":` + kubeVirtTestStream(t, nil) + `}`,
			err:      "could not find kubevirt image in this release",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := v2alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
					Mirror: v2alpha1.Mirror{Platform: tt.platform},
				},
			}
			col := LocalStorageCollector{Log: clog.New("trace"), Config: cfg}

			images, err := col.getKubeVirtImages(writeKubeVirtTestBootImages(t, tt.stream, tt.streams))
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, images)
		})
	}
}

// OCPBUGS-138157: each stream of a stream collection gets its own deterministic
// destination in the mirror to disk and disk to mirror copy batches.
func TestGetKubeVirtImagesStreamCollectionDestinations(t *testing.T) {
	stream1 := kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest1})
	stream2 := kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest2})
	releaseDir := writeKubeVirtTestBootImages(t, stream1, `{"rhel-9":`+stream1+`,"rhel-10":`+stream2+`}`)

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{Platform: v2alpha1.Platform{KubeVirtContainer: true}},
		},
	}
	col := LocalStorageCollector{Log: clog.New("trace"), Config: cfg, LocalStorageFQDN: "localhost:9999", Manifest: &MockManifest{}}

	images, err := col.getKubeVirtImages(releaseDir)
	require.NoError(t, err)
	// The streams are read into a map, whose iteration order is random:
	// the result must be the same on every call.
	for range 10 {
		again, err := col.getKubeVirtImages(releaseDir)
		require.NoError(t, err)
		require.Equal(t, images, again)
	}

	t.Run("mirror to disk", func(t *testing.T) {
		m2d := col
		m2d.Opts.Mode = mirror.MirrorToDisk
		batch, err := m2d.prepareM2DCopyBatch(images, kubeVirtTestReleaseTag)
		require.NoError(t, err)
		require.Equal(t, []v2alpha1.CopyImageSchema{
			{
				Origin:      kubeVirtTestRepo + "@" + kubeVirtTestDigest2,
				Source:      consts.DockerProtocol + kubeVirtTestRepo + "@" + kubeVirtTestDigest2,
				Destination: consts.DockerProtocol + "localhost:9999/openshift/release:1.0.0-x86_64-kube-virt-container-rhel-10",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Origin:      kubeVirtTestRepo + "@" + kubeVirtTestDigest1,
				Source:      consts.DockerProtocol + kubeVirtTestRepo + "@" + kubeVirtTestDigest1,
				Destination: consts.DockerProtocol + "localhost:9999/openshift/release:1.0.0-x86_64-kube-virt-container-rhel-9",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
		}, batch)
	})

	t.Run("disk to mirror", func(t *testing.T) {
		d2m := col
		d2m.Opts = kubeVirtTestD2MOptions()
		batch, err := d2m.prepareReleaseBatchFromDir(context.Background(), releaseDir)
		require.NoError(t, err)

		require.Equal(t, []v2alpha1.CopyImageSchema{
			{
				Origin:      kubeVirtTestRepo + "@" + kubeVirtTestDigest2,
				Source:      consts.DockerProtocol + "localhost:9999/openshift/release:1.0.0-x86_64-kube-virt-container-rhel-10",
				Destination: consts.DockerProtocol + "localhost:5000/test/openshift/release:1.0.0-x86_64-kube-virt-container-rhel-10",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Origin:      kubeVirtTestRepo + "@" + kubeVirtTestDigest1,
				Source:      consts.DockerProtocol + "localhost:9999/openshift/release:1.0.0-x86_64-kube-virt-container-rhel-9",
				Destination: consts.DockerProtocol + "localhost:5000/test/openshift/release:1.0.0-x86_64-kube-virt-container-rhel-9",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
		}, kubeVirtTestCopies(batch))
	})
}

// OCPBUGS-138157: archives created before stream collections were supported only hold
// the image of the legacy stream, under its legacy name. Disk to mirror copies it to the
// destination of a stream only when its digest is the digest of the image of that stream.
func TestPrepareReleaseBatchFromDirKubeVirtLegacyArchive(t *testing.T) {
	encoded := func(dgst string) string { return strings.TrimPrefix(dgst, "sha256:") }
	cacheRef := func(name string) string {
		return consts.DockerProtocol + "localhost:9999/openshift/release:" + kubeVirtTestReleaseTag + "-" + name
	}
	kubeVirtCopy := func(dgst, sourceName, destinationName string) v2alpha1.CopyImageSchema {
		return v2alpha1.CopyImageSchema{
			Origin:      kubeVirtTestRepo + "@" + dgst,
			Source:      cacheRef(sourceName),
			Destination: consts.DockerProtocol + "localhost:5000/test/openshift/release:" + kubeVirtTestReleaseTag + "-" + destinationName,
			Type:        v2alpha1.TypeOCPReleaseContent,
		}
	}

	singleArchRelease := writeKubeVirtTestBootImages(t,
		kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest1}),
		`{"rhel-9":`+kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest1})+
			`,"rhel-10":`+kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest2})+`}`)
	multiArchRelease := writeKubeVirtTestBootImages(t,
		kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest1, "s390x": kubeVirtTestDigest3}),
		`{"rhel-9":`+kubeVirtTestStream(t, map[string]string{"x86_64": kubeVirtTestDigest1, "s390x": kubeVirtTestDigest3})+`}`)
	multiArchPlatform := v2alpha1.Platform{
		Platforms: []v2alpha1.InstancePlatformFilter{
			{OS: "linux", Architecture: "amd64"},
			{OS: "linux", Architecture: "s390x"},
		},
		KubeVirtContainer: true,
	}
	streamCopies := []v2alpha1.CopyImageSchema{
		kubeVirtCopy(kubeVirtTestDigest2, "kube-virt-container-rhel-10", "kube-virt-container-rhel-10"),
		kubeVirtCopy(kubeVirtTestDigest1, "kube-virt-container-rhel-9", "kube-virt-container-rhel-9"),
	}

	tests := []struct {
		name       string
		platform   v2alpha1.Platform
		releaseDir string
		deleteMode bool
		cache      map[string]string
		expected   []v2alpha1.CopyImageSchema
	}{
		{
			name:       "archive with the images of the streams",
			releaseDir: singleArchRelease,
			cache: map[string]string{
				cacheRef("kube-virt-container-rhel-10"): encoded(kubeVirtTestDigest2),
				cacheRef("kube-virt-container-rhel-9"):  encoded(kubeVirtTestDigest1),
				cacheRef("kube-virt-container"):         encoded(kubeVirtTestDigest1),
			},
			expected: streamCopies,
		},
		{
			name:       "older archive: the legacy image is copied to the destination of its stream only",
			releaseDir: singleArchRelease,
			cache:      map[string]string{cacheRef("kube-virt-container"): encoded(kubeVirtTestDigest1)},
			expected: []v2alpha1.CopyImageSchema{
				kubeVirtCopy(kubeVirtTestDigest2, "kube-virt-container-rhel-10", "kube-virt-container-rhel-10"),
				kubeVirtCopy(kubeVirtTestDigest1, "kube-virt-container", "kube-virt-container-rhel-9"),
			},
		},
		{
			name:       "legacy image with another digest is not copied",
			releaseDir: singleArchRelease,
			cache:      map[string]string{cacheRef("kube-virt-container"): encoded(kubeVirtTestDigest4)},
			expected:   streamCopies,
		},
		{
			name:       "archive without the legacy image",
			releaseDir: singleArchRelease,
			cache:      map[string]string{},
			expected:   streamCopies,
		},
		{
			name:       "older multi-arch archive: the legacy image of each architecture is used",
			platform:   multiArchPlatform,
			releaseDir: multiArchRelease,
			cache: map[string]string{
				cacheRef("kube-virt-container-x86_64"): encoded(kubeVirtTestDigest1),
				cacheRef("kube-virt-container-s390x"):  encoded(kubeVirtTestDigest3),
			},
			expected: []v2alpha1.CopyImageSchema{
				kubeVirtCopy(kubeVirtTestDigest1, "kube-virt-container-x86_64", "kube-virt-container-x86_64-rhel-9"),
				kubeVirtCopy(kubeVirtTestDigest3, "kube-virt-container-s390x", "kube-virt-container-s390x-rhel-9"),
			},
		},
		{
			name:       "older multi-arch archive: the legacy image of another architecture is not used",
			platform:   multiArchPlatform,
			releaseDir: multiArchRelease,
			cache:      map[string]string{cacheRef("kube-virt-container-x86_64"): encoded(kubeVirtTestDigest3)},
			expected: []v2alpha1.CopyImageSchema{
				kubeVirtCopy(kubeVirtTestDigest1, "kube-virt-container-x86_64-rhel-9", "kube-virt-container-x86_64-rhel-9"),
				kubeVirtCopy(kubeVirtTestDigest3, "kube-virt-container-s390x-rhel-9", "kube-virt-container-s390x-rhel-9"),
			},
		},
		{
			name:       "delete mode does not read the cache",
			releaseDir: singleArchRelease,
			deleteMode: true,
			cache:      map[string]string{cacheRef("kube-virt-container"): encoded(kubeVirtTestDigest1)},
			expected:   streamCopies,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platform := tt.platform
			if !platform.KubeVirtContainer {
				platform = v2alpha1.Platform{KubeVirtContainer: true}
			}
			cache := &kubeVirtTestCache{digests: tt.cache}
			col := LocalStorageCollector{
				Log: clog.New("trace"),
				Config: v2alpha1.ImageSetConfiguration{
					ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
						Mirror: v2alpha1.Mirror{Platform: platform},
					},
				},
				LocalStorageFQDN: "localhost:9999",
				Manifest:         cache,
				Opts:             kubeVirtTestD2MOptions(),
			}
			if tt.deleteMode {
				col.Opts.Function = string(mirror.DeleteMode)
			}

			batch, err := col.prepareReleaseBatchFromDir(context.Background(), tt.releaseDir)
			require.NoError(t, err)
			require.Equal(t, tt.expected, kubeVirtTestCopies(batch))
			if tt.deleteMode {
				require.Zero(t, cache.lookups)
			}
		})
	}
}

// kubeVirtTestCache serves the manifest digests of the images of a synthetic local cache.
type kubeVirtTestCache struct {
	MockManifest
	digests map[string]string
	lookups int
}

func (c *kubeVirtTestCache) ImageDigest(_ context.Context, _ *types.SystemContext, imgRef string) (string, error) {
	c.lookups++
	if dgst, ok := c.digests[imgRef]; ok {
		return dgst, nil
	}
	return "", fmt.Errorf("manifest unknown: %s", imgRef)
}

// kubeVirtTestD2MOptions returns the copy options of a disk to mirror run to localhost:5000/test.
func kubeVirtTestD2MOptions() mirror.CopyOptions {
	global := &mirror.GlobalOptions{}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	return mirror.CopyOptions{
		Global:      global,
		SrcImage:    srcOpts,
		Destination: consts.DockerProtocol + "localhost:5000/test",
		Mode:        mirror.DiskToMirror,
	}
}

// kubeVirtTestCopies returns the copies of the synthetic kubevirt images of a copy batch.
func kubeVirtTestCopies(batch []v2alpha1.CopyImageSchema) []v2alpha1.CopyImageSchema {
	var copies []v2alpha1.CopyImageSchema
	for _, c := range batch {
		if strings.HasPrefix(c.Origin, kubeVirtTestRepo+"@") {
			copies = append(copies, c)
		}
	}
	return copies
}

// kubeVirtTestStream returns the JSON document of a synthetic CoreOS stream with a
// kubevirt image digest for each given bootimages architecture key.
func kubeVirtTestStream(t *testing.T, digestByArch map[string]string) string {
	t.Helper()
	architectures := map[string]v2alpha1.ArchImages{}
	for arch, dgst := range digestByArch {
		architectures[arch] = v2alpha1.ArchImages{
			Images: v2alpha1.Images{Kubevirt: v2alpha1.Kubevirt{DigestRef: kubeVirtTestRepo + "@" + dgst}},
		}
	}
	data, err := json.Marshal(map[string]any{"architectures": architectures})
	require.NoError(t, err)
	return string(data)
}

// writeKubeVirtTestBootImages writes a synthetic bootimages ConfigMap with the given
// data.stream and data.streams values to a temporary release directory and returns
// the directory. Empty values are left out of the ConfigMap.
func writeKubeVirtTestBootImages(t *testing.T, stream, streams string) string {
	t.Helper()
	data := map[string]string{"releaseVersion": "0.0.1"}
	if stream != "" {
		data["stream"] = stream
	}
	if streams != "" {
		data["streams"] = streams
	}
	configMap, err := yaml.Marshal(map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]string{"name": "coreos-bootimages"},
		"data":       data,
	})
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), kubeVirtTestReleaseTag)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, releaseManifests), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, releaseBootableImagesFullPath), configMap, 0o600))
	return dir
}

func kubeVirtTestImage(dgst, name string) v2alpha1.RelatedImage {
	return v2alpha1.RelatedImage{Image: kubeVirtTestRepo + "@" + dgst, Name: name, Type: v2alpha1.TypeOCPReleaseContent}
}

func TestBuildReleasePlatformFilters(t *testing.T) {
	platforms := []v2alpha1.InstancePlatformFilter{
		{OS: "linux", Architecture: "amd64"},
		{OS: "linux", Architecture: "arm64"},
	}

	allImages := []v2alpha1.CopyImageSchema{
		{Origin: "docker://registry/release:latest", Type: v2alpha1.TypeOCPRelease},
		{Origin: "docker://registry/component:latest", Type: v2alpha1.TypeOCPReleaseContent},
		{Origin: "docker://registry/graph:latest", Type: v2alpha1.TypeCincinnatiGraph},
	}

	filters := buildReleasePlatformFilters(allImages, platforms)

	// Release and component images must have platform filters applied.
	assert.Equal(t, platforms, filters["docker://registry/release:latest"])
	assert.Equal(t, platforms, filters["docker://registry/component:latest"])

	// Graph images must always be excluded — they are not platform-filtered.
	_, graphPresent := filters["docker://registry/graph:latest"]
	assert.False(t, graphPresent, "TypeCincinnatiGraph images must not appear in platform filters")
}
