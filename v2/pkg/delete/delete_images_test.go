package delete

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	mirror "github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/stretchr/testify/assert"
)

// TestAllDeleteImages
func TestAllDeleteImages(t *testing.T) {
	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		TlsVerify:         false,
		SecurePolicy:      false,
		Quiet:             false,
		WorkingDir:        "../../tests/",
		DeleteDestination: "docker://myregistry",
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
		Destination:         "docker://myregistry",
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
	}

	isc := v1alpha2.ImageSetConfiguration{
		ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
			Mirror: v1alpha2.Mirror{
				Operators: []v1alpha2.Operator{
					{
						Catalog: "redhat-operator-index:v4.14",
						IncludeConfig: v1alpha2.IncludeConfig{
							Packages: []v1alpha2.IncludePackage{
								{Name: "node-observability-operator"},
							},
						},
					},
				},
				AdditionalImages: []v1alpha2.Image{
					{Name: "test.registry.io/test-image@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2"},
				},
			},
		},
	}

	di := New(log, opts, &mockBatch{}, &mockBlobs{}, isc, &mockManifest{}, "/tmp", "localhost:8888")

	t.Run("Testing ReadDeleteData : should pass", func(t *testing.T) {
		opts.Global.WorkingDir = "../../tests"
		data, err := di.ReadDeleteMetaData()
		if err != nil {
			t.Fatal("should not fail")
		}
		assert.Equal(t, "docker://localhost:55000/openshift-release-dev/ocp-v4.0-art-dev@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2", data.Items[0].ImageReference)
	})

	t.Run("Testing ConvertReleaseImages : should pass", func(t *testing.T) {
		ri := []v1alpha3.RelatedImage{
			{
				Name:  "node-observability-agent",
				Image: "test.registry.io/test-image:v1.0.0",
			},
			{
				Name:  "node-observability-controller",
				Image: "test.registry.io/test-image-controller:v1.0.0",
			},
		}
		data, err := di.ConvertReleaseImages(ri)
		if err != nil {
			t.Fatal("should not fail")
		}
		assert.Equal(t, len(data), 2)
		assert.Equal(t, "docker://myregistry/test-image:v1.0.0", data[0].Destination)
	})

	t.Run("Testing DeleteRegistryImages : should pass", func(t *testing.T) {
		opts.Global.WorkingDir = "../../tests"
		imgs, err := di.ReadDeleteMetaData()
		if err != nil {
			t.Fatal("should not fail")
		}
		err = di.DeleteRegistryImages(imgs)
		if err != nil {
			t.Fatal("should not fail")
		}
	})

	t.Run("Testing DeleteCacheBlobs : should pass", func(t *testing.T) {
		testFolder := t.TempDir()
		defer os.RemoveAll(testFolder)
		opts.Global.WorkingDir = "../../tests"
		opts.Global.ForceCacheDelete = true
		deleteDI := New(log, opts, &mockBatch{}, &mockBlobs{}, v1alpha2.ImageSetConfiguration{}, &mockManifest{}, "/tmp", "localhost:8888")
		imgs, err := di.ReadDeleteMetaData()
		if err != nil {
			t.Fatal("should not fail")
		}

		err = deleteDI.DeleteRegistryImages(imgs)
		if err != nil {
			t.Fatal("should not fail")
		}
	})
}

// TestWriteMetaData
func TestWriteMetaData(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()
	defer os.RemoveAll(tempDir)

	global := &mirror.GlobalOptions{
		TlsVerify:    false,
		SecurePolicy: false,
		Quiet:        false,
		WorkingDir:   tempDir,
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
		Destination:         "oci:test",
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
	}

	cfg := v1alpha2.ImageSetConfiguration{}
	di := New(log, opts, &mockBatch{}, &mockBlobs{}, cfg, &mockManifest{}, "/tmp", "localhost:8888")

	t.Run("Testing ReadDeleteData : should pass", func(t *testing.T) {
		cpImages := []v1alpha3.CopyImageSchema{
			{
				Source:      "docker://localhost:55000/openshift-release-dev/ocp-v4.0-art-dev@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2",
				Destination: "docker://localhost:55000/openshift-release-dev/ocp-v4.0-art-dev@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2",
				Origin:      "test",
			},
		}
		err := di.WriteDeleteMetaData(cpImages)
		if err != nil {
			t.Fatalf("should not fail %v", err)
		}
	})
}

