package operator

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func TestRelatedImagesFromCatalog(t *testing.T) {
	type testCase struct {
		caseName        string
		cfg             *declcfg.DeclarativeConfig
		expectedBundles []string
		expectedError   error
	}

	testCases := []testCase{
		{
			caseName:        "fail scenario - no related images found",
			cfg:             &declcfg.DeclarativeConfig{Packages: []declcfg.Package{{Name: "netscaler-operator"}}},
			expectedBundles: []string{},
			expectedError:   errors.New("no related images found"),
		},
	}

	handler := &CatalogHandler{Log: clog.New("debug")}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: make(map[string]map[string]struct{}), BundlesByImage: make(map[string]map[string]string)}
			res, err := handler.getRelatedImagesFromCatalog(testCase.cfg, v2alpha1.Operator{}, copyImageSchemaMap)
			if testCase.expectedError != nil {
				assert.EqualError(t, err, testCase.expectedError.Error())
			} else {
				assert.NoError(t, err)
				assert.Len(t, testCase.expectedBundles, len(res), "the number of expected bundles is different from the one returned")
			}
		})
	}
}

func TestFilterCatalog(t *testing.T) {
	type testCase struct {
		caseName        string
		cfg             v2alpha1.Operator
		expectedBundles []string
		expectedError   error
	}

	testCases := []testCase{
		{
			caseName: "only catalog (no filtering) - only heads of all channels - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{},
			},
			expectedBundles: []string{
				"3scale-operator.v0.11.0-mas",
				"devworkspace-operator.v0.19.1-0.1682321189.p",
				"3scale-operator.v0.9.1-0.1664967752.p",
				"3scale-operator.v0.8.4-0.1655690146.p",
				"jaeger-operator.v1.51.0-1",
			},
		},
		{
			caseName: "only catalog with full: true - all bundles of all channels of the specified catalog - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{},
				Full:          true,
			},
			expectedBundles: []string{
				"3scale-operator.v0.8.0",
				"3scale-operator.v0.8.0-0.1634606167.p",
				"3scale-operator.v0.8.1",
				"3scale-operator.v0.8.2",
				"3scale-operator.v0.8.3",
				"3scale-operator.v0.8.3-0.1645735250.p",
				"3scale-operator.v0.8.3-0.1646619125.p",
				"3scale-operator.v0.8.3-0.1646742992.p",
				"3scale-operator.v0.8.3-0.1649688682.p",
				"3scale-operator.v0.8.4",
				"3scale-operator.v0.8.4-0.1655690146.p",
				"3scale-operator.v0.9.0",
				"3scale-operator.v0.9.1",
				"3scale-operator.v0.9.1-0.1664967752.p",
				"3scale-operator.v0.10.0-mas",
				"3scale-operator.v0.11.0-mas",
				"devworkspace-operator.v0.9.0",
				"devworkspace-operator.v0.10.0",
				"devworkspace-operator.v0.11.0",
				"devworkspace-operator.v0.12.0",
				"devworkspace-operator.v0.13.0",
				"devworkspace-operator.v0.14.1",
				"devworkspace-operator.v0.15.2",
				"devworkspace-operator.v0.15.2-0.1661828401.p",
				"devworkspace-operator.v0.16.0",
				"devworkspace-operator.v0.16.0-0.1666668361.p",
				"devworkspace-operator.v0.17.0",
				"devworkspace-operator.v0.18.1",
				"devworkspace-operator.v0.18.1-0.1675929565.p",
				"devworkspace-operator.v0.19.1",
				"devworkspace-operator.v0.19.1-0.1679521112.p",
				"devworkspace-operator.v0.19.1-0.1682321189.p",
				"jaeger-operator.v1.30.2",
				"jaeger-operator.v1.34.1-5",
				"jaeger-operator.v1.42.0-5",
				"jaeger-operator.v1.42.0-5-0.1687199951.p",
				"jaeger-operator.v1.47.1-5",
				"jaeger-operator.v1.51.0-1",
			},
		},
		{
			caseName: "packages with no Min Max version (no channels) - 1 bundle, corresponding to the head version of any channel for selected packages - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name: "3scale-operator",
						},
						{
							Name: "devworkspace-operator",
						},
						{
							Name: "jaeger-product",
						},
					},
				},
			},
			expectedBundles: []string{
				"3scale-operator.v0.11.0-mas",
				"3scale-operator.v0.9.1-0.1664967752.p",
				"3scale-operator.v0.8.4-0.1655690146.p",
				"devworkspace-operator.v0.19.1-0.1682321189.p",
				"jaeger-operator.v1.51.0-1",
			},
		},
		{
			caseName: "packages with full: true (no channels) - all bundles of all channels for the packages specified - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name: "3scale-operator",
						},
						{
							Name: "devworkspace-operator",
						},
						{
							Name: "jaeger-product",
						},
					},
				},
				Full: true,
			},
			expectedBundles: []string{
				"3scale-operator.v0.8.0",
				"3scale-operator.v0.8.0-0.1634606167.p",
				"3scale-operator.v0.8.1",
				"3scale-operator.v0.8.2",
				"3scale-operator.v0.8.3",
				"3scale-operator.v0.8.3-0.1645735250.p",
				"3scale-operator.v0.8.3-0.1646619125.p",
				"3scale-operator.v0.8.3-0.1646742992.p",
				"3scale-operator.v0.8.3-0.1649688682.p",
				"3scale-operator.v0.8.4",
				"3scale-operator.v0.8.4-0.1655690146.p",
				"3scale-operator.v0.9.0",
				"3scale-operator.v0.9.1",
				"3scale-operator.v0.9.1-0.1664967752.p",
				"3scale-operator.v0.10.0-mas",
				"3scale-operator.v0.11.0-mas",
				"devworkspace-operator.v0.9.0",
				"devworkspace-operator.v0.10.0",
				"devworkspace-operator.v0.11.0",
				"devworkspace-operator.v0.12.0",
				"devworkspace-operator.v0.13.0",
				"devworkspace-operator.v0.14.1",
				"devworkspace-operator.v0.15.2",
				"devworkspace-operator.v0.15.2-0.1661828401.p",
				"devworkspace-operator.v0.16.0",
				"devworkspace-operator.v0.16.0-0.1666668361.p",
				"devworkspace-operator.v0.17.0",
				"devworkspace-operator.v0.18.1",
				"devworkspace-operator.v0.18.1-0.1675929565.p",
				"devworkspace-operator.v0.19.1",
				"devworkspace-operator.v0.19.1-0.1679521112.p",
				"devworkspace-operator.v0.19.1-0.1682321189.p",
				"jaeger-operator.v1.30.2",
				"jaeger-operator.v1.34.1-5",
				"jaeger-operator.v1.42.0-5",
				"jaeger-operator.v1.42.0-5-0.1687199951.p",
				"jaeger-operator.v1.47.1-5",
				"jaeger-operator.v1.51.0-1",
			},
		},
		{
			caseName: "packages with minVersion only (no channels) - all bundles in the default channel, from minVersion, up to channel head for that package (not relying of shortest path from upgrade graph) - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name: "3scale-operator",
						},
						{
							Name:          "devworkspace-operator",
							IncludeBundle: v2alpha1.IncludeBundle{MinVersion: "0.18.1"},
						},
						{
							Name: "jaeger-product",
						},
					},
				},
			},
			expectedBundles: []string{
				"3scale-operator.v0.11.0-mas",
				"3scale-operator.v0.9.1-0.1664967752.p",
				"3scale-operator.v0.8.4-0.1655690146.p",
				"devworkspace-operator.v0.18.1",
				"devworkspace-operator.v0.18.1-0.1675929565.p",
				"devworkspace-operator.v0.19.1",
				"devworkspace-operator.v0.19.1-0.1679521112.p",
				"devworkspace-operator.v0.19.1-0.1682321189.p",
				"jaeger-operator.v1.51.0-1",
			},
		},
		{
			caseName: "packages with maxVersion only (no channels) - all bundles in the default channel, that are lower than maxVersion for each package - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name: "3scale-operator",
						},
						{
							Name:          "devworkspace-operator",
							IncludeBundle: v2alpha1.IncludeBundle{MaxVersion: "0.18.1"},
						},
						{
							Name: "jaeger-product",
						},
					},
				},
			},
			expectedBundles: []string{
				"3scale-operator.v0.11.0-mas",
				"3scale-operator.v0.9.1-0.1664967752.p",
				"3scale-operator.v0.8.4-0.1655690146.p",
				"devworkspace-operator.v0.9.0",
				"devworkspace-operator.v0.10.0",
				"devworkspace-operator.v0.11.0",
				"devworkspace-operator.v0.12.0",
				"devworkspace-operator.v0.13.0",
				"devworkspace-operator.v0.14.1",
				"devworkspace-operator.v0.15.2",
				"devworkspace-operator.v0.15.2-0.1661828401.p",
				"devworkspace-operator.v0.16.0",
				"devworkspace-operator.v0.16.0-0.1666668361.p",
				"devworkspace-operator.v0.17.0",
				"devworkspace-operator.v0.18.1",
				"devworkspace-operator.v0.18.1-0.1675929565.p",
				"jaeger-operator.v1.51.0-1",
			},
		},
		{
			caseName: "packages with minVersion and maxVersion (no channels) - all bundles in the default channel, between minVersion and maxVersion for that package. Head of channel is not included, even if multiple channels are included in the filtering - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name: "3scale-operator",
						},
						{
							Name: "devworkspace-operator",
							IncludeBundle: v2alpha1.IncludeBundle{
								MinVersion: "0.16.0",
								MaxVersion: "0.17.0",
							},
						},
						{
							Name: "jaeger-product",
						},
					},
				},
			},
			expectedBundles: []string{
				"3scale-operator.v0.11.0-mas",
				"3scale-operator.v0.9.1-0.1664967752.p",
				"3scale-operator.v0.8.4-0.1655690146.p",
				"devworkspace-operator.v0.16.0",
				"devworkspace-operator.v0.16.0-0.1666668361.p",
				"devworkspace-operator.v0.17.0",
				"jaeger-operator.v1.51.0-1",
			},
		},
		{
			caseName: "packages with minVersion only (with channels) - within the selected channel of that package, all version starting minVersion up to channel head (not relying of shortest path from upgrade graph) - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name:           "3scale-operator",
							DefaultChannel: "threescale-2.11",
							Channels: []v2alpha1.IncludeChannel{
								{
									Name:          "threescale-2.11",
									IncludeBundle: v2alpha1.IncludeBundle{MinVersion: "0.8.3"},
								},
							},
						},
					},
				},
			},
			expectedBundles: []string{
				"3scale-operator.v0.8.3",
				"3scale-operator.v0.8.3-0.1645735250.p",
				"3scale-operator.v0.8.3-0.1646619125.p",
				"3scale-operator.v0.8.3-0.1646742992.p",
				"3scale-operator.v0.8.3-0.1649688682.p",
				"3scale-operator.v0.8.4",
				"3scale-operator.v0.8.4-0.1655690146.p",
			},
		},
		{
			caseName: "packages with channel name only - head bundle for the selected channel of that package - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name:           "3scale-operator",
							DefaultChannel: "threescale-2.11",
							Channels: []v2alpha1.IncludeChannel{
								{
									Name: "threescale-2.11",
								},
							},
						},
						{
							Name:           "devworkspace-operator",
							DefaultChannel: "fast",
							Channels: []v2alpha1.IncludeChannel{
								{
									Name: "fast",
								},
							},
						},
					},
				},
			},
			expectedBundles: []string{
				"3scale-operator.v0.8.4-0.1655690146.p",
				"devworkspace-operator.v0.19.1-0.1682321189.p",
			},
		},
		{
			caseName: "packages with multiple channels - head bundle for the each selected channel of that package - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name: "3scale-operator",
							Channels: []v2alpha1.IncludeChannel{
								{
									Name: "threescale-2.11",
								},
								{
									Name: "threescale-mas",
								},
							},
						},
					},
				},
			},
			expectedBundles: []string{
				"3scale-operator.v0.8.4-0.1655690146.p",
				"3scale-operator.v0.11.0-mas",
			},
		},
		{
			caseName: "packages with maxVersion only (with channels) - within the selected channel of that package, all versions up to maxVersion (not relying of shortest path from upgrade graph): Head of channel is not included, even if multiple channels are included in the filtering - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name:           "3scale-operator",
							DefaultChannel: "threescale-2.11",
							Channels: []v2alpha1.IncludeChannel{
								{
									Name:          "threescale-2.11",
									IncludeBundle: v2alpha1.IncludeBundle{MaxVersion: "0.8.2"},
								},
							},
						},
					},
				},
			},
			expectedBundles: []string{
				"3scale-operator.v0.8.0",
				"3scale-operator.v0.8.0-0.1634606167.p",
				"3scale-operator.v0.8.1",
				"3scale-operator.v0.8.2",
			},
		},
		{
			caseName: "packages with minVersion and maxVersion (with channels) - within the selected channel of that package, all versions between minVersion and maxVersion (not relying of shortest path from upgrade graph): Head of channel is not included, even if multiple channels are included in the filtering - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name:           "3scale-operator",
							DefaultChannel: "threescale-2.11",
							Channels: []v2alpha1.IncludeChannel{
								{
									Name:          "threescale-2.11",
									IncludeBundle: v2alpha1.IncludeBundle{MinVersion: "0.8.1", MaxVersion: "0.8.3"},
								},
							},
						},
					},
				},
			},
			expectedBundles: []string{
				"3scale-operator.v0.8.1",
				"3scale-operator.v0.8.2",
				"3scale-operator.v0.8.3",
				"3scale-operator.v0.8.3-0.1645735250.p",
				"3scale-operator.v0.8.3-0.1646619125.p",
				"3scale-operator.v0.8.3-0.1646742992.p",
				"3scale-operator.v0.8.3-0.1649688682.p",
			},
		},
		{
			caseName: "packages with Full:true (with channels) - all bundles for the packages and channels specified - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name:           "3scale-operator",
							DefaultChannel: "threescale-2.11",
							Channels: []v2alpha1.IncludeChannel{
								{
									Name: "threescale-2.11",
								},
							},
						},
					},
				},
				Full: true,
			},
			expectedBundles: []string{
				"3scale-operator.v0.8.0",
				"3scale-operator.v0.8.0-0.1634606167.p",
				"3scale-operator.v0.8.1",
				"3scale-operator.v0.8.2",
				"3scale-operator.v0.8.3",
				"3scale-operator.v0.8.3-0.1645735250.p",
				"3scale-operator.v0.8.3-0.1646619125.p",
				"3scale-operator.v0.8.3-0.1646742992.p",
				"3scale-operator.v0.8.3-0.1649688682.p",
				"3scale-operator.v0.8.4",
				"3scale-operator.v0.8.4-0.1655690146.p",
			},
		},
		{
			caseName: "packages with MinVersion MaxVersion with channels - Error: filtering by channel and by package min max should not be allowed - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name:           "3scale-operator",
							DefaultChannel: "threescale-2.11",
							Channels: []v2alpha1.IncludeChannel{
								{
									Name:          "threescale-2.11",
									IncludeBundle: v2alpha1.IncludeBundle{MinVersion: "1.2.3"},
								},
							},
							IncludeBundle: v2alpha1.IncludeBundle{
								MinVersion: "0.8.0",
								MaxVersion: "0.8.1",
							},
						},
					},
				},
			},
			expectedBundles: []string{},
			expectedError:   errors.New("failed to validate catalog filter: package \"3scale-operator\" at index [0] is invalid: package specifies a VersionRange, while channel \"threescale-2.11\" at index [0] equally specifies one: package.VersionRange and channel.VersionRange are exclusive"),
		},
		{
			caseName: "packages with full:true and min OR max version under packages - Error: filtering using full:true and min or max version is not allowed - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name: "3scale-operator",
							IncludeBundle: v2alpha1.IncludeBundle{
								MinVersion: "0.8.0",
								MaxVersion: "0.8.1",
							},
						},
					},
				},
				Full: true,
			},
			expectedBundles: []string{},
			expectedError:   errors.New("failed to filter catalog: Full: true cannot be mixed with versionRange"),
		},
		{
			caseName: "package not found - logs warning - should pass",
			cfg: v2alpha1.Operator{
				Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.16",
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name: "chocolate-factory-operator",
							IncludeBundle: v2alpha1.IncludeBundle{
								MinVersion: "0.8.0",
								MaxVersion: "0.8.1",
							},
						},
					},
				},
			},
			expectedBundles: []string{},
			expectedError:   nil,
		},
		{
			caseName: "filtering comes back empty - logs warning - should pass",
			cfg: v2alpha1.Operator{
				Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.16",
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name: "3scale-operator",
							IncludeBundle: v2alpha1.IncludeBundle{
								MinVersion: "77.77.77",
								MaxVersion: "77.77.77",
							},
						},
					},
				},
			},

			expectedBundles: []string{},
			expectedError:   errors.New("failed to filter catalog: error finding specific bundle: specific version 77.77.77 not found in bundles"),
		},
	}

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	log := clog.New("debug")
	handler := &CatalogHandler{Log: log}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			dc, err := handler.GetDeclarativeConfig(t.Context(), filepath.Join(consts.TestFolder, "configs"))
			assert.NoError(t, err)
			res, err := filterCatalog(context.TODO(), *dc, testCase.cfg)
			if testCase.expectedError != nil {
				assert.EqualError(t, err, testCase.expectedError.Error())
			} else {
				assert.NoError(t, err)
				assert.Len(t, testCase.expectedBundles, len(res.Bundles))

				allPresent := true
				for _, val := range testCase.expectedBundles {
					if !slices.ContainsFunc(res.Bundles, func(b declcfg.Bundle) bool {
						return b.Name == val
					}) {
						allPresent = false
						break
					}
				}
				assert.True(t, allPresent, "Not all expected bundles are present in the result")
			}
		})
	}
}

