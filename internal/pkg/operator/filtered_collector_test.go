package operator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/fake"
	"github.com/opencontainers/go-digest"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/parser"
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

type MockHandler struct {
	Log clog.PluggableLoggerInterface
}

var (
	nominalConfigD2M = v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.14",
					},
					{
						Catalog: consts.OciProtocol + consts.TestFolder + "simple-test-bundle",
					},
				},
			},
		},
	}
	failingConfigD2MWithTargetCatalog4OCI = v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog:       consts.OciProtocol + consts.TestFolder + "simple-test-bundle",
						TargetCatalog: "test-catalog:v4.14",
					},
				},
			},
		},
	}
	nominalConfigD2MWithTargetCatalogTag4OCI = v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog:       consts.OciProtocol + consts.TestFolder + "simple-test-bundle",
						TargetTag:     "v4.14",
						TargetCatalog: "test-catalog",
					},
				},
			},
		},
	}
	// nolint: unused
	nominalConfigD2MWithTargetCatalogTag = v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog:       "redhat-operator-index:v4.14",
						TargetCatalog: "test-namespace/test-catalog",
						TargetTag:     "v2.0",
					},
				},
			},
		},
	}
	nominalConfigM2D = v2alpha1.ImageSetConfiguration{
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
					Graph: true,
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
					{
						Catalog: consts.OciProtocol + consts.TestFolder + "simple-test-bundle",
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
	nominalConfigM2DWithTargetTag4Oci = v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog:   consts.OciProtocol + consts.TestFolder + "simple-test-bundle",
						TargetTag: "v4.14",
					},
				},
			},
		},
	}

	failingConfigM2DWithTargetCatalog4Oci = v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog:       consts.OciProtocol + consts.TestFolder + "simple-test-bundle",
						TargetCatalog: "test-catalog:v4.14",
					},
				},
			},
		},
	}
	nominalConfigM2DWithTargetCatalogTag4Oci = v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog:       consts.OciProtocol + consts.TestFolder + "simple-test-bundle",
						TargetCatalog: "test-catalog",
						TargetTag:     "v4.14",
					},
				},
			},
		},
	}
	nominalConfigM2DWithTargetCatalogTag = v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						TargetCatalog: "test-namespace/test-catalog",
						TargetTag:     "v2.0",
						Catalog:       "certified-operators:v4.7",
						Full:          true,
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
				},
			},
		},
	}
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
						Catalog: consts.OciProtocol + consts.TestFolder + "catalog-on-disk1",
						IncludeConfig: v2alpha1.IncludeConfig{
							Packages: []v2alpha1.IncludePackage{
								{Name: "op1"},
							},
						},
					},
					{
						Catalog:       consts.OciProtocol + consts.TestFolder + "catalog-on-disk2",
						Full:          true,
						TargetCatalog: "coffee-shop-index",
						IncludeConfig: v2alpha1.IncludeConfig{
							Packages: []v2alpha1.IncludePackage{
								{Name: "op1"},
							},
						},
					},
					{
						Catalog:       consts.OciProtocol + consts.TestFolder + "catalog-on-disk3",
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

	type testCase struct {
		caseName       string
		config         v2alpha1.ImageSetConfiguration
		expectedResult []v2alpha1.CopyImageSchema
		expectedError  bool
	}

	ctx := context.Background()
	manifest := &MockManifest{Log: log}

	testDir, err := filepath.Abs(consts.TestFolder)
	assert.NoError(t, err, "should get tests/ absolute path")

	testCases := []testCase{
		{
			caseName:      "OperatorImageCollector - Mirror to disk: should pass",
			config:        nominalConfigM2D,
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: consts.DockerProtocol + "localhost:9999/sometestimage-a:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      consts.DockerProtocol + "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      consts.DockerProtocol + "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: consts.DockerProtocol + "localhost:9999/sometestimage-b:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      consts.DockerProtocol + "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      consts.DockerProtocol + "gcr.io/kubebuilder/kube-rbac-proxy@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Destination: consts.DockerProtocol + "localhost:9999/kubebuilder/kube-rbac-proxy:v0.13.1",
					Origin:      consts.DockerProtocol + "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      consts.DockerProtocol + "redhat-operators:v4.7",
					Destination: consts.DockerProtocol + "localhost:9999/redhat-operators:v4.7",
					Origin:      consts.DockerProtocol + "redhat-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      consts.DockerProtocol + "certified-operators:v4.7",
					Destination: consts.DockerProtocol + "localhost:9999/certified-operators:v4.7",
					Origin:      consts.DockerProtocol + "certified-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "70eb0b2116707316c6130de415ceeb69",
				},
				{
					Source:      consts.DockerProtocol + "community-operators:v4.7",
					Destination: consts.DockerProtocol + "localhost:9999/community-operators:v4.7",
					Origin:      consts.DockerProtocol + "community-operators:v4.7",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "ac8e314872a499f2c6edb0616489c628",
				},
				{
					Source:      consts.OciProtocol + filepath.Join(testDir, "simple-test-bundle"),
					Destination: consts.DockerProtocol + "localhost:9999/simple-test-bundle:latest",
					Origin:      consts.OciProtocol + filepath.Join(testDir, "simple-test-bundle"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "eaf28fd0a9f205e44fb52a8b0bd8e678",
				},
			},
		},
		{
			caseName:      "OperatorImageCollector - Mirror to disk - OCI Catalog with TargetTag: should pass",
			config:        nominalConfigM2DWithTargetTag4Oci,
			expectedError: false,
			expectedResult: []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: consts.DockerProtocol + "localhost:9999/sometestimage-a:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      consts.DockerProtocol + "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      consts.DockerProtocol + "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: consts.DockerProtocol + "localhost:9999/sometestimage-b:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      consts.DockerProtocol + "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      consts.DockerProtocol + "gcr.io/kubebuilder/kube-rbac-proxy@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Destination: consts.DockerProtocol + "localhost:9999/kubebuilder/kube-rbac-proxy:v0.13.1",
					Origin:      consts.DockerProtocol + "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      consts.OciProtocol + filepath.Join(testDir, "simple-test-bundle"),
					Destination: consts.DockerProtocol + "localhost:9999/simple-test-bundle:v4.14",
					Origin:      consts.OciProtocol + filepath.Join(testDir, "simple-test-bundle"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "eaf28fd0a9f205e44fb52a8b0bd8e678",
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
					Source:      consts.DockerProtocol + "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: consts.DockerProtocol + "localhost:9999/sometestimage-a:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      consts.DockerProtocol + "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      consts.DockerProtocol + "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: consts.DockerProtocol + "localhost:9999/sometestimage-b:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Origin:      consts.DockerProtocol + "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      consts.DockerProtocol + "gcr.io/kubebuilder/kube-rbac-proxy@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Destination: consts.DockerProtocol + "localhost:9999/kubebuilder/kube-rbac-proxy:v0.13.1",
					Origin:      consts.DockerProtocol + "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522",
					Type:        v2alpha1.TypeInvalid,
				},
				{
					Source:      consts.OciProtocol + filepath.Join(testDir, "simple-test-bundle"),
					Destination: "docker://localhost:9999/test-catalog:v4.14",
					Origin:      consts.OciProtocol + filepath.Join(testDir, "simple-test-bundle"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "eaf28fd0a9f205e44fb52a8b0bd8e678",
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
					RebuiltTag:  "70eb0b2116707316c6130de415ceeb69",
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			ex := setupFilterCollector_MirrorToDisk(tempDir, log, manifest)
			ex = ex.withConfig(testCase.config)
			res, err := ex.OperatorImageCollector(ctx)
			if testCase.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.ElementsMatch(t, testCase.expectedResult, res.AllImages)
		})
	}

	// this test should cover over 80% M2D
	t.Run("Testing OperatorImageCollector - Mirror to disk: should pass", func(t *testing.T) {
		ex := setupFilterCollector_MirrorToDisk(tempDir, log, manifest)
		// ensure coverage in new.go
		_ = NewWithFilter(log, "working-dir", ex.Config, ex.Opts, ex.Mirror, manifest)
	})
}