func TestFilterReleasesForDelete(t *testing.T) {
	log := clog.New("trace")

	globalM2D := &mirror.GlobalOptions{
		TlsVerify:    false,
		SecurePolicy: false,
		WorkingDir:   "../../tests/release-semver-test/",
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, retryOpts := mirror.RetryFlags()
	_, srcOptsM2D := mirror.ImageSrcFlags(globalM2D, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOptsM2D := mirror.ImageDestFlags(globalM2D, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")

	delOpts := mirror.CopyOptions{
		Global:              globalM2D,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOptsM2D,
		DestImage:           destOptsM2D,
		RetryOpts:           retryOpts,
		Destination:         "file://test",
		Dev:                 false,
		Function:            "delete",
	}

	tests := []struct {
		name          string
		cfg           v1alpha2.ImageSetConfiguration
		expectedCount int
		err           error
	}{
		{
			name: "cross matrix a : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name: "stable-4.14",
								},
							},
						},
					},
				},
			},
			expectedCount: 3,
			err:           nil,
		},
		{
			name: "cross matrix b : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MinVersion: "4.14.1",
								},
							},
						},
					},
				},
			},
			expectedCount: 3,
			err:           nil,
		},
		{
			name: "cross matrix c : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MaxVersion: "4.14.2",
								},
							},
						},
					},
				},
			},
			expectedCount: 2,
			err:           nil,
		},
		{
			name: "cross matrix d : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MinVersion: "4.14.1",
									MaxVersion: "4.14.1",
								},
							},
						},
					},
				},
			},
			expectedCount: 1,
			err:           nil,
		},
		{
			name: "cross matrix e : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name: "stable-4.14",
								},
							},
							Architectures: []string{"multi"},
						},
					},
				},
			},
			expectedCount: 1,
			err:           nil,
		},
		{
			name: "cross matrix f : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MinVersion: "4.14.1",
								},
							},
							Architectures: []string{"multi"},
						},
					},
				},
			},
			expectedCount: 1,
			err:           nil,
		},
		{
			name: "cross matrix g : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MaxVersion: "4.14.2",
								},
							},
							Architectures: []string{"multi"},
						},
					},
				},
			},
			expectedCount: 0,
			err:           nil,
		},
		{
			name: "cross matrix h : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MinVersion: "4.14.1",
									MaxVersion: "4.14.1",
								},
							},
							Architectures: []string{"multi"},
						},
					},
				},
			},
			expectedCount: 0,
			err:           nil,
		},
		{
			name: "cross matrix i : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name: "stable-4.14",
								},
							},
							Architectures: []string{"amd64"},
						},
					},
				},
			},
			expectedCount: 3,
			err:           nil,
		},
		{
			name: "cross matrix j : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MinVersion: "4.14.1",
								},
							},
							Architectures: []string{"amd64"},
						},
					},
				},
			},
			expectedCount: 3,
			err:           nil,
		},
		{
			name: "cross matrix k : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MaxVersion: "4.14.2",
								},
							},
							Architectures: []string{"amd64"},
						},
					},
				},
			},
			expectedCount: 2,
			err:           nil,
		},
		{
			name: "cross matrix l : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MinVersion: "4.14.1",
									MaxVersion: "4.14.1",
								},
							},
							Architectures: []string{"amd64"},
						},
					},
				},
			},
			expectedCount: 1,
			err:           nil,
		},
		{
			name: "cross matrix m : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name: "stable-4.14",
								},
							},
							Architectures: []string{"arm64"},
						},
					},
				},
			},
			expectedCount: 1,
			err:           nil,
		},
		{
			name: "cross matrix n : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MinVersion: "4.14.1",
								},
							},
							Architectures: []string{"arm64"},
						},
					},
				},
			},
			expectedCount: 1,
			err:           nil,
		},
		{
			name: "cross matrix o : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MaxVersion: "4.14.2",
								},
							},
							Architectures: []string{"arm64"},
						},
					},
				},
			},
			expectedCount: 1,
			err:           nil,
		},
		{
			name: "cross matrix p : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MinVersion: "4.14.1",
									MaxVersion: "4.14.1",
								},
							},
							Architectures: []string{"arm64"},
						},
					},
				},
			},
			expectedCount: 0,
			err:           nil,
		},
		{
			name: "cross matrix q : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name: "stable-4.14",
								},
							},
							Architectures: []string{"ppc64le"},
						},
					},
				},
			},
			expectedCount: 1,
			err:           nil,
		},
		{
			name: "cross matrix r : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MinVersion: "4.14.1",
								},
							},
							Architectures: []string{"ppc64le"},
						},
					},
				},
			},
			expectedCount: 1,
			err:           nil,
		},
		{
			name: "cross matrix s : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MaxVersion: "4.14.2",
								},
							},
							Architectures: []string{"ppc64le"},
						},
					},
				},
			},
			expectedCount: 1,
			err:           nil,
		},
		{
			name: "cross matrix t : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MinVersion: "4.14.1",
									MaxVersion: "4.14.1",
								},
							},
							Architectures: []string{"ppc64le"},
						},
					},
				},
			},
			expectedCount: 0,
			err:           nil,
		},
		{
			name: "cross matrix u : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name: "stable-4.14",
								},
							},
							Architectures: []string{"s390x"},
						},
					},
				},
			},
			expectedCount: 1,
			err:           nil,
		},
		{
			name: "cross matrix v : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MinVersion: "4.14.1",
								},
							},
							Architectures: []string{"s390x"},
						},
					},
				},
			},
			expectedCount: 1,
			err:           nil,
		},
		{
			name: "cross matrix w : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MaxVersion: "4.14.2",
								},
							},
							Architectures: []string{"s390x"},
						},
					},
				},
			},
			expectedCount: 1,
			err:           nil,
		},
		{
			name: "cross matrix x : should pass",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.14",
									MinVersion: "4.14.1",
									MaxVersion: "4.14.1",
								},
							},
							Architectures: []string{"s390x"},
						},
					},
				},
			},
			expectedCount: 0,
			err:           nil,
		},
	}

	t.Run("Testing FilterReleasesForDelete : cross matrix", func(t *testing.T) {

		var arch, minVersion, maxVersion string
		for _, tt := range tests {
			log.Info("Executing  %s ", tt.name)
			if len(tt.cfg.ImageSetConfigurationSpec.Mirror.Platform.Architectures) == 0 {
				arch = "amd64"
			} else {
				arch = tt.cfg.ImageSetConfigurationSpec.Mirror.Platform.Architectures[0]
			}
			if len(tt.cfg.Mirror.Platform.Channels) == 0 {
				minVersion = "none"
				maxVersion = "none"
			} else {
				if tt.cfg.Mirror.Platform.Channels[0].MinVersion == "" {
					minVersion = "none"
				} else {
					minVersion = tt.cfg.Mirror.Platform.Channels[0].MinVersion
				}
				if tt.cfg.Mirror.Platform.Channels[0].MaxVersion == "" {
					maxVersion = "none"
				} else {
					maxVersion = tt.cfg.Mirror.Platform.Channels[0].MaxVersion
				}
			}
			log.Info("arch = %s", arch)
			log.Info("minVersion = %s", minVersion)
			log.Info("maxVersion = %s", maxVersion)
			deleteID := New(log, delOpts, &mockBatch{}, &mockBlobs{}, tt.cfg, &mockManifest{}, "/tmp", "localhost:8888")
			res, err := deleteID.FilterReleasesForDelete()
			if err != tt.err {
				t.Fatalf("should pass: %v", err)
			}
			assert.Equal(t, tt.expectedCount, len(res))
			fmt.Println("")
		}
	})
}