type securePolicyRecordingMirror struct {
	SecurePolicyAtRun bool
	RunCalled         bool
}

func (m *securePolicyRecordingMirror) Run(ctx context.Context, src, dest string, mode mirror.Mode, opts *mirror.CopyOptions) error {
	m.RunCalled = true
	if opts.Global != nil {
		m.SecurePolicyAtRun = opts.Global.SecurePolicy
	}
	return nil
}

func (m *securePolicyRecordingMirror) Check(ctx context.Context, imageName string, opts *mirror.CopyOptions, asCopySrc bool) (bool, error) {
	return true, nil
}

func TestEnsureCatalogInOCIFormatDoesNotMutateSharedSecurePolicy(t *testing.T) {
	log := clog.New("trace")
	tempDir := t.TempDir()

	global := &mirror.GlobalOptions{
		SecurePolicy: true,
		WorkingDir:   filepath.Join(tempDir, "working-dir"),
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, retryOpts := mirror.RetryFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")

	recordingMirror := &securePolicyRecordingMirror{}
	handler := CatalogHandler{
		Log:    log,
		Mirror: recordingMirror,
	}

	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Mode:                mirror.MirrorToDisk,
		LocalStorageFQDN:    "localhost:9999",
	}

	imgSpec, err := image.ParseRef("docker://registry.example.com/catalog:v1.0")
	assert.NoError(t, err)

	imageIndexDir := filepath.Join(tempDir, "working-dir", operatorCatalogsDir, "catalog", "abc123")
	err = handler.EnsureCatalogInOCIFormat(context.Background(), imgSpec, "registry.example.com/catalog:v1.0", imageIndexDir, opts)
	assert.NoError(t, err)
	assert.True(t, recordingMirror.RunCalled, "Mirror.Run should be called for docker:// catalog")
	assert.False(t, recordingMirror.SecurePolicyAtRun, "local copy passed to Mirror.Run should have SecurePolicy=false")
	assert.True(t, global.SecurePolicy, "shared GlobalOptions.SecurePolicy must remain true after EnsureCatalogInOCIFormat")
}

