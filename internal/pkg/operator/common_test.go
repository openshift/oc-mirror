package operator

import (
	"path/filepath"
	"testing"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	"github.com/stretchr/testify/assert"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func TestPrepareDeleteForV1(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()

	type testCase struct {
		caseName       string
		relatedImages  map[string][]v2alpha1.RelatedImage
		expectedResult []v2alpha1.CopyImageSchema
		expectedError  bool
	}

	testCases := []testCase{
		{
			caseName: "OperatorImageCollector - Mirror to Mirror: related images by digest should pass",
			relatedImages: map[string][]v2alpha1.RelatedImage{
				"operatorA": {
					{ //=localhost:5000/43731/openshift4/ose-kube-rbac-proxy:5574585a

						Name:  "testA",
						Image: "registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:7efeeb8b29872a6f0271f651d7ae02c91daea16d853c50e374c310f044d8c76c",
						Type:  v2alpha1.TypeOperatorBundle,
					},
					{
						Name:  "testB",
						Image: "registry.redhat.io/openshift-sandboxed-containers/osc-operator-bundle@sha256:8da62ba1c19c905bc1b87a6233ead475b047a766dc2acb7569149ac5cfe7f0f1",
						Type:  v2alpha1.TypeOperatorRelatedImage,
					},
				},
			},
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "localhost:9999/openshift4/ose-kube-rbac-proxy:sha256-7efeeb8b29872a6f0271f651d7ae02c91daea16d853c50e374c310f044d8c76c",
					Destination: consts.DockerProtocol + "localhost:5000/test/openshift4/ose-kube-rbac-proxy:5574585a",
					Origin:      consts.DockerProtocol + "registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:7efeeb8b29872a6f0271f651d7ae02c91daea16d853c50e374c310f044d8c76c",
					Type:        v2alpha1.TypeOperatorBundle,
				},
				{
					Source:      consts.DockerProtocol + "localhost:9999/openshift-sandboxed-containers/osc-operator-bundle:sha256-8da62ba1c19c905bc1b87a6233ead475b047a766dc2acb7569149ac5cfe7f0f1",
					Destination: consts.DockerProtocol + "localhost:5000/test/openshift-sandboxed-containers/osc-operator-bundle:1adce9f",
					Origin:      consts.DockerProtocol + "registry.redhat.io/openshift-sandboxed-containers/osc-operator-bundle@sha256:8da62ba1c19c905bc1b87a6233ead475b047a766dc2acb7569149ac5cfe7f0f1",
					Type:        v2alpha1.TypeOperatorRelatedImage,
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			ex := setupFilterCollector_MirrorToDisk(tempDir, log, &MockManifest{})
			ex.Opts.Mode = mirror.MirrorToMirror
			ex.generateV1DestTags = true
			ex.Opts.Destination = consts.DockerProtocol + "localhost:5000/test"
			res, err := ex.prepareD2MCopyBatch(testCase.relatedImages)
			if testCase.expectedError && err == nil {
				t.Fatalf("should fail")
			}
			if !testCase.expectedError && err != nil {
				t.Fatal("should not fail")
			}
			assert.ElementsMatch(t, testCase.expectedResult, res)
		})
	}
}

