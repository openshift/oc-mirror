package operator

import (
	"context"
	"os"

	"path/filepath"
	"testing"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"

	//"github.com/stretchr/testify/assert"
	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
)

func TestFilterCollectorM2D(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()
	defer os.RemoveAll(tempDir)
	type testCase struct {
		caseName       string
		config         v2alpha1.ImageSetConfiguration
		expectedResult []v2alpha1.CopyImageSchema
		expectedError  bool
	}

	ctx := context.Background()
	manifest := &MockManifest{Log: log}

	testCases := []testCase{
		{
			caseName:      "OperatorImageCollector - Mirror to disk: should pass",
			config:        nominalConfigM2D,
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:9999/sometestimage-a:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:9999/sometestimage-b:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://gcr.io/kubebuilder/kube-rbac-proxy@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Destination: "docker://localhost:9999/kubebuilder/kube-rbac-proxy:v0.13.1",
					Origin:      "docker://gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://redhat-operators:v4.7",
					Destination: "docker://localhost:9999/redhat-operators:v4.7",
					Origin:      "docker://redhat-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "docker://certified-operators:v4.7",
					Destination: "docker://localhost:9999/certified-operators:v4.7",
					Origin:      "docker://certified-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "docker://community-operators:v4.7",
					Destination: "docker://localhost:9999/community-operators:v4.7",
					Origin:      "docker://community-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "oci://" + common.TestFolder + "simple-test-bundle",
					Destination: "docker://localhost:9999/simple-test-bundle:latest",
					Origin:      "oci://" + common.TestFolder + "simple-test-bundle",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
			},
		},
		{
			caseName:      "OperatorImageCollector - Mirror to disk - OCI Catalog with TargetTag: should pass",
			config:        nominalConfigM2DWithTargetTag4Oci,
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:9999/sometestimage-a:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:9999/sometestimage-b:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://gcr.io/kubebuilder/kube-rbac-proxy@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Destination: "docker://localhost:9999/kubebuilder/kube-rbac-proxy:v0.13.1",
					Origin:      "docker://gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "oci://" + common.TestFolder + "simple-test-bundle",
					Destination: "docker://localhost:9999/simple-test-bundle:v4.14",
					Origin:      "oci://" + common.TestFolder + "simple-test-bundle",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
			},
		},
		{
			caseName:       "OperatorImageCollector - Mirror to disk - OCI Catalog with invalid TargetCatalog: should fail",
			config:         failingConfigM2DWithTargetCatalog4Oci,
			expectedError:  true,
			expectedResult: []v2alpha1.CopyImageSchema{},
		},
		{
			caseName:      "OperatorImageCollector - Mirror to disk - OCI Catalog with TargetTag + TargetCatalog: should pass",
			config:        nominalConfigM2DWithTargetCatalogTag4Oci,
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:9999/sometestimage-a:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:9999/sometestimage-b:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://gcr.io/kubebuilder/kube-rbac-proxy@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Destination: "docker://localhost:9999/kubebuilder/kube-rbac-proxy:v0.13.1",
					Origin:      "docker://gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "oci://" + common.TestFolder + "simple-test-bundle",
					Destination: "docker://localhost:9999/test-catalog:v4.14",
					Origin:      "oci://" + common.TestFolder + "simple-test-bundle",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
			},
		},
		{
			caseName:      "OperatorImageCollector - Mirror to disk - Catalog with TargetTag + TargetCatalog: should pass",
			config:        nominalConfigM2DWithTargetCatalogTag,
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:9999/sometestimage-a:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:9999/sometestimage-b:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://gcr.io/kubebuilder/kube-rbac-proxy@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Destination: "docker://localhost:9999/kubebuilder/kube-rbac-proxy:v0.13.1",
					Origin:      "docker://gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://certified-operators:v4.7",
					Destination: "docker://localhost:9999/test-namespace/test-catalog:v2.0",
					Origin:      "docker://certified-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			ex := setupFilterCollector_MirrorToDisk(tempDir, log, manifest)
			ex = ex.withConfig(testCase.config)
			res, err := ex.OperatorImageCollector(ctx)
			if testCase.expectedError && err == nil {
				t.Fatalf("should fail")
			}
			if !testCase.expectedError && err != nil {
				t.Fatal("should not fail")
			}
			assert.ElementsMatch(t, testCase.expectedResult, res.AllImages)
		})
	}

	// this test should cover over 80% M2D
	t.Run("Testing OperatorImageCollector - Mirror to disk: should pass", func(t *testing.T) {
		ex := setupFilterCollector_MirrorToDisk(tempDir, log, manifest)
		// ensure coverage in new.go
		_ = New(log, "working-dir", ex.Config, ex.Opts, ex.Mirror, manifest)
	})
}
func TestFilterCollectorD2M(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()
	defer os.RemoveAll(tempDir)
	type testCase struct {
		caseName       string
		config         v2alpha1.ImageSetConfiguration
		expectedResult []v2alpha1.CopyImageSchema
		expectedError  bool
	}

	ctx := context.Background()
	os.RemoveAll(common.TestFolder + "hold-operator/")
	os.RemoveAll(common.TestFolder + "operator-images")
	os.RemoveAll(common.TestFolder + "tmp/")

	//copy tests/hold-test-fake to working-dir
	err := copy.Copy(common.TestFolder+"working-dir-fake/hold-operator/redhat-operator-index/v4.14", filepath.Join(tempDir, "working-dir", operatorImageExtractDir, "redhat-operator-index/f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"))
	if err != nil {
		t.Fatalf("should not fail")
	}

	testCases := []testCase{
		{
			caseName:      "OperatorImageCollector - Disk to mirror : should pass",
			config:        nominalConfigD2M,
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://localhost:9999/sometestimage-a:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:5000/test/sometestimage-a:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://localhost:9999/sometestimage-b:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:5000/test/sometestimage-b:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://localhost:9999/kubebuilder/kube-rbac-proxy:v0.13.1",
					Destination: "docker://localhost:5000/test/kubebuilder/kube-rbac-proxy:v0.13.1",
					Origin:      "docker://gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://localhost:9999/simple-test-bundle:latest",
					Destination: "docker://localhost:5000/test/simple-test-bundle:latest",
					Origin:      "oci://" + common.TestFolder + "simple-test-bundle",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "docker://localhost:9999/redhat/redhat-operator-index:v4.14",
					Destination: "docker://localhost:5000/test/redhat/redhat-operator-index:v4.14",
					Origin:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.14",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
			},
		},
		{
			caseName:      "OperatorImageCollector - Disk to mirror - OCI Catalog with invalid TargetCatalog: should fail",
			config:        failingConfigD2MWithTargetCatalog4OCI,
			expectedError: true,
		},
		{
			caseName:      "OperatorImageCollector - Disk to mirror - OCI Catalog with TargetTag + TargetCatalog: should pass",
			config:        nominalConfigD2MWithTargetCatalogTag4OCI,
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://localhost:9999/sometestimage-a:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:5000/test/sometestimage-a:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://localhost:9999/sometestimage-b:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:5000/test/sometestimage-b:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://localhost:9999/kubebuilder/kube-rbac-proxy:v0.13.1",
					Destination: "docker://localhost:5000/test/kubebuilder/kube-rbac-proxy:v0.13.1",
					Origin:      "docker://gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://localhost:9999/test-catalog:v4.14",
					Destination: "docker://localhost:5000/test/test-catalog:v4.14",
					Origin:      "oci://" + common.TestFolder + "simple-test-bundle",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			ex := setupFilterCollector_DiskToMirror(tempDir, log)
			ex = ex.withConfig(testCase.config)
			res, err := ex.OperatorImageCollector(ctx)
			if testCase.expectedError && err == nil {
				t.Fatalf("should fail")
			}
			if !testCase.expectedError && err != nil {
				t.Fatal("should not fail")
			}
			assert.ElementsMatch(t, testCase.expectedResult, res.AllImages)
		})
	}

}