func TestFilterCollectorD2M(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()

	testDir, err := filepath.Abs(consts.TestFolder)
	assert.NoError(t, err, "should get tests/ absolute path")

	type testCase struct {
		caseName       string
		config         v2alpha1.ImageSetConfiguration
		expectedResult []v2alpha1.CopyImageSchema
		expectedError  bool
	}

	ctx := context.Background()
	os.RemoveAll(consts.TestFolder + "hold-operator/")
	os.RemoveAll(consts.TestFolder + "operator-images")
	os.RemoveAll(consts.TestFolder + "tmp/")

	// copy tests/hold-test-fake to working-dir
	err = copy.Copy(
		filepath.Join(consts.TestFolder, "working-dir-fake", "hold-operator", "redhat-operator-index", "v4.14"),
		filepath.Join(tempDir, "working-dir", operatorImageExtractDir, "redhat-operator-index", "f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"),
	)
	assert.NoError(t, err)

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
					Source:      "docker://localhost:9999/simple-test-bundle:eaf28fd0a9f205e44fb52a8b0bd8e678",
					Destination: "docker://localhost:5000/test/simple-test-bundle:latest",
					Origin:      consts.OciProtocol + filepath.Join(testDir, "simple-test-bundle"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "eaf28fd0a9f205e44fb52a8b0bd8e678",
				},
				{
					Source:      "docker://localhost:9999/redhat/redhat-operator-index:94563f14d54e0ea1d600fa8c002c204b",
					Destination: "docker://localhost:5000/test/redhat/redhat-operator-index:v4.14",
					Origin:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.14",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "94563f14d54e0ea1d600fa8c002c204b",
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
					Source:      "docker://localhost:9999/test-catalog:eaf28fd0a9f205e44fb52a8b0bd8e678",
					Destination: "docker://localhost:5000/test/test-catalog:v4.14",
					Origin:      consts.OciProtocol + filepath.Join(testDir, "simple-test-bundle"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "eaf28fd0a9f205e44fb52a8b0bd8e678",
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			ex := setupFilterCollector_DiskToMirror(tempDir, log)
			ex = ex.withConfig(testCase.config)
			res, err := ex.OperatorImageCollector(ctx)
			if testCase.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.ElementsMatch(t, testCase.expectedResult, res.AllImages)
		})
	}
}

