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

var (
	nominalConfigM2M = v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: "registry.redhat.io/redhat/community-operator-index:v4.18",
						Full:    true,
					},
					{
						Catalog:       "registry.redhat.io/redhat/redhat-operator-index:v4.17",
						TargetCatalog: "redhat/redhat-filtered-index",
						IncludeConfig: v2alpha1.IncludeConfig{
							Packages: []v2alpha1.IncludePackage{
								{Name: "op1"},
							},
						},
					},
					{
						Catalog:       "registry.redhat.io/redhat/certified-operators:v4.17",
						Full:          true,
						TargetCatalog: "redhat/certified-operators-pinned",
						TargetTag:     "v4.17.0-20241114",
						IncludeConfig: v2alpha1.IncludeConfig{
							Packages: []v2alpha1.IncludePackage{
								{Name: "op1"},
							},
						},
					},
					{
						Catalog: "oci://" + common.TestFolder + "catalog-on-disk1",
						IncludeConfig: v2alpha1.IncludeConfig{
							Packages: []v2alpha1.IncludePackage{
								{Name: "op1"},
							},
						},
					},
					{
						Catalog:       "oci://" + common.TestFolder + "catalog-on-disk2",
						Full:          true,
						TargetCatalog: "coffee-shop-index",
						IncludeConfig: v2alpha1.IncludeConfig{
							Packages: []v2alpha1.IncludePackage{
								{Name: "op1"},
							},
						},
					},
					{
						Catalog:       "oci://" + common.TestFolder + "catalog-on-disk3",
						TargetCatalog: "tea-shop-index",
						TargetTag:     "v3.14",
						IncludeConfig: v2alpha1.IncludeConfig{
							Packages: []v2alpha1.IncludePackage{
								{Name: "op1"},
							},
						},
					},
				},
			},
		},
	}
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
					Destination: "docker://localhost:9999/sometestimage-a:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:9999/sometestimage-b:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
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
					RebuiltTag:  "442c7ba64d56a85eea155325aa0c6537",
				},
				{
					Source:      "docker://community-operators:v4.7",
					Destination: "docker://localhost:9999/community-operators:v4.7",
					Origin:      "docker://community-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "4dab2467f35b4d9c9ba7c2a7823de8bd",
				},
				{
					Source:      "oci://" + common.TestFolder + "simple-test-bundle",
					Destination: "docker://localhost:9999/simple-test-bundle:latest",
					Origin:      "oci://" + common.TestFolder + "simple-test-bundle",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "9fadc6c70adb4b2571f66f674a876279",
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
					Destination: "docker://localhost:9999/sometestimage-a:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:9999/sometestimage-b:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
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
					RebuiltTag:  "9fadc6c70adb4b2571f66f674a876279",
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
					Destination: "docker://localhost:9999/sometestimage-a:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:9999/sometestimage-b:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
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
					RebuiltTag:  "9fadc6c70adb4b2571f66f674a876279",
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
					Destination: "docker://localhost:9999/sometestimage-a:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:9999/sometestimage-b:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
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
					RebuiltTag:  "442c7ba64d56a85eea155325aa0c6537",
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
					Source:      "docker://localhost:9999/sometestimage-a:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:5000/test/sometestimage-a:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://localhost:9999/sometestimage-b:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:5000/test/sometestimage-b:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
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
					Source:      "docker://localhost:9999/simple-test-bundle:9fadc6c70adb4b2571f66f674a876279",
					Destination: "docker://localhost:5000/test/simple-test-bundle:latest",
					Origin:      "oci://" + common.TestFolder + "simple-test-bundle",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "9fadc6c70adb4b2571f66f674a876279",
				},
				{
					Source:      "docker://localhost:9999/redhat/redhat-operator-index:6566d78129230a2e107cb5aafcb7787b",
					Destination: "docker://localhost:5000/test/redhat/redhat-operator-index:v4.14",
					Origin:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.14",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "6566d78129230a2e107cb5aafcb7787b",
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
					Source:      "docker://localhost:9999/sometestimage-a:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:5000/test/sometestimage-a:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://localhost:9999/sometestimage-b:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:5000/test/sometestimage-b:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
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
					Source:      "docker://localhost:9999/test-catalog:9fadc6c70adb4b2571f66f674a876279",
					Destination: "docker://localhost:5000/test/test-catalog:v4.14",
					Origin:      "oci://" + common.TestFolder + "simple-test-bundle",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "9fadc6c70adb4b2571f66f674a876279",
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
	err := copy.Copy(common.TestFolder+"working-dir-fake/hold-operator/redhat-operator-index/v4.14", filepath.Join(tempDir, "working-dir", operatorImageExtractDir, "redhat/redhat-operator-index/f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"))
	if err != nil {
		t.Fatalf("should not fail")
	}

	os.MkdirAll(common.TestFolder+"/catalog-on-disk1", 0755)
	os.MkdirAll(common.TestFolder+"/catalog-on-disk2", 0755)
	os.MkdirAll(common.TestFolder+"/catalog-on-disk3", 0755)
	//copy tests/hold-test-fake to working-dir
	err = copy.Copy(common.TestFolder+"/oci-image", common.TestFolder+"/catalog-on-disk1")
	if err != nil {
		t.Fatalf("should not fail")
	}
	err = copy.Copy(common.TestFolder+"/oci-image", common.TestFolder+"/catalog-on-disk2")
	if err != nil {
		t.Fatalf("should not fail")
	}
	err = copy.Copy(common.TestFolder+"/oci-image", common.TestFolder+"/catalog-on-disk3")
	if err != nil {
		t.Fatalf("should not fail")
	}
	defer os.RemoveAll(common.TestFolder + "/catalog-on-disk1")
	defer os.RemoveAll(common.TestFolder + "/catalog-on-disk2")
	defer os.RemoveAll(common.TestFolder + "/catalog-on-disk3")

	testCases := []testCase{
		{
			caseName:      "OperatorImageCollector - Mirror to Mirror: should pass",
			config:        nominalConfigM2M,
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:5000/test/sometestimage-a:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      "docker://sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      "docker://sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:5000/test/sometestimage-b:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
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
					Source:      "docker://registry.redhat.io/redhat/community-operator-index:v4.18",
					Destination: "docker://localhost:9999/redhat/community-operator-index:v4.18",
					Origin:      "docker://registry.redhat.io/redhat/community-operator-index:v4.18",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "docker://localhost:9999/redhat/community-operator-index:v4.18",
					Destination: "docker://localhost:5000/test/redhat/community-operator-index:v4.18",
					Origin:      "docker://registry.redhat.io/redhat/community-operator-index:v4.18",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.17",
					Destination: "docker://localhost:9999/redhat/redhat-filtered-index:v4.17",
					Origin:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.17",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "08a5610c0e6f72fd34b1c76d30788c66",
				},
				{
					Source:      "docker://registry.redhat.io/redhat/certified-operators:v4.17",
					Destination: "docker://localhost:9999/redhat/certified-operators-pinned:v4.17.0-20241114",
					Origin:      "docker://registry.redhat.io/redhat/certified-operators:v4.17",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "65af60f894902a1758a30ae262c0e39e",
				},
				{
					Source:      "oci://" + common.TestFolder + "catalog-on-disk1",
					Destination: "docker://localhost:9999/catalog-on-disk1:latest",
					Origin:      "oci://" + common.TestFolder + "catalog-on-disk1",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "fc2e113a1d6f0dbe89bd2bc5c83886e3",
				},
				{
					Source:      "oci://" + common.TestFolder + "catalog-on-disk2",
					Destination: "docker://localhost:9999/coffee-shop-index:latest",
					Origin:      "oci://" + common.TestFolder + "catalog-on-disk2",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "421035ded2cb0e83f50ee6445b1466a5",
				},
				{
					Source:      "oci://" + common.TestFolder + "catalog-on-disk3",
					Destination: "docker://localhost:9999/tea-shop-index:v3.14",
					Origin:      "oci://" + common.TestFolder + "catalog-on-disk3",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "d81a7ad49cabfc8aa050edaf56f25a3f",
				},

				{
					Source:      "docker://localhost:9999/redhat/redhat-filtered-index:08a5610c0e6f72fd34b1c76d30788c66",
					Destination: "docker://localhost:5000/test/redhat/redhat-filtered-index:v4.17",
					Origin:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.17",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "08a5610c0e6f72fd34b1c76d30788c66",
				},
				{
					Source:      "docker://localhost:9999/redhat/certified-operators-pinned:65af60f894902a1758a30ae262c0e39e",
					Destination: "docker://localhost:5000/test/redhat/certified-operators-pinned:v4.17.0-20241114",
					Origin:      "docker://registry.redhat.io/redhat/certified-operators:v4.17",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "65af60f894902a1758a30ae262c0e39e",
				},
				{
					Source:      "docker://localhost:9999/catalog-on-disk1:fc2e113a1d6f0dbe89bd2bc5c83886e3",
					Destination: "docker://localhost:5000/test/catalog-on-disk1:latest",
					Origin:      "oci://" + common.TestFolder + "catalog-on-disk1",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "fc2e113a1d6f0dbe89bd2bc5c83886e3",
				},
				{
					Source:      "docker://localhost:9999/coffee-shop-index:421035ded2cb0e83f50ee6445b1466a5",
					Destination: "docker://localhost:5000/test/coffee-shop-index:latest",
					Origin:      "oci://" + common.TestFolder + "catalog-on-disk2",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "421035ded2cb0e83f50ee6445b1466a5",
				},
				{
					Source:      "docker://localhost:9999/tea-shop-index:d81a7ad49cabfc8aa050edaf56f25a3f",
					Destination: "docker://localhost:5000/test/tea-shop-index:v3.14",
					Origin:      "oci://" + common.TestFolder + "catalog-on-disk3",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "d81a7ad49cabfc8aa050edaf56f25a3f",
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