func TestFilterCollectorM2M(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()
	defer os.RemoveAll(tempDir)
	type testCase struct {
		caseName       string
		config         v2alpha1.ImageSetConfiguration
		expectedResult []v2alpha1.CopyImageSchema
		expectedError  bool
	}

	ctx := context.Background()
	os.RemoveAll(common.TestFolder + "hold-operator/")
	os.RemoveAll(common.TestFolder + "operator-images")
	os.RemoveAll(common.TestFolder + "tmp/")

	//copy tests/hold-test-fake to working-dir
	err := copy.Copy(common.TestFolder+"working-dir-fake/hold-operator/redhat-operator-index/v4.14", filepath.Join(tempDir, "working-dir", operatorImageExtractDir, "redhat-operator-index/f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"))
	if err != nil {
		t.Fatalf("should not fail")
	}

	testCases := []testCase{
		{
			caseName:      "OperatorImageCollector - Mirror to disk: should pass",
			config:        nominalConfigM2D,
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:5000/test/sometestimage-a:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:5000/test/sometestimage-b:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://gcr.io/kubebuilder/kube-rbac-proxy@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Destination: "docker://localhost:5000/test/kubebuilder/kube-rbac-proxy:v0.13.1",
					Origin:      "docker://gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://redhat-operators:v4.7",
					Destination: "docker://localhost:5000/test/redhat-operators:v4.7",
					Origin:      "docker://redhat-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "docker://certified-operators:v4.7",
					Destination: "docker://localhost:5000/test/certified-operators:v4.7",
					Origin:      "docker://certified-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "docker://community-operators:v4.7",
					Destination: "docker://localhost:5000/test/community-operators:v4.7",
					Origin:      "docker://community-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "oci://" + common.TestFolder + "simple-test-bundle",
					Destination: "docker://localhost:5000/test/simple-test-bundle:latest",
					Origin:      "oci://" + common.TestFolder + "simple-test-bundle",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "docker://redhat-operators:v4.7",
					Destination: "docker://localhost:9999/redhat-operators:v4.7",
					Origin:      "docker://redhat-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "docker://certified-operators:v4.7",
					Destination: "docker://localhost:9999/certified-operators:v4.7",
					Origin:      "docker://certified-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "docker://community-operators:v4.7",
					Destination: "docker://localhost:9999/community-operators:v4.7",
					Origin:      "docker://community-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "oci://" + common.TestFolder + "simple-test-bundle",
					Destination: "docker://localhost:9999/simple-test-bundle:latest",
					Origin:      "oci://" + common.TestFolder + "simple-test-bundle",
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
			ex = ex.withConfig(testCase.config)
			res, err := ex.OperatorImageCollector(ctx)
			if testCase.expectedError && err == nil {
				t.Fatalf("should fail")
			}
			if !testCase.expectedError && err != nil {
				t.Fatal("should not fail")
			}
			assert.ElementsMatch(t, testCase.expectedResult, res.AllImages)
		})
	}

}