func TestRelatedImageSelection(t *testing.T) {
	type testCase struct {
		caseName  string
		selectors []*metav1.LabelSelector
		imgLabels map[string]string
		expected  bool
	}

	testCases := []testCase{
		{
			caseName:  "no selectors - an unlabeled image is selected",
			selectors: nil,
			imgLabels: nil,
			expected:  true,
		},
		{
			caseName:  "no selectors - a labeled image is not selected",
			selectors: nil,
			imgLabels: map[string]string{"CoolFeatureA": ""},
			expected:  false,
		},
		{
			caseName:  "an unlabeled image is selected even when selectors match nothing",
			selectors: []*metav1.LabelSelector{{MatchLabels: map[string]string{"tier": "frontend"}}},
			imgLabels: nil,
			expected:  true,
		},
		{
			caseName: "Exists selects an image carrying the key",
			selectors: []*metav1.LabelSelector{
				{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "CoolFeatureA", Operator: metav1.LabelSelectorOpExists}}},
			},
			imgLabels: map[string]string{"CoolFeatureA": ""},
			expected:  true,
		},
		{
			caseName: "Exists does not select an image carrying another key",
			selectors: []*metav1.LabelSelector{
				{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "CoolFeatureA", Operator: metav1.LabelSelectorOpExists}}},
			},
			imgLabels: map[string]string{"CoolFeatureC": ""},
			expected:  false,
		},
		{
			caseName: "DoesNotExist selects every labeled image but the excluded one",
			selectors: []*metav1.LabelSelector{
				{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "CoolFeatureC", Operator: metav1.LabelSelectorOpDoesNotExist}}},
			},
			imgLabels: map[string]string{"CoolFeatureA": ""},
			expected:  true,
		},
		{
			caseName: "DoesNotExist does not select the excluded image",
			selectors: []*metav1.LabelSelector{
				{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "CoolFeatureC", Operator: metav1.LabelSelectorOpDoesNotExist}}},
			},
			imgLabels: map[string]string{"CoolFeatureC": ""},
			expected:  false,
		},
		{
			caseName: "requirements of a single selector are ANDed",
			selectors: []*metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"version": "1.2.3"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "GreatFeatureB", Operator: metav1.LabelSelectorOpExists},
						{Key: "tier", Operator: metav1.LabelSelectorOpIn, Values: []string{"frontend"}},
					},
				},
			},
			imgLabels: map[string]string{"GreatFeatureB": "", "tier": "frontend", "version": "1.2.3"},
			expected:  true,
		},
		{
			caseName: "an image missing one requirement of a selector is not selected",
			selectors: []*metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"version": "1.2.3"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "GreatFeatureB", Operator: metav1.LabelSelectorOpExists},
						{Key: "tier", Operator: metav1.LabelSelectorOpIn, Values: []string{"frontend"}},
					},
				},
			},
			imgLabels: map[string]string{"GreatFeatureB": "", "tier": "backend", "version": "1.2.3"},
			expected:  false,
		},
		{
			caseName: "selectors of a package are ORed",
			selectors: []*metav1.LabelSelector{
				{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "CoolFeatureA", Operator: metav1.LabelSelectorOpExists}}},
				{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "GreatFeatureB", Operator: metav1.LabelSelectorOpExists}}},
			},
			imgLabels: map[string]string{"GreatFeatureB": ""},
			expected:  true,
		},
		{
			caseName:  "an empty selector selects every image",
			selectors: []*metav1.LabelSelector{{}},
			imgLabels: map[string]string{"CoolFeatureC": ""},
			expected:  true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			selectors, err := packageSelectors([]v2alpha1.IncludePackage{{Name: "aws-load-balancer-operator", Selectors: testCase.selectors}})
			assert.NoError(t, err)
			assert.Equal(t, testCase.expected, isRelatedImageSelected(testCase.imgLabels, selectors["aws-load-balancer-operator"]))
		})
	}
}