func TestPrepareM2MCopyBatch(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()

	type testCase struct {
		caseName       string
		relatedImages  map[string][]v2alpha1.RelatedImage
		expectedResult []v2alpha1.CopyImageSchema
		expectedError  bool
	}

	testCases := []testCase{
		{
			caseName: "OperatorImageCollector - Mirror to Mirror: related images by digest should pass",
			relatedImages: map[string][]v2alpha1.RelatedImage{
				"operatorA": {
					{
						Name:  "testA",
						Image: "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
						Type:  v2alpha1.TypeOperatorBundle,
					},
					{
						Name:  "testB",
						Image: "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
						Type:  v2alpha1.TypeOperatorRelatedImage,
					},
				},
			},
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: consts.DockerProtocol + "localhost:5000/test/sometestimage-a:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      consts.DockerProtocol + "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeOperatorBundle,
				},
				{
					Source:      consts.DockerProtocol + "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: consts.DockerProtocol + "localhost:5000/test/sometestimage-b:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      consts.DockerProtocol + "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeOperatorRelatedImage,
				},
			},
		},
		{
			caseName: "OperatorImageCollector - Mirror to Mirror: related image by digest and tag should pass",
			relatedImages: map[string][]v2alpha1.RelatedImage{
				"operatorB": {
					{
						Name:  "kube-rbac-proxy",
						Image: "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
						Type:  v2alpha1.TypeOperatorRelatedImage,
					},
				},
			},
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "gcr.io/kubebuilder/kube-rbac-proxy@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Destination: consts.DockerProtocol + "localhost:5000/test/kubebuilder/kube-rbac-proxy:v0.13.1",
					Origin:      consts.DockerProtocol + "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Type:        v2alpha1.TypeOperatorRelatedImage,
				},
			},
		},
		{
			caseName: "OperatorImageCollector - Mirror to Mirror: catalog image nominal should pass",
			relatedImages: map[string][]v2alpha1.RelatedImage{
				"redhat-operator-index.045638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea": {
					{
						Image:      "registry.redhat.io/redhat/redhat-operator-index:v4.17",
						Type:       v2alpha1.TypeOperatorCatalog,
						RebuiltTag: "fbf7a9e933d930758fcf18e1c6e6deff3",
					},
				},
			},
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "registry.redhat.io/redhat/redhat-operator-index:v4.17",
					Destination: consts.DockerProtocol + "localhost:9999/redhat/redhat-operator-index:v4.17",
					Origin:      consts.DockerProtocol + "registry.redhat.io/redhat/redhat-operator-index:v4.17",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "fbf7a9e933d930758fcf18e1c6e6deff3",
				},
				{
					Source:      consts.DockerProtocol + "localhost:9999/redhat/redhat-operator-index:fbf7a9e933d930758fcf18e1c6e6deff3",
					Destination: consts.DockerProtocol + "localhost:5000/test/redhat/redhat-operator-index:v4.17",
					Origin:      consts.DockerProtocol + "registry.redhat.io/redhat/redhat-operator-index:v4.17",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "fbf7a9e933d930758fcf18e1c6e6deff3",
				},
			},
		},
		{
			caseName: "OperatorImageCollector - Mirror to Mirror: catalog image - targetTag should pass",
			relatedImages: map[string][]v2alpha1.RelatedImage{
				"redhat-operator-index.543218f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea": {
					{
						Image:      "registry.redhat.io/redhat/certified-operators:v4.10",
						Type:       v2alpha1.TypeOperatorCatalog,
						TargetTag:  "v4.10.0",
						RebuiltTag: "543219e933d930758fcf18e1c6e6deff3",
					},
				},
			},
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "registry.redhat.io/redhat/certified-operators:v4.10",
					Destination: consts.DockerProtocol + "localhost:9999/redhat/certified-operators:v4.10.0",
					Origin:      consts.DockerProtocol + "registry.redhat.io/redhat/certified-operators:v4.10",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "543219e933d930758fcf18e1c6e6deff3",
				},
				{
					Source:      consts.DockerProtocol + "localhost:9999/redhat/certified-operators:543219e933d930758fcf18e1c6e6deff3",
					Destination: consts.DockerProtocol + "localhost:5000/test/redhat/certified-operators:v4.10.0",
					Origin:      consts.DockerProtocol + "registry.redhat.io/redhat/certified-operators:v4.10",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "543219e933d930758fcf18e1c6e6deff3",
				},
			},
		},
		{
			caseName: "OperatorImageCollector - Mirror to Mirror: catalog image - targetCatalog should pass",
			relatedImages: map[string][]v2alpha1.RelatedImage{
				"redhat-operator-index.123458f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea": {
					{
						Image:         "registry.redhat.io/redhat/certified-operators:v4.14",
						Type:          v2alpha1.TypeOperatorCatalog,
						TargetCatalog: "12345/certified-operators-pinned",
						RebuiltTag:    "123459e933d930758fcf18e1c6e6deff3",
					},
				},
			},
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "registry.redhat.io/redhat/certified-operators:v4.14",
					Destination: consts.DockerProtocol + "localhost:9999/12345/certified-operators-pinned:v4.14",
					Origin:      consts.DockerProtocol + "registry.redhat.io/redhat/certified-operators:v4.14",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "123459e933d930758fcf18e1c6e6deff3",
				},
				{
					Source:      consts.DockerProtocol + "localhost:9999/12345/certified-operators-pinned:123459e933d930758fcf18e1c6e6deff3",
					Destination: consts.DockerProtocol + "localhost:5000/test/12345/certified-operators-pinned:v4.14",
					Origin:      consts.DockerProtocol + "registry.redhat.io/redhat/certified-operators:v4.14",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "123459e933d930758fcf18e1c6e6deff3",
				},
			},
		},
		{
			caseName: "OperatorImageCollector - Mirror to Mirror: catalog image - targetTag & targetCatalog should pass",
			relatedImages: map[string][]v2alpha1.RelatedImage{
				"certified-operators.f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea": {
					{
						Image:         "registry.redhat.io/redhat/certified-operators:v4.17",
						Type:          v2alpha1.TypeOperatorCatalog,
						TargetTag:     "v4.17.0-20241114",
						TargetCatalog: "redhat/certified-operators-pinned",
						RebuiltTag:    "dbf7a9e933d930758fcf18e1c6e6deff3",
					},
				},
			},
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "registry.redhat.io/redhat/certified-operators:v4.17",
					Destination: consts.DockerProtocol + "localhost:9999/redhat/certified-operators-pinned:v4.17.0-20241114",
					Origin:      consts.DockerProtocol + "registry.redhat.io/redhat/certified-operators:v4.17",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "dbf7a9e933d930758fcf18e1c6e6deff3",
				},
				{
					Source:      "docker://localhost:9999/redhat/certified-operators-pinned:dbf7a9e933d930758fcf18e1c6e6deff3",
					Destination: "docker://localhost:5000/test/redhat/certified-operators-pinned:v4.17.0-20241114",
					Origin:      "docker://registry.redhat.io/redhat/certified-operators:v4.17",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "dbf7a9e933d930758fcf18e1c6e6deff3",
				},
			},
			expectedError: false,
		},
		{
			caseName: "OperatorImageCollector - Mirror to Mirror: oci catalog image - targetTag & targetCatalog should pass",
			relatedImages: map[string][]v2alpha1.RelatedImage{
				"catalog-on-disk2.f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea": {
					{
						Name:          "coffee-shop-index",
						Image:         consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk2"),
						Type:          v2alpha1.TypeOperatorCatalog,
						TargetTag:     "v1.0",
						TargetCatalog: "coffee-shop-index",
						RebuiltTag:    "af7a9e933d930758fcf18e1c6e6deff3",
					},
				},
			},
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk2"),
					Destination: "docker://localhost:9999/coffee-shop-index:v1.0",
					Origin:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk2"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "af7a9e933d930758fcf18e1c6e6deff3",
				},
				{
					Source:      "docker://localhost:9999/coffee-shop-index:af7a9e933d930758fcf18e1c6e6deff3",
					Destination: "docker://localhost:5000/test/coffee-shop-index:v1.0",
					Origin:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk2"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "af7a9e933d930758fcf18e1c6e6deff3",
				},
			},
		},
		{
			caseName: "OperatorImageCollector - Mirror to Mirror: oci catalog image - targetCatalog should pass",
			relatedImages: map[string][]v2alpha1.RelatedImage{
				"catalog-on-disk3.f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea": {
					{
						Name:          "tea-shop-index",
						Image:         consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk3"),
						Type:          v2alpha1.TypeOperatorCatalog,
						TargetCatalog: "tea-shop-index",
						RebuiltTag:    "bf7a9e933d930758fcf18e1c6e6deff3",
					},
				},
			},
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk3"),
					Destination: "docker://localhost:9999/tea-shop-index:latest",
					Origin:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk3"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "bf7a9e933d930758fcf18e1c6e6deff3",
				},
				{
					Source:      "docker://localhost:9999/tea-shop-index:bf7a9e933d930758fcf18e1c6e6deff3",
					Destination: "docker://localhost:5000/test/tea-shop-index:latest",
					Origin:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk3"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "bf7a9e933d930758fcf18e1c6e6deff3",
				},
			},
		},
		{
			caseName: "OperatorImageCollector - Mirror to Mirror: oci catalog image - targetTag should pass",
			relatedImages: map[string][]v2alpha1.RelatedImage{
				"catalog-on-disk1.f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea": {
					{
						Name:       "catalog-on-disk1",
						Image:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk1"),
						Type:       v2alpha1.TypeOperatorCatalog,
						TargetTag:  "v1.1",
						RebuiltTag: "cf7a9e933d930758fcf18e1c6e6deff3",
					},
				},
			},
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk1"),
					Destination: "docker://localhost:9999/catalog-on-disk1:v1.1",
					Origin:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk1"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "cf7a9e933d930758fcf18e1c6e6deff3",
				},
				{
					Source:      "docker://localhost:9999/catalog-on-disk1:cf7a9e933d930758fcf18e1c6e6deff3",
					Destination: "docker://localhost:5000/test/catalog-on-disk1:v1.1",
					Origin:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk1"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "cf7a9e933d930758fcf18e1c6e6deff3",
				},
			},
		},
		{
			caseName: "OperatorImageCollector - Mirror to Mirror: oci catalog image nominal should pass",
			relatedImages: map[string][]v2alpha1.RelatedImage{
				"catalog-on-disk1.0987660452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea": {
					{
						Name:       "catalog-on-disk4",
						Image:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk4"),
						Type:       v2alpha1.TypeOperatorCatalog,
						RebuiltTag: "09876e933d930758fcf18e1c6e6deff3",
					},
				},
			},
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk4"),
					Destination: "docker://localhost:9999/catalog-on-disk4:latest",
					Origin:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk4"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "09876e933d930758fcf18e1c6e6deff3",
				},
				{
					Source:      "docker://localhost:9999/catalog-on-disk4:09876e933d930758fcf18e1c6e6deff3",
					Destination: "docker://localhost:5000/test/catalog-on-disk4:latest",
					Origin:      consts.OciProtocol + filepath.Join(consts.TestFolder, "catalog-on-disk4"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "09876e933d930758fcf18e1c6e6deff3",
				},
			},
		},
		{
			caseName: "OperatorImageCollector - Mirror to Mirror: Full=true catalog image nominal should pass",
			relatedImages: map[string][]v2alpha1.RelatedImage{
				"redhat-operator-index.f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea": {
					{
						Image: "registry.redhat.io/redhat/redhat-operator-index:v4.18",
						Type:  v2alpha1.TypeOperatorCatalog,
						// no rebuildTag: simulating full = true
					},
				},
			},
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.18",
					Destination: "docker://localhost:9999/redhat/redhat-operator-index:v4.18",
					Origin:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.18",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "docker://localhost:9999/redhat/redhat-operator-index:v4.18",
					Destination: "docker://localhost:5000/test/redhat/redhat-operator-index:v4.18",
					Origin:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.18",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			ex := setupFilterCollector_MirrorToDisk(tempDir, log, &MockManifest{})
			ex.Opts.Mode = mirror.MirrorToMirror
			ex.Opts.Destination = "docker://localhost:5000/test"
			res, err := ex.dispatchImagesForM2M(testCase.relatedImages)
			if testCase.expectedError && err == nil {
				t.Fatalf("should fail")
			}
			if !testCase.expectedError && err != nil {
				t.Fatal("should not fail")
			}
			assert.ElementsMatch(t, testCase.expectedResult, res)
		})
	}
}