// mockBatch
type mockBatch struct {
	Fail bool
}

// mockBlobs
type mockBlobs struct {
	Fail bool
}

type mockManifest struct{}

func (o *mockBatch) Worker(ctx context.Context, images []v1alpha3.CopyImageSchema, opts mirror.CopyOptions) error {
	if o.Fail {
		return fmt.Errorf("forced error")
	}
	return nil
}

func (o *mockBlobs) GatherBlobs(ctx context.Context, image string) (map[string]string, error) {
	res := map[string]string{"sha256:95ad8395795ee0460baf05458f669d3b865535f213f015519ef9a221a6a08280": ""}
	if o.Fail {
		return nil, fmt.Errorf("forced error")
	}
	return res, nil
}

func (o mockManifest) GetImageIndex(dir string) (*v1alpha3.OCISchema, error) {
	return &v1alpha3.OCISchema{}, nil
}

func (o mockManifest) GetImageManifest(file string) (*v1alpha3.OCISchema, error) {
	return &v1alpha3.OCISchema{}, nil
}

func (o mockManifest) GetOperatorConfig(file string) (*v1alpha3.OperatorConfigSchema, error) {
	return &v1alpha3.OperatorConfigSchema{}, nil
}

func (o mockManifest) GetCatalog(filePath string) (manifest.OperatorCatalog, error) {
	return manifest.OperatorCatalog{}, nil
}

func (o mockManifest) GetRelatedImagesFromCatalog(operatorCatalog manifest.OperatorCatalog, ctlgInIsc v1alpha2.Operator) (map[string][]v1alpha3.RelatedImage, error) {
	res := map[string][]v1alpha3.RelatedImage{}
	ri := []v1alpha3.RelatedImage{
		{
			Name:  "node-observability-agent",
			Image: "test.registry.io/test-image:v1.0.0",
		},
		{
			Name:  "node-observability-controller",
			Image: "test.registry.io/test-image-controller:v1.0.0",
		},
	}
	res["node-observability-operator"] = ri
	return res, nil
}

func (o mockManifest) ExtractLayersOCI(filePath, toPath, label string, oci *v1alpha3.OCISchema) error {
	return nil
}

func (o mockManifest) GetReleaseSchema(filePath string) ([]v1alpha3.RelatedImage, error) {
	return []v1alpha3.RelatedImage{}, nil
}

func (o mockManifest) ConvertIndexToSingleManifest(dir string, oci *v1alpha3.OCISchema) error {
	return nil
}

func (o mockManifest) GetDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	return "", nil
}