func TestPackageSelectorsRejectsInvalidSelector(t *testing.T) {
	_, err := packageSelectors([]v2alpha1.IncludePackage{{
		Name: "3scale-operator",
		Selectors: []*metav1.LabelSelector{
			{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "tier", Operator: "Bogus"}}},
		},
	}})
	assert.ErrorContains(t, err, "3scale-operator")
}

// labeledCatalog builds a catalog holding one bundle per package, each with an
// unlabeled image, an image labeled CoolFeatureA and an image labeled CoolFeatureC.
func labeledCatalog(packageNames ...string) *declcfg.DeclarativeConfig {
	dc := &declcfg.DeclarativeConfig{}
	for _, packageName := range packageNames {
		bundleImage := "registry.redhat.io/" + packageName + "-bundle:v1.2.3"
		dc.Bundles = append(dc.Bundles, declcfg.Bundle{
			Name:    packageName + ".v1.2.3",
			Package: packageName,
			Image:   bundleImage,
			RelatedImages: []declcfg.RelatedImage{
				{Name: "bundle", Image: bundleImage},
				{Name: "controller", Image: "registry.redhat.io/" + packageName + "-controller:v1.2.3"},
				{Name: "feature-a", Image: "registry.redhat.io/feature-a:v1.2.3", Labels: map[string]string{"CoolFeatureA": ""}},
				{Name: "feature-c", Image: "registry.redhat.io/feature-c:v1.2.3", Labels: map[string]string{"CoolFeatureC": ""}},
			},
		})
	}
	return dc
}