// TestPrepareCopyBatchTagDigestCollision reproduces OCPBUGS-105878 at the operator
// collector level: two related images share the same repository and tag but pin
// different digests (the "repo:tag@sha256:..." form). Before the fix both collapsed to
// a single tag-only cache destination, so the archive dropped one digest's blobs.
// After the fix, mirrorToDisk keys the cache by digest (distinct destinations, no
// collision) and diskToMirror reads each back by its own digest key while pushing both
// to the shared human tag at the destination.
func TestPrepareCopyBatchTagDigestCollision(t *testing.T) {
	log := clog.New("trace")
	tempDir := t.TempDir()

	const (
		digestA = "1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58"
		digestB = "1b8392488dabcf78c82c72866d34edf6ef3d5bfb6ec00c81a377486a90d3d9ad"
	)
	imgA := "quay.io/oc-mirror/oc-mirror-dev:shared-operand@sha256:" + digestA
	imgB := "quay.io/oc-mirror/oc-mirror-dev:shared-operand@sha256:" + digestB

	relatedImages := map[string][]v2alpha1.RelatedImage{
		"collisiontest": {
			{Name: "operandA", Image: imgA, Type: v2alpha1.TypeOperatorRelatedImage},
			{Name: "operandB", Image: imgB, Type: v2alpha1.TypeOperatorRelatedImage},
		},
	}

	t.Run("MirrorToDisk keys the cache by digest (no collision)", func(t *testing.T) {
		ex := setupFilterCollector_MirrorToDisk(tempDir, log, &MockManifest{})
		res, err := ex.prepareM2DCopyBatch(relatedImages)
		assert.NoError(t, err)

		expected := []v2alpha1.CopyImageSchema{
			{
				Source:      consts.DockerProtocol + "quay.io/oc-mirror/oc-mirror-dev@sha256:" + digestA,
				Destination: consts.DockerProtocol + "localhost:9999/oc-mirror/oc-mirror-dev:sha256-" + digestA,
				Origin:      consts.DockerProtocol + imgA,
				Type:        v2alpha1.TypeOperatorRelatedImage,
			},
			{
				Source:      consts.DockerProtocol + "quay.io/oc-mirror/oc-mirror-dev@sha256:" + digestB,
				Destination: consts.DockerProtocol + "localhost:9999/oc-mirror/oc-mirror-dev:sha256-" + digestB,
				Origin:      consts.DockerProtocol + imgB,
				Type:        v2alpha1.TypeOperatorRelatedImage,
			},
		}
		assert.ElementsMatch(t, expected, res)

		// Core property: the two cache destinations must differ, otherwise one digest's
		// blobs would be lost during archiving.
		assert.Len(t, res, 2)
		assert.NotEqual(t, res[0].Destination, res[1].Destination, "cache destinations must not collide")
	})

	t.Run("DiskToMirror reads each digest from the cache, pushes both to the human tag", func(t *testing.T) {
		ex := setupFilterCollector_DiskToMirror(tempDir, log)
		res, err := ex.prepareD2MCopyBatch(relatedImages)
		assert.NoError(t, err)

		expected := []v2alpha1.CopyImageSchema{
			{
				Source:      consts.DockerProtocol + "localhost:9999/oc-mirror/oc-mirror-dev:sha256-" + digestA,
				Destination: consts.DockerProtocol + "localhost:5000/test/oc-mirror/oc-mirror-dev:shared-operand",
				Origin:      consts.DockerProtocol + imgA,
				Type:        v2alpha1.TypeOperatorRelatedImage,
			},
			{
				Source:      consts.DockerProtocol + "localhost:9999/oc-mirror/oc-mirror-dev:sha256-" + digestB,
				Destination: consts.DockerProtocol + "localhost:5000/test/oc-mirror/oc-mirror-dev:shared-operand",
				Origin:      consts.DockerProtocol + imgB,
				Type:        v2alpha1.TypeOperatorRelatedImage,
			},
		}
		assert.ElementsMatch(t, expected, res)

		// Core property: distinct cache sources (both readable by digest) that resolve to
		// the same human tag at the destination.
		assert.Len(t, res, 2)
		assert.NotEqual(t, res[0].Source, res[1].Source, "cache sources must be distinct per digest")
		assert.Equal(t, res[0].Destination, res[1].Destination, "both digests push to the shared human tag")
	})
}

