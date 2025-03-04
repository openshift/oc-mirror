package release

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
	imgbuildermock "github.com/openshift/oc-mirror/v2/internal/pkg/imagebuilder/mock"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	manifestmock "github.com/openshift/oc-mirror/v2/internal/pkg/manifest/mock"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	mirrormock "github.com/openshift/oc-mirror/v2/internal/pkg/mirror/mock"
	releasemock "github.com/openshift/oc-mirror/v2/internal/pkg/release/mock"
)

func TestReleaseLocalStoredCollector(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	defaultImageIndex := &v2alpha1.OCISchema{
		SchemaVersion: 2,
		Manifests: []v2alpha1.OCIManifest{
			{
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
				Size:      567,
			},
		},
	}

	defaultImageManifest := &v2alpha1.OCISchema{
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
	}

	defaultReleaseSchema := []v2alpha1.RelatedImage{
		{Name: "agent-installer-api-server", Type: v2alpha1.TypeOCPReleaseContent, Image: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e4182331e"},
		{Name: "agent-installer-node-agent", Type: v2alpha1.TypeOCPReleaseContent, Image: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:955faaa822dc107f4dffa6a7e457f8d57a65d10949f74f6780ddd63c115e31e5"},
		{Name: "agent-installer-orchestrator", Type: v2alpha1.TypeOCPReleaseContent, Image: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4949b93b3fd0f6b22197402ba22c2775eba408b53d30ac2e3ab2dda409314f5e"},
		{Name: "apiserver-network-proxy", Type: v2alpha1.TypeOCPReleaseContent, Image: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:2a0dd75b1b327a0c5b17145fc71beb2bf805e6cc3b8fc3f672ce06772caddf21"},
	}

	// this test should cover over 80% M2D
	t.Run("Testing ReleaseImageCollector - Mirror to disk: should pass", func(t *testing.T) {
		ctx := context.Background()

		manifestMock := manifestmock.NewMockManifestInterface(mockCtrl)

		manifestMock.
			EXPECT().
			GetImageIndex(gomock.Any()).
			Return(defaultImageIndex, nil).
			AnyTimes()

		manifestMock.
			EXPECT().
			GetImageManifest(gomock.Any()).
			Return(defaultImageManifest, nil).
			AnyTimes()

		manifestMock.
			EXPECT().
			ExtractLayersOCI(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		manifestMock.
			EXPECT().
			GetReleaseSchema(gomock.Any()).
			Return(defaultReleaseSchema, nil).
			AnyTimes()

		mirrorMock := mirrormock.NewMockMirrorInterface(mockCtrl)

		mirrorMock.
			EXPECT().
			Run(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		cincinnatiMock := releasemock.NewMockCincinnatiInterface(mockCtrl)

		cincinnatiMock.
			EXPECT().
			GetReleaseReferenceImages(gomock.Any()).
			Return(
				[]v2alpha1.CopyImageSchema{
					{
						Type:   v2alpha1.TypeOCPRelease,
						Source: "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
						Origin: "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
					},
				}, nil,
			).
			AnyTimes()

		imgbuilderMock := imgbuildermock.NewMockImageBuilderInterface(mockCtrl)

		imgbuilderMock.
			EXPECT().
			BuildAndPush(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return("sha256:12345", nil).
			AnyTimes()

		imgbuilderMock.
			EXPECT().
			SaveImageLayoutToDir(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(layout.FromPath(common.TestFolder + "test-untar")).
			AnyTimes()

		ex := setupCollector_MirrorToDisk(tempDir, log, mockCtrl)
		ex.Manifest = manifestMock
		ex.Mirror = mirrorMock
		ex.Cincinnati = cincinnatiMock
		ex.ImageBuilder = imgbuilderMock

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

		ex := setupCollector_DiskToMirror(tempDir, log, mockCtrl)

		manifestMock := manifestmock.NewMockManifestInterface(mockCtrl)

		manifestMock.
			EXPECT().
			GetReleaseSchema(gomock.Any()).
			Return(defaultReleaseSchema, nil).
			AnyTimes()

		manifestMock.
			EXPECT().
			GetDigest(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419", nil).
			AnyTimes()

		ex.Manifest = manifestMock // override default empty mock

		cincinnatiMock := releasemock.NewMockCincinnatiInterface(mockCtrl)
		cincinnatiMock.
			EXPECT().
			GetReleaseReferenceImages(gomock.Any()).
			Return(
				[]v2alpha1.CopyImageSchema{
					{
						Type:   v2alpha1.TypeOCPRelease,
						Source: "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
						Origin: "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
					},
				}, nil,
			).AnyTimes()

		ex.Cincinnati = cincinnatiMock

		// copy tests/hold-test-fake to working-dir
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

		manifestMock := manifestmock.NewMockManifestInterface(mockCtrl)

		manifestMock.
			EXPECT().
			GetReleaseSchema(gomock.Any()).
			Return(defaultReleaseSchema, nil).
			AnyTimes()

		manifestMock.
			EXPECT().
			GetDigest(gomock.Any(), gomock.Any(), gomock.Any()).
			Return("3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419", nil).
			AnyTimes()

		ex := setupCollector_DiskToMirror(tempDir, log, mockCtrl)
		ex.Manifest = manifestMock

		client := &ocpClient{}
		client.SetQueryParams(ex.Config.Mirror.Platform.Architectures[0], ex.Config.Mirror.Platform.Channels[0].Name, "")

		cincinnatiMock := releasemock.NewMockCincinnatiInterface(mockCtrl)

		cincinnatiMock.
			EXPECT().
			GetReleaseReferenceImages(gomock.Any()).
			Return(
				[]v2alpha1.CopyImageSchema{
					{
						Type:   v2alpha1.TypeOCPRelease,
						Source: "quay.io/openshift-release-dev/ocp-release:v4.13.10",
						Origin: "quay.io/openshift-release-dev/ocp-release:v4.13.10",
					},
				}, nil,
			).
			AnyTimes()

		ex.Cincinnati = cincinnatiMock

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
		manifestMock := manifestmock.NewMockManifestInterface(mockCtrl)

		manifestMock.
			EXPECT().
			GetImageIndex(gomock.Any()).
			Return(nil, errors.New("force-fail image index")).
			AnyTimes()

		cincinnatiMock := releasemock.NewMockCincinnatiInterface(mockCtrl)
		cincinnatiMock.
			EXPECT().
			GetReleaseReferenceImages(gomock.Any()).
			Return(
				[]v2alpha1.CopyImageSchema{
					{
						Type:   v2alpha1.TypeOCPRelease,
						Source: "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
						Origin: "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
					},
				}, nil,
			).
			AnyTimes()

		ex := setupCollector_MirrorToDisk(tempDir, log, mockCtrl)
		ex.Manifest = manifestMock
		ex.Cincinnati = cincinnatiMock

		res, err := ex.ReleaseImageCollector(context.Background())
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail image manifest", func(t *testing.T) {
		manifestMock := manifestmock.NewMockManifestInterface(mockCtrl)

		manifestMock.
			EXPECT().
			GetImageIndex(gomock.Any()).
			Return(defaultImageIndex, nil).
			AnyTimes()

		manifestMock.
			EXPECT().
			GetImageManifest(gomock.Any()).
			Return(nil, errors.New("force-fail image manifest")).
			AnyTimes()

		cincinnatiMock := releasemock.NewMockCincinnatiInterface(mockCtrl)

		cincinnatiMock.
			EXPECT().
			GetReleaseReferenceImages(gomock.Any()).
			Return(
				[]v2alpha1.CopyImageSchema{
					{
						Type:   v2alpha1.TypeOCPRelease,
						Source: "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
						Origin: "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
					},
				}, nil,
			).
			AnyTimes()

		ex := setupCollector_MirrorToDisk(tempDir, log, mockCtrl)
		ex.Manifest = manifestMock
		ex.Cincinnati = cincinnatiMock

		res, err := ex.ReleaseImageCollector(context.Background())
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail extract", func(t *testing.T) {
		manifestMock := manifestmock.NewMockManifestInterface(mockCtrl)

		manifestMock.
			EXPECT().
			GetImageIndex(gomock.Any()).
			Return(defaultImageIndex, nil).
			AnyTimes()

		manifestMock.
			EXPECT().
			GetImageManifest(gomock.Any()).
			Return(defaultImageManifest, nil).
			AnyTimes()

		manifestMock.
			EXPECT().
			ExtractLayersOCI(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("force-fail extract OCI")).
			AnyTimes()

		cincinnatiMock := releasemock.NewMockCincinnatiInterface(mockCtrl)

		cincinnatiMock.
			EXPECT().
			GetReleaseReferenceImages(gomock.Any()).
			Return(
				[]v2alpha1.CopyImageSchema{
					{
						Type:   v2alpha1.TypeOCPRelease,
						Source: "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
						Origin: "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64",
					},
				}, nil,
			).
			AnyTimes()

		ex := setupCollector_MirrorToDisk(tempDir, log, mockCtrl)
		ex.Manifest = manifestMock
		ex.Cincinnati = cincinnatiMock

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
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	t.Run("Testing GraphImage : should fail", func(t *testing.T) {
		ex := setupCollector_DiskToMirror(tempDir, log, mockCtrl)

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
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	t.Run("Testing ReleaseImage : should pass", func(t *testing.T) {
		os.RemoveAll(common.TestFolder + "hold-release/")
		os.RemoveAll(common.TestFolder + "release-images")
		os.RemoveAll(common.TestFolder + "tmp/")

		ex := setupCollector_DiskToMirror(tempDir, log, mockCtrl)

		cincinnatiMock := releasemock.NewMockCincinnatiInterface(mockCtrl)
		cincinnatiMock.
			EXPECT().
			GetReleaseReferenceImages(gomock.Any()).
			Return(
				[]v2alpha1.CopyImageSchema{
					{
						Type:   v2alpha1.TypeOCPRelease,
						Source: "quay.io/openshift-release-dev/ocp-release:v4.13.10",
						Origin: "quay.io/openshift-release-dev/ocp-release:v4.13.10",
					},
				}, nil,
			).AnyTimes()

		ex.Cincinnati = cincinnatiMock

		// copy tests/hold-test-fake to working-dir
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

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			manifestMock := manifestmock.NewMockManifestInterface(mockCtrl)

			manifestMock.
				EXPECT().
				GetDigest(gomock.Any(), gomock.Any(), gomock.Eq("docker://localhost:9999/openshift/graph-image:latest")).
				DoAndReturn(func(context.Context, any, string) (string, error) {
					if testCase.imageInCache {
						return "123456", nil
					}
					return "", errors.New("force-fail image digest")
				}).
				AnyTimes()

			manifestMock.
				EXPECT().
				GetDigest(gomock.Any(), gomock.Any(), fmt.Sprintf("oci://%s/%s", copyOpts.Global.WorkingDir, graphPreparationDir)).
				DoAndReturn(func(context.Context, any, string) (string, error) {
					if testCase.imageInWorkingDir {
						return "123456", nil
					}
					return "", errors.New("force-fail image digest")
				}).
				AnyTimes()

			imgbuilderMock := imgbuildermock.NewMockImageBuilderInterface(mockCtrl)

			imgbuilderMock.
				EXPECT().
				BuildAndPush(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return("sha256:12345", nil).
				AnyTimes()

			imgbuilderMock.
				EXPECT().
				SaveImageLayoutToDir(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(layout.FromPath(common.TestFolder + "test-untar")).
				AnyTimes()

			if testCase.updateURLOverride != "" {
				t.Setenv("UPDATE_URL_OVERRIDE", testCase.updateURLOverride)
			}
			ex := &LocalStorageCollector{
				Log:              log,
				Mirror:           mirrormock.NewMockMirrorInterface(mockCtrl),
				Opts:             copyOpts,
				Manifest:         manifestMock,
				LocalStorageFQDN: "localhost:9999",
				ImageBuilder:     imgbuilderMock,
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

func setupCollector_DiskToMirror(tempDir string, log clog.PluggableLoggerInterface, mockCtrl *gomock.Controller) *LocalStorageCollector {
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

	ex := &LocalStorageCollector{
		Log:              log,
		Mirror:           mirrormock.NewMockMirrorInterface(mockCtrl),
		Config:           cfgd2m,
		Manifest:         manifestmock.NewMockManifestInterface(mockCtrl),
		Opts:             d2mOpts,
		Cincinnati:       releasemock.NewMockCincinnatiInterface(mockCtrl),
		LocalStorageFQDN: "localhost:9999",
		LogsDir:          "/tmp/",
	}

	return ex
}

func setupCollector_MirrorToDisk(tempDir string, log clog.PluggableLoggerInterface, mockCtrl *gomock.Controller) *LocalStorageCollector {
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

	ex := &LocalStorageCollector{
		Log:              log,
		Mirror:           mirrormock.NewMockMirrorInterface(mockCtrl),
		Config:           cfgm2d,
		Manifest:         manifestmock.NewMockManifestInterface(mockCtrl),
		Opts:             m2dOpts,
		Cincinnati:       releasemock.NewMockCincinnatiInterface(mockCtrl),
		LocalStorageFQDN: "localhost:9999",
		ImageBuilder:     imgbuildermock.NewMockImageBuilderInterface(mockCtrl),
		LogsDir:          "/tmp/",
	}
	return ex
}