func relatedImageNames(images []v2alpha1.RelatedImage) []string {
	names := make([]string, 0, len(images))
	for _, img := range images {
		names = append(names, img.Name)
	}
	return names
}

func TestRelatedImagesFromCatalogWithSelectors(t *testing.T) {
	type testCase struct {
		caseName      string
		operator      v2alpha1.Operator
		expectedNames []string
	}

	testCases := []testCase{
		{
			caseName:      "a package without selectors keeps only the images without labels",
			operator:      v2alpha1.Operator{IncludeConfig: v2alpha1.IncludeConfig{Packages: []v2alpha1.IncludePackage{{Name: "3scale-operator"}}}},
			expectedNames: []string{"bundle", "controller"},
		},
		{
			caseName:      "a package absent from the configuration keeps only the images without labels",
			operator:      v2alpha1.Operator{},
			expectedNames: []string{"bundle", "controller"},
		},
		{
			caseName: "a selector adds the images it matches to the images without labels",
			operator: v2alpha1.Operator{IncludeConfig: v2alpha1.IncludeConfig{Packages: []v2alpha1.IncludePackage{{
				Name:      "3scale-operator",
				Selectors: []*metav1.LabelSelector{{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "CoolFeatureA", Operator: metav1.LabelSelectorOpExists}}}},
			}}}},
			expectedNames: []string{"bundle", "controller", "feature-a"},
		},
		{
			caseName: "a DoesNotExist selector keeps every image but the excluded one",
			operator: v2alpha1.Operator{IncludeConfig: v2alpha1.IncludeConfig{Packages: []v2alpha1.IncludePackage{{
				Name:      "3scale-operator",
				Selectors: []*metav1.LabelSelector{{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "CoolFeatureC", Operator: metav1.LabelSelectorOpDoesNotExist}}}},
			}}}},
			expectedNames: []string{"bundle", "controller", "feature-a"},
		},
	}

	handler := &CatalogHandler{Log: clog.New("debug")}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: make(map[string]map[string]struct{}), BundlesByImage: make(map[string]map[string]string)}
			res, err := handler.getRelatedImagesFromCatalog(labeledCatalog("3scale-operator"), testCase.operator, copyImageSchemaMap)
			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedNames, relatedImageNames(res["3scale-operator.v1.2.3"]))
		})
	}
}