// TestPrepareCopyBatchSameDigestDifferentTags covers the inverse of OCPBUGS-105878: two
// related images share the same digest but carry different tags. The digest-keyed cache
// is content-addressed, so both share a single cache entry (stored/archived once), while
// diskToMirror recreates BOTH human tags at the destination pointing to that one digest.
// This guards against a regression where digest-keying the cache could drop a tag.
func TestPrepareCopyBatchSameDigestDifferentTags(t *testing.T) {
	log := clog.New("trace")
	tempDir := t.TempDir()

	const digest = "1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58"
	imgA := "quay.io/oc-mirror/oc-mirror-dev:tag-one@sha256:" + digest
	imgB := "quay.io/oc-mirror/oc-mirror-dev:tag-two@sha256:" + digest

	relatedImages := map[string][]v2alpha1.RelatedImage{
		"sharedigest": {
			{Name: "tagOne", Image: imgA, Type: v2alpha1.TypeOperatorRelatedImage},
			{Name: "tagTwo", Image: imgB, Type: v2alpha1.TypeOperatorRelatedImage},
		},
	}

	t.Run("MirrorToDisk shares a single digest-keyed cache entry", func(t *testing.T) {
		ex := setupFilterCollector_MirrorToDisk(tempDir, log, &MockManifest{})
		res, err := ex.prepareM2DCopyBatch(relatedImages)
		assert.NoError(t, err)

		expected := []v2alpha1.CopyImageSchema{
			{
				Source:      consts.DockerProtocol + "quay.io/oc-mirror/oc-mirror-dev@sha256:" + digest,
				Destination: consts.DockerProtocol + "localhost:9999/oc-mirror/oc-mirror-dev:sha256-" + digest,
				Origin:      consts.DockerProtocol + imgA,
				Type:        v2alpha1.TypeOperatorRelatedImage,
			},
			{
				Source:      consts.DockerProtocol + "quay.io/oc-mirror/oc-mirror-dev@sha256:" + digest,
				Destination: consts.DockerProtocol + "localhost:9999/oc-mirror/oc-mirror-dev:sha256-" + digest,
				Origin:      consts.DockerProtocol + imgB,
				Type:        v2alpha1.TypeOperatorRelatedImage,
			},
		}
		assert.ElementsMatch(t, expected, res)

		// Core property: both images resolve to the SAME digest-keyed cache entry.
		assert.Len(t, res, 2)
		assert.Equal(t, res[0].Destination, res[1].Destination, "same digest must share one cache entry")
	})

	t.Run("DiskToMirror recreates both tags from the shared cache entry", func(t *testing.T) {
		ex := setupFilterCollector_DiskToMirror(tempDir, log)
		res, err := ex.prepareD2MCopyBatch(relatedImages)
		assert.NoError(t, err)

		expected := []v2alpha1.CopyImageSchema{
			{
				Source:      consts.DockerProtocol + "localhost:9999/oc-mirror/oc-mirror-dev:sha256-" + digest,
				Destination: consts.DockerProtocol + "localhost:5000/test/oc-mirror/oc-mirror-dev:tag-one",
				Origin:      consts.DockerProtocol + imgA,
				Type:        v2alpha1.TypeOperatorRelatedImage,
			},
			{
				Source:      consts.DockerProtocol + "localhost:9999/oc-mirror/oc-mirror-dev:sha256-" + digest,
				Destination: consts.DockerProtocol + "localhost:5000/test/oc-mirror/oc-mirror-dev:tag-two",
				Origin:      consts.DockerProtocol + imgB,
				Type:        v2alpha1.TypeOperatorRelatedImage,
			},
		}
		assert.ElementsMatch(t, expected, res)

		// Core property: one shared cache source recreates BOTH distinct human tags.
		assert.Len(t, res, 2)
		assert.Equal(t, res[0].Source, res[1].Source, "both tags are read from the one shared cache entry")
		assert.NotEqual(t, res[0].Destination, res[1].Destination, "both distinct tags must be recreated")
	})
}

