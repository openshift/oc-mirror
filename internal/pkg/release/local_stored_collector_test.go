package release

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/fake"
	"github.com/google/uuid"
	digest "github.com/opencontainers/go-digest"
	"github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
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
	defer os.RemoveAll(tempDir)

	// this test should cover over 80% M2D
	t.Run("Testing ReleaseImageCollector - Mirror to disk: should pass", func(t *testing.T) {
		ctx := context.Background()
		manifest := &MockManifest{Log: log}

		ex := setupCollector_MirrorToDisk(tempDir, log, manifest)

		err := copy.Copy(common.TestFolder+"working-dir-fake/hold-release/ocp-release/4.14.1-x86_64", filepath.Join(ex.Opts.Global.WorkingDir, releaseImageExtractDir, "ocp-release/4.13.10-x86_64"))
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
				Source:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e4182331e",
				Destination: "docker://localhost:9999/openshift/release:4.13.10-x86_64-agent-installer-api-server",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e4182331e",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Source:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:955faaa822dc107f4dffa6a7e457f8d57a65d10949f74f6780ddd63c115e31e5",
				Destination: "docker://localhost:9999/openshift/release:4.13.10-x86_64-agent-installer-node-agent",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:955faaa822dc107f4dffa6a7e457f8d57a65d10949f74f6780ddd63c115e31e5",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Source:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4949b93b3fd0f6b22197402ba22c2775eba408b53d30ac2e3ab2dda409314f5e",
				Destination: "docker://localhost:9999/openshift/release:4.13.10-x86_64-agent-installer-orchestrator",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4949b93b3fd0f6b22197402ba22c2775eba408b53d30ac2e3ab2dda409314f5e",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Source:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:2a0dd75b1b327a0c5b17145fc71beb2bf805e6cc3b8fc3f672ce06772caddf21",
				Destination: "docker://localhost:9999/openshift/release:4.13.10-x86_64-apiserver-network-proxy",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:2a0dd75b1b327a0c5b17145fc71beb2bf805e6cc3b8fc3f672ce06772caddf21",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Type:        v2alpha1.TypeOCPReleaseContent,
				Source:      "docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:729265d5ef6ed6a45bcd55c46877e3acb9eae3f49c78cd795d5b53aa85e3775b",
				Destination: "docker://localhost:9999/openshift/release:4.13.10-x86_64-kube-virt-container",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:729265d5ef6ed6a45bcd55c46877e3acb9eae3f49c78cd795d5b53aa85e3775b",
			},
			{
				Source:      "docker://quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
				Destination: "docker://localhost:9999/openshift/release-images:4.13.10-x86_64",
				Origin:      "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
				Type:        v2alpha1.TypeOCPRelease,
			},
			{
				Source:      "docker://localhost:9999/openshift/graph-image:latest",
				Destination: "docker://localhost:9999/openshift/graph-image:latest",
				Origin:      "docker://localhost:9999/openshift/graph-image:latest",
				Type:        v2alpha1.TypeCincinnatiGraph,
			},
		}
		assert.ElementsMatch(t, expected, res)
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector - Disk to mirror : should pass", func(t *testing.T) {

		os.RemoveAll(common.TestFolder + "hold-release/")
		os.RemoveAll(common.TestFolder + "release-images")
		os.RemoveAll(common.TestFolder + "tmp/")

		ex := setupCollector_DiskToMirror(tempDir, log)
		//copy tests/hold-test-fake to working-dir
		err := copy.Copy(common.TestFolder+"working-dir-fake/hold-release/ocp-release/4.14.1-x86_64", filepath.Join(ex.Opts.Global.WorkingDir, releaseImageExtractDir, "ocp-release/4.13.10-x86_64"))
		if err != nil {
			t.Fatalf("should not fail")
		}

		res, err := ex.ReleaseImageCollector(context.Background())
		if err != nil {
			t.Fatalf("should not fail: %v", err)
		}
		if len(res) == 0 {
			t.Fatalf("should contain at least 1 image")
		}
		if !strings.Contains(res[0].Source, ex.LocalStorageFQDN) {
			t.Fatalf("source images should be from local storage")
		}
		// must contain 4 release component images
		// must contain 1 graph image
		// must contain 1 release image
		// must contain 1 kubevirt image
		expected := []v2alpha1.CopyImageSchema{
			{
				Source:      "docker://localhost:9999/openshift/release:4.13.10-x86_64-agent-installer-api-server",
				Destination: "docker://localhost:5000/test/openshift/release:4.13.10-x86_64-agent-installer-api-server",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e4182331e",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Source:      "docker://localhost:9999/openshift/release:4.13.10-x86_64-agent-installer-node-agent",
				Destination: "docker://localhost:5000/test/openshift/release:4.13.10-x86_64-agent-installer-node-agent",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:955faaa822dc107f4dffa6a7e457f8d57a65d10949f74f6780ddd63c115e31e5",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Source:      "docker://localhost:9999/openshift/release:4.13.10-x86_64-agent-installer-orchestrator",
				Destination: "docker://localhost:5000/test/openshift/release:4.13.10-x86_64-agent-installer-orchestrator",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4949b93b3fd0f6b22197402ba22c2775eba408b53d30ac2e3ab2dda409314f5e",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Source:      "docker://localhost:9999/openshift/release:4.13.10-x86_64-apiserver-network-proxy",
				Destination: "docker://localhost:5000/test/openshift/release:4.13.10-x86_64-apiserver-network-proxy",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:2a0dd75b1b327a0c5b17145fc71beb2bf805e6cc3b8fc3f672ce06772caddf21",
				Type:        v2alpha1.TypeOCPReleaseContent,
			},
			{
				Type:        v2alpha1.TypeOCPReleaseContent,
				Source:      "docker://localhost:9999/openshift/release:4.13.10-x86_64-kube-virt-container",
				Destination: "docker://localhost:5000/test/openshift/release:4.13.10-x86_64-kube-virt-container",
				Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:729265d5ef6ed6a45bcd55c46877e3acb9eae3f49c78cd795d5b53aa85e3775b",
			},
			{
				Source:      "docker://localhost:9999/openshift/release-images:4.13.10-x86_64",
				Destination: "docker://localhost:5000/test/openshift/release-images:4.13.10-x86_64",
				Origin:      "docker://quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
				Type:        v2alpha1.TypeOCPRelease,
			},
			{
				Source:      "docker://localhost:9999/openshift/graph-image:latest",
				Destination: "docker://localhost:5000/test/openshift/graph-image:latest",
				Origin:      "docker://localhost:9999/openshift/graph-image:latest",
				Type:        v2alpha1.TypeCincinnatiGraph,
			},
		}
		assert.ElementsMatch(t, expected, res)
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector with real GetReleaseReferenceImages - Disk to mirror : should pass", func(t *testing.T) {

		os.RemoveAll(common.TestFolder + "hold-release/")
		os.RemoveAll(common.TestFolder + "release-images")
		os.RemoveAll(common.TestFolder + "tmp/")

		ex := setupCollector_DiskToMirror(tempDir, log)

		client := &ocpClient{}
		client.SetQueryParams(ex.Config.Mirror.Platform.Architectures[0], ex.Config.Mirror.Platform.Channels[0].Name, "")
		sig := MockCincinnati{}
		cn := NewCincinnati(ex.Log, nil, &ex.Config, ex.Opts, client, false, sig)

		ex.Cincinnati = cn

		ex.Opts.Global.WorkingDir = filepath.Join(common.TestFolder, "working-dir-fake")

		res, err := ex.ReleaseImageCollector(context.Background())
		if err != nil {
			t.Fatalf("should not fail: %v", err)
		}
		if len(res) == 0 {
			t.Fatalf("should contain at least 1 image")
		}
		if !strings.Contains(res[0].Source, ex.LocalStorageFQDN) {
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
	defer os.RemoveAll(tempDir)
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
	defer os.RemoveAll(tempDir)
	t.Run("Testing ReleaseImage : should pass", func(t *testing.T) {
		os.RemoveAll(common.TestFolder + "hold-release/")
		os.RemoveAll(common.TestFolder + "release-images")
		os.RemoveAll(common.TestFolder + "tmp/")

		ex := setupCollector_DiskToMirror(tempDir, log)
		//copy tests/hold-test-fake to working-dir
		err := copy.Copy(common.TestFolder+"working-dir-fake/hold-release/ocp-release/4.14.1-x86_64", filepath.Join(ex.Opts.Global.WorkingDir, releaseImageExtractDir, "ocp-release/4.13.9-x86_64"))
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
				Source:      "docker://mymirror/openshift/graph-image:latest",
				Destination: "docker://mymirror/openshift/graph-image:latest",
				Origin:      "docker://mymirror/openshift/graph-image:latest",
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
				Source:      "docker://localhost:9999/openshift/graph-image:latest",
				Destination: "docker://localhost:9999/openshift/graph-image:latest",
				Origin:      "docker://localhost:9999/openshift/graph-image:latest",
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
				Source:      "oci://TEMPDIR/" + graphPreparationDir,
				Destination: "docker://mymirror/openshift/graph-image:latest",
				Origin:      "oci://TEMPDIR/" + graphPreparationDir,
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
				Source:      "oci://TEMPDIR/" + graphPreparationDir,
				Destination: "docker://localhost:9999/openshift/graph-image:latest",
				Origin:      "oci://TEMPDIR/" + graphPreparationDir,
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
				copyOpts.Destination = "file://" + tempDir
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
				manifestMock.On("ImageDigest", mock.Anything, mock.Anything, "oci://"+copyOpts.Global.WorkingDir+"/"+graphPreparationDir).Return("123456", nil)
			} else {
				manifestMock.On("ImageDigest", mock.Anything, mock.Anything, "oci://"+copyOpts.Global.WorkingDir+"/"+graphPreparationDir).Return("", fmt.Errorf("simulating image doesn't exist in cache"))
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
		Destination:         "file://test",
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
				AdditionalImages: []v2alpha1.Image{
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
				BlockedImages: []v2alpha1.Image{
					{Name: "alpine"},
					{Name: "redis"},
				},
				Samples: []v2alpha1.SampleImages{
					{Image: v2alpha1.Image{Name: "ruby"}},
					{Image: v2alpha1.Image{Name: "python"}},
					{Image: v2alpha1.Image{Name: "nginx"}},
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

func (o MockManifest) GetOCIImageIndex(name string) (*v2alpha1.OCISchema, error) {
	if o.FailImageIndex {
		return &v2alpha1.OCISchema{}, fmt.Errorf("forced error image index")
	}
	return &v2alpha1.OCISchema{
		SchemaVersion: 2,
		Manifests: []v2alpha1.OCIManifest{
			{
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
				Size:      567,
			},
		},
	}, nil
}

func (o MockManifest) GetOCIImageManifest(name string) (*v2alpha1.OCISchema, error) {
	if o.FailImageManifest {
		return &v2alpha1.OCISchema{}, fmt.Errorf("forced error image index")
	}

	return &v2alpha1.OCISchema{
		SchemaVersion: 2,
		Manifests: []v2alpha1.OCIManifest{
			{
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
				Size:      567,
			},
		},
		Config: v2alpha1.OCIManifest{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
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

func (o MockManifest) ConvertOCIIndexToSingleManifest(dir string, oci *v2alpha1.OCISchema) error {
	return nil
}

func (o MockManifest) ImageDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	return "3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419", nil
}

func (o MockManifest) ImageManifest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, instanceDigest *digest.Digest) ([]byte, string, error) {
	return nil, "", nil
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

func (o *ManifestMock) GetOCIImageIndex(dir string) (*v2alpha1.OCISchema, error) {
	return &v2alpha1.OCISchema{}, nil
}

func (o *ManifestMock) GetOCIImageManifest(file string) (*v2alpha1.OCISchema, error) {
	return &v2alpha1.OCISchema{}, nil
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

func (o *ManifestMock) ConvertOCIIndexToSingleManifest(dir string, oci *v2alpha1.OCISchema) error {
	return nil
}

func (o *ManifestMock) ImageDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	args := o.Called(ctx, sourceCtx, imgRef)
	return args.String(0), args.Error(1)
}

func (o *ManifestMock) ImageManifest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, instanceDigest *digest.Digest) ([]byte, string, error) {
	return nil, "", nil
}