func TestRelatedImagesFromCatalogDoesNotRegisterSkippedImages(t *testing.T) {
	handler := &CatalogHandler{Log: clog.New("debug")}
	copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: make(map[string]map[string]struct{}), BundlesByImage: make(map[string]map[string]string)}

	_, err := handler.getRelatedImagesFromCatalog(labeledCatalog("3scale-operator"), v2alpha1.Operator{}, copyImageSchemaMap)
	assert.NoError(t, err)

	assert.NotContains(t, copyImageSchemaMap.OperatorsByImage, "docker://registry.redhat.io/feature-a:v1.2.3")
	assert.NotContains(t, copyImageSchemaMap.BundlesByImage, "docker://registry.redhat.io/feature-a:v1.2.3")
	assert.Contains(t, copyImageSchemaMap.OperatorsByImage, "docker://registry.redhat.io/3scale-operator-controller:v1.2.3")
}

func TestRelatedImagesFromCatalogLogsSkippedImages(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	handler := &CatalogHandler{Log: clog.New("info")}
	copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: make(map[string]map[string]struct{}), BundlesByImage: make(map[string]map[string]string)}

	_, err := handler.getRelatedImagesFromCatalog(labeledCatalog("3scale-operator"), v2alpha1.Operator{}, copyImageSchemaMap)
	assert.NoError(t, err)

	assert.Contains(t, buf.String(), "image registry.redhat.io/feature-a:v1.2.3 is not mirrored in bundle 3scale-operator.v1.2.3")
}