func TestOperatorCollector(t *testing.T) {
	log := clog.New("trace")

	t.Run("OperatorCollector - cachedCatalog", func(t *testing.T) {
		op := OperatorCollector{
			Log:              log,
			LocalStorageFQDN: "localhost:5000",
		}

		t.Run("should succeed", func(t *testing.T) {
			t.Run("with path component", func(t *testing.T) {
				catalog := v2alpha1.Operator{
					Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.17",
				}
				ref, err := op.cachedCatalog(catalog, "filteredTag")
				assert.NoError(t, err)
				assert.Equal(t, "docker://localhost:5000/redhat/redhat-operator-index:filteredTag", ref)
			})
			t.Run("with target catalog", func(t *testing.T) {
				catalog := v2alpha1.Operator{
					Catalog:       "registry.redhat.io/redhat/redhat-operator-index:v4.17",
					TargetCatalog: "targetTag",
				}
				ref, err := op.cachedCatalog(catalog, "filteredTag")
				assert.NoError(t, err)
				assert.Equal(t, "docker://localhost:5000/targetTag:filteredTag", ref)
			})
			t.Run("with oci protocol", func(t *testing.T) {
				catalog := v2alpha1.Operator{
					Catalog: consts.OciProtocol + "registry.redhat.io/redhat/redhat-operator-index",
				}
				ref, err := op.cachedCatalog(catalog, "filteredTag")
				assert.NoError(t, err)
				assert.Equal(t, "docker://localhost:5000/redhat-operator-index:filteredTag", ref)
			})
			t.Run("with empty filteredTag", func(t *testing.T) {
				catalog := v2alpha1.Operator{
					Catalog:       "registry.redhat.io/redhat/redhat-operator-index:v4.17",
					TargetCatalog: "targetTag",
				}
				ref, err := op.cachedCatalog(catalog, "")
				assert.NoError(t, err)
				assert.Equal(t, "docker://localhost:5000/targetTag", ref)
			})
		})
		t.Run("should fail when parsing reference fails", func(t *testing.T) {
			ref, err := op.cachedCatalog(v2alpha1.Operator{}, "filteredTag")
			assert.ErrorContains(t, err, "unable to determine cached reference for catalog")
			assert.Empty(t, ref)
		})
	})
}