func setupFilterCollector_DiskToMirror(tempDir string, log clog.PluggableLoggerInterface) *FilterCollector {
	manifest := &MockManifest{Log: log}
	handler := &MockHandler{Log: log}
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
		LocalStorageFQDN:    "localhost:9999",
	}

	ex := &FilterCollector{
		OperatorCollector{Log: log,
			Mirror:           &MockMirror{Fail: false},
			Config:           nominalConfigD2M,
			Manifest:         manifest,
			Opts:             d2mOpts,
			LocalStorageFQDN: "localhost:9999",
			ctlgHandler:      handler},
	}

	return ex
}

func setupFilterCollector_MirrorToDisk(tempDir string, log clog.PluggableLoggerInterface, manifest *MockManifest) *FilterCollector {
	handler := &MockHandler{Log: log}

	globalM2D := &mirror.GlobalOptions{
		SecurePolicy: false,
		WorkingDir:   tempDir + "/working-dir",
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
		LocalStorageFQDN:    "localhost:9999",
	}

	ex := &FilterCollector{
		OperatorCollector{Log: log,
			Mirror:           &MockMirror{Fail: false},
			Config:           nominalConfigM2D,
			Manifest:         manifest,
			Opts:             m2dOpts,
			LocalStorageFQDN: "localhost:9999",
			ctlgHandler:      handler,
		},
	}
	return ex
}

func (ex *FilterCollector) withConfig(cfg v2alpha1.ImageSetConfiguration) *FilterCollector {
	ex.Config = cfg
	return ex
}