func TestRelatedImagesFromCatalogKeepsAnImageSelectedByAnotherPackage(t *testing.T) {
	handler := &CatalogHandler{Log: clog.New("debug")}
	copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: make(map[string]map[string]struct{}), BundlesByImage: make(map[string]map[string]string)}

	operator := v2alpha1.Operator{IncludeConfig: v2alpha1.IncludeConfig{Packages: []v2alpha1.IncludePackage{
		{Name: "3scale-operator"},
		{
			Name:      "aws-load-balancer-operator",
			Selectors: []*metav1.LabelSelector{{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "CoolFeatureA", Operator: metav1.LabelSelectorOpExists}}}},
		},
	}}}

	res, err := handler.getRelatedImagesFromCatalog(labeledCatalog("3scale-operator", "aws-load-balancer-operator"), operator, copyImageSchemaMap)
	assert.NoError(t, err)

	assert.NotContains(t, relatedImageNames(res["3scale-operator.v1.2.3"]), "feature-a")
	assert.Contains(t, relatedImageNames(res["aws-load-balancer-operator.v1.2.3"]), "feature-a")
}

func TestRelatedImagesFromCatalogRejectsInvalidSelector(t *testing.T) {
	handler := &CatalogHandler{Log: clog.New("debug")}
	copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: make(map[string]map[string]struct{}), BundlesByImage: make(map[string]map[string]string)}

	operator := v2alpha1.Operator{IncludeConfig: v2alpha1.IncludeConfig{Packages: []v2alpha1.IncludePackage{{
		Name:      "3scale-operator",
		Selectors: []*metav1.LabelSelector{{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "tier", Operator: "Bogus"}}}},
	}}}}

	_, err := handler.getRelatedImagesFromCatalog(labeledCatalog("3scale-operator"), operator, copyImageSchemaMap)
	assert.ErrorContains(t, err, "3scale-operator")
}