func TestFilterCollectorM2M(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()

	testDir, err := filepath.Abs(consts.TestFolder)
	assert.NoError(t, err, "should get tests/ absolute path")

	type testCase struct {
		caseName       string
		config         v2alpha1.ImageSetConfiguration
		expectedResult []v2alpha1.CopyImageSchema
		expectedError  bool
	}

	ctx := context.Background()
	os.RemoveAll(consts.TestFolder + "hold-operator/")
	os.RemoveAll(consts.TestFolder + "operator-images")
	os.RemoveAll(consts.TestFolder + "tmp/")

	// copy tests/hold-test-fake to working-dir
	err = copy.Copy(
		filepath.Join(consts.TestFolder, "working-dir-fake", "hold-operator", "redhat-operator-index", "v4.14"),
		filepath.Join(tempDir, "working-dir", operatorImageExtractDir, "redhat", "redhat-operator-index", "f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"),
	)
	assert.NoError(t, err)

	err = os.MkdirAll(consts.TestFolder+"/catalog-on-disk1", 0o755)
	assert.NoError(t, err, "should create catalog dir")
	err = os.MkdirAll(consts.TestFolder+"/catalog-on-disk2", 0o755)
	assert.NoError(t, err, "should create catalog dir")
	err = os.MkdirAll(consts.TestFolder+"/catalog-on-disk3", 0o755)
	assert.NoError(t, err, "should create catalog dir")
	// copy tests/hold-test-fake to working-dir
	err = copy.Copy(
		filepath.Join(consts.TestFolder, "oci-image"),
		filepath.Join(consts.TestFolder, "catalog-on-disk1"),
	)
	assert.NoError(t, err)
	defer os.RemoveAll(consts.TestFolder + "/catalog-on-disk1")
	err = copy.Copy(
		filepath.Join(consts.TestFolder, "oci-image"),
		filepath.Join(consts.TestFolder, "catalog-on-disk2"),
	)
	assert.NoError(t, err)
	defer os.RemoveAll(consts.TestFolder + "/catalog-on-disk2")
	err = copy.Copy(
		filepath.Join(consts.TestFolder, "oci-image"),
		filepath.Join(consts.TestFolder, "catalog-on-disk3"),
	)
	assert.NoError(t, err)
	defer os.RemoveAll(consts.TestFolder + "/catalog-on-disk3")

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
					Source:      "docker://registry.redhat.io/redhat/community-operator-index:v4.18",
					Destination: "docker://localhost:5000/test/redhat/community-operator-index:v4.18",
					Origin:      "docker://registry.redhat.io/redhat/community-operator-index:v4.18",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
				{
					Source:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.17",
					Destination: "docker://localhost:9999/redhat/redhat-filtered-index:v4.17",
					Origin:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.17",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "b6db5253b0a8b995840d4d6b5a8aefca",
				},
				{
					Source:      "docker://registry.redhat.io/redhat/certified-operators:v4.17",
					Destination: "docker://localhost:9999/redhat/certified-operators-pinned:v4.17.0-20241114",
					Origin:      "docker://registry.redhat.io/redhat/certified-operators:v4.17",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "37e8b17cf0089fb1de93893cfb41dbfb",
				},
				{
					Source:      consts.OciProtocol + filepath.Join(testDir, "catalog-on-disk1"),
					Destination: "docker://localhost:9999/catalog-on-disk1:latest",
					Origin:      consts.OciProtocol + filepath.Join(testDir, "catalog-on-disk1"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "bff06b6d6cc99438ad7a080e38025b52",
				},
				{
					Source:      consts.OciProtocol + filepath.Join(testDir, "catalog-on-disk2"),
					Destination: "docker://localhost:9999/coffee-shop-index:latest",
					Origin:      consts.OciProtocol + filepath.Join(testDir, "catalog-on-disk2"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "04a29cd46d562afadfa317467451756e",
				},
				{
					Source:      consts.OciProtocol + filepath.Join(testDir, "catalog-on-disk3"),
					Destination: "docker://localhost:9999/tea-shop-index:v3.14",
					Origin:      consts.OciProtocol + filepath.Join(testDir, "catalog-on-disk3"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "4b3bae8f9360ced2d4a4473d5481cc9f",
				},

				{
					Source:      "docker://localhost:9999/redhat/redhat-filtered-index:b6db5253b0a8b995840d4d6b5a8aefca",
					Destination: "docker://localhost:5000/test/redhat/redhat-filtered-index:v4.17",
					Origin:      "docker://registry.redhat.io/redhat/redhat-operator-index:v4.17",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "b6db5253b0a8b995840d4d6b5a8aefca",
				},
				{
					Source:      "docker://localhost:9999/redhat/certified-operators-pinned:37e8b17cf0089fb1de93893cfb41dbfb",
					Destination: "docker://localhost:5000/test/redhat/certified-operators-pinned:v4.17.0-20241114",
					Origin:      "docker://registry.redhat.io/redhat/certified-operators:v4.17",
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "37e8b17cf0089fb1de93893cfb41dbfb",
				},
				{
					Source:      "docker://localhost:9999/catalog-on-disk1:bff06b6d6cc99438ad7a080e38025b52",
					Destination: "docker://localhost:5000/test/catalog-on-disk1:latest",
					Origin:      consts.OciProtocol + filepath.Join(testDir, "catalog-on-disk1"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "bff06b6d6cc99438ad7a080e38025b52",
				},
				{
					Source:      "docker://localhost:9999/coffee-shop-index:04a29cd46d562afadfa317467451756e",
					Destination: "docker://localhost:5000/test/coffee-shop-index:latest",
					Origin:      consts.OciProtocol + filepath.Join(testDir, "catalog-on-disk2"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "04a29cd46d562afadfa317467451756e",
				},
				{
					Source:      "docker://localhost:9999/tea-shop-index:4b3bae8f9360ced2d4a4473d5481cc9f",
					Destination: "docker://localhost:5000/test/tea-shop-index:v3.14",
					Origin:      consts.OciProtocol + filepath.Join(testDir, "catalog-on-disk3"),
					Type:        v2alpha1.TypeOperatorCatalog,
					RebuiltTag:  "4b3bae8f9360ced2d4a4473d5481cc9f",
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
			if testCase.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
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
		OperatorCollector{
			Log:              log,
			Mirror:           &MockMirror{Fail: false},
			Config:           nominalConfigD2M,
			Manifest:         manifest,
			Opts:             d2mOpts,
			LocalStorageFQDN: "localhost:9999",
			ctlgHandler:      handler,
		},
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
		Destination:         consts.FileProtocol + "test",
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
		LocalStorageFQDN:    "localhost:9999",
	}

	ex := &FilterCollector{
		OperatorCollector{
			Log:              log,
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
	return parser.ParseJsonFile[*v2alpha1.OperatorConfigSchema](path.Join(consts.TestFolder, "operator-config.json"))
}

func (o MockManifest) GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error) {
	relatedImages := []v2alpha1.RelatedImage{
		{Name: "testA", Image: "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testB", Image: "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testC", Image: "sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testD", Image: "sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
	}
	return relatedImages, nil
}

func (o MockManifest) GetImageIndex(name string) (*v2alpha1.OCISchema, error) {
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

func (o MockManifest) GetImageManifest(name string) (*v2alpha1.OCISchema, error) {
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

func (o MockManifest) GetOCIImageFromIndex(dir string) (gcrv1.Image, error) { //nolint:ireturn // as expected by go-containerregistry
	return &fake.FakeImage{
		ConfigFileStub: func() (*gcrv1.ConfigFile, error) {
			return &gcrv1.ConfigFile{}, nil
		},
	}, nil
}

func (o MockManifest) ExtractLayersOCI(_ gcrv1.Image, toPath, label string) error {
	if o.FailExtract {
		return fmt.Errorf("forced extract oci fail")
	}
	return nil
}

func (o MockManifest) ConvertIndexToSingleManifest(dir string, oci *v2alpha1.OCISchema) error {
	return nil
}

func (o MockManifest) GetDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	return "f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", nil
}

func (o MockManifest) ConvertOCIIndexToSingleManifest(dir string, oci *v2alpha1.OCISchema) error {
	return errors.New("not implemented")
}

func (o MockManifest) ExtractOCILayers(_ gcrv1.Image, to, label string) error {
	if o.FailExtract {
		return errors.New("forced extract to fail")
	}
	return nil
}

func (o MockManifest) GetOCIImageIndex(dir string) (*v2alpha1.OCISchema, error) {
	if o.FailImageIndex {
		return nil, errors.New("forced error image index")
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

func (o MockManifest) GetOCIImageManifest(dir string) (*v2alpha1.OCISchema, error) {
	if o.FailImageManifest {
		return nil, errors.New("forced error image manifest")
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

func (o MockManifest) ImageDigest(ctx context.Context, srcCtx *types.SystemContext, ref string) (string, error) {
	return "f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", nil
}

func (o MockManifest) ImageManifest(ctx context.Context, srcCtx *types.SystemContext, ref string, digest *digest.Digest) ([]byte, string, error) {
	return nil, "", errors.New("not implemented")
}

func (o MockHandler) getCatalog(filePath string) (OperatorCatalog, error) {
	return OperatorCatalog{}, nil
}

func (o MockHandler) getRelatedImagesFromCatalog(dc *declcfg.DeclarativeConfig, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) (map[string][]v2alpha1.RelatedImage, error) {
	relatedImages := make(map[string][]v2alpha1.RelatedImage)
	relatedImages["abc"] = []v2alpha1.RelatedImage{
		{Name: "testA", Image: "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testB", Image: "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "kube-rbac-proxy", Image: "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522"}, // OCPBUGS-37867
		{Name: "", Image: ""}, // OCPBUGS-31622
	}
	return relatedImages, nil
}

func (o MockHandler) filterRelatedImagesFromCatalog(operatorCatalog OperatorCatalog, op v2alpha1.Operator, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) (map[string][]v2alpha1.RelatedImage, error) {
	relatedImages := make(map[string][]v2alpha1.RelatedImage)
	relatedImages["abc"] = []v2alpha1.RelatedImage{
		{Name: "testA", Image: "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testB", Image: "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "kube-rbac-proxy", Image: "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522"}, // OCPBUGS-37867
		{Name: "", Image: ""}, // OCPBUGS-31622
	}
	return relatedImages, nil
}

func (o MockHandler) getDeclarativeConfig(filePath string) (*declcfg.DeclarativeConfig, error) {
	return &declcfg.DeclarativeConfig{
		Packages: []declcfg.Package{
			{Name: "op1", DefaultChannel: "ch1"},
		},
		Channels: []declcfg.Channel{
			{Name: "ch1", Package: "op1", Entries: []declcfg.ChannelEntry{{Name: "abc"}}},
		},
		Bundles: []declcfg.Bundle{
			{
				Name:    "abc",
				Package: "op1",
				RelatedImages: []declcfg.RelatedImage{
					{Name: "testA", Image: "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
					{Name: "testB", Image: "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
					{Name: "kube-rbac-proxy", Image: "gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1@sha256:d4883d7c622683b3319b5e6b3a7edfbf2594c18060131a8bf64504805f875522"},
				},
				Properties: []property.Property{
					{Type: property.TypePackage, Value: []byte(`{"packageName": "op1", "version": "1.0.0"}`)},
				},
			},
		},
	}, nil
}

func TestFindFilterDigest(t *testing.T) {
	t.Run("returns normalized digest when it exists", func(t *testing.T) {
		tempDir := t.TempDir()
		op := v2alpha1.Operator{
			Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.14",
			IncludeConfig: v2alpha1.IncludeConfig{
				Packages: []v2alpha1.IncludePackage{{Name: "op1"}},
			},
		}
		catalogDigest := "abc123"

		normalizedDigest, err := digestOfFilter(op, catalogDigest)
		assert.NoError(t, err)

		// Create the normalized digest folder
		normalizedDir := filepath.Join(tempDir, normalizedDigest)
		err = os.MkdirAll(normalizedDir, 0755)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(normalizedDir, "digest"), []byte("somefilteredimagedigest"), 0644) // #nosec G306
		assert.NoError(t, err)

		result, err := findFilterDigest(op, catalogDigest, tempDir)
		assert.NoError(t, err)
		assert.Equal(t, normalizedDigest, result)
	})

	t.Run("returns legacy digest when only legacy folder exists", func(t *testing.T) {
		tempDir := t.TempDir()
		op := v2alpha1.Operator{
			Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.14",
			IncludeConfig: v2alpha1.IncludeConfig{
				Packages: []v2alpha1.IncludePackage{{Name: "op1"}},
			},
		}
		catalogDigest := "abc123"

		normalizedDigest, err := digestOfFilter(op, catalogDigest)
		assert.NoError(t, err)
		legacyDigest, err := digestOfFilter(op, "")
		assert.NoError(t, err)
		assert.NotEqual(t, normalizedDigest, legacyDigest, "digests should differ for this test")

		// Create the legacy digest folder (not the normalized one)
		legacyDir := filepath.Join(tempDir, legacyDigest)
		err = os.MkdirAll(legacyDir, 0755)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(legacyDir, "digest"), []byte("somefilteredimagedigest"), 0644) // #nosec G306
		assert.NoError(t, err)

		result, err := findFilterDigest(op, catalogDigest, tempDir)
		assert.NoError(t, err)
		assert.Equal(t, legacyDigest, result, "should return the legacy digest for backwards compatibility")

		_, err = os.Stat(legacyDir)
		assert.NoError(t, err, "legacy folder should still exist")
	})

	t.Run("returns normalized digest when neither exists", func(t *testing.T) {
		tempDir := t.TempDir()
		op := v2alpha1.Operator{
			Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.14",
			IncludeConfig: v2alpha1.IncludeConfig{
				Packages: []v2alpha1.IncludePackage{{Name: "op1"}},
			},
		}
		catalogDigest := "abc123"

		normalizedDigest, err := digestOfFilter(op, catalogDigest)
		assert.NoError(t, err)

		result, err := findFilterDigest(op, catalogDigest, tempDir)
		assert.NoError(t, err)
		assert.Equal(t, normalizedDigest, result)
	})

	t.Run("returns same digest when legacy equals normalized", func(t *testing.T) {
		tempDir := t.TempDir()
		// When catalogDigest is empty, legacy and normalized should be the same
		op := v2alpha1.Operator{
			Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.14",
			IncludeConfig: v2alpha1.IncludeConfig{
				Packages: []v2alpha1.IncludePackage{{Name: "op1"}},
			},
		}
		catalogDigest := "" // empty catalog digest means no normalization

		normalizedDigest, err := digestOfFilter(op, catalogDigest)
		assert.NoError(t, err)
		legacyDigest, err := digestOfFilter(op, "")
		assert.NoError(t, err)
		assert.Equal(t, normalizedDigest, legacyDigest, "should be equal when catalogDigest is empty")

		result, err := findFilterDigest(op, catalogDigest, tempDir)
		assert.NoError(t, err)
		assert.Equal(t, normalizedDigest, result)
	})
}
