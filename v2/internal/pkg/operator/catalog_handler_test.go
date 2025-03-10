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

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/stretchr/testify/assert"
)

func TestFilterRelatedImagesFromCatalog(t *testing.T) {
	type testCase struct {
		caseName        string
		cfg             v2alpha1.Operator
		expectedBundles []string
		expectedError   error
		expectedWarning string
	}

	testCases := []testCase{
		{
			caseName: "only catalog (no filtering) - heads for all channels - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{},
			},
			expectedBundles: []string{
				"3scale-operator.v0.11.0-mas",
				"devworkspace-operator.v0.19.1-0.1682321189.p",
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
			caseName: "packages with no Min Max version (no channels) - 1 bundle, corresponding to the head version of the default channel for each package - should pass",
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
							Name: "3scale-operator",
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
							Name: "3scale-operator",
							Channels: []v2alpha1.IncludeChannel{
								{
									Name: "threescale-2.11",
								},
							},
						},
						{
							Name: "devworkspace-operator",
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
							Name: "3scale-operator",
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
							Name: "3scale-operator",
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
							Name: "3scale-operator",
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
							Name: "3scale-operator",
							Channels: []v2alpha1.IncludeChannel{
								{
									Name: "threescale-2.11",
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
			expectedError:   errors.New("cannot use channels/full and min/max versions at the same time"),
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
			expectedError:   errors.New("cannot use channels/full and min/max versions at the same time"),
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
			expectedWarning: "package chocolate-factory-operator not found in catalog registry.redhat.io/redhat/redhat-operator-index:v4.16",
		},
		{
			caseName: "filtering comes back empty - should fail",
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
			expectedError:   nil,
			expectedWarning: "no bundles matching filtering for 3scale-operator in catalog registry.redhat.io/redhat/redhat-operator-index:v4.16",
		},
	}

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	log := clog.New("debug")
	handler := &catalogHandler{Log: log}

	operatorCatalog, err := handler.getCatalog(filepath.Join(common.TestFolder, "configs"))
	assert.NoError(t, err)

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {

			var res map[string][]v2alpha1.RelatedImage
			var err error
			copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: make(map[string]map[string]struct{}), BundlesByImage: make(map[string]map[string]string)}

			res, err = handler.filterRelatedImagesFromCatalog(operatorCatalog, testCase.cfg, copyImageSchemaMap)

			if testCase.expectedError == nil {
				assert.NoError(t, err)
			}

			allPresent := true
			for _, val := range testCase.expectedBundles {
				if _, ok := res[val]; !ok {
					allPresent = false
					break
				}
			}

			assert.True(t, allPresent, "Not all expected bundles are present in the result")
			assert.Equal(t, len(testCase.expectedBundles), len(res), "the number of expected bundles is different from the one returned")

			if testCase.expectedError != nil && (err == nil || err.Error() != testCase.expectedError.Error()) {
				assert.EqualError(t, err, testCase.expectedError.Error())
			}

			if testCase.expectedWarning != "" {
				assert.Contains(t, buf.String(), testCase.expectedWarning)

			}
			t.Log(buf.String())
			buf.Reset()

			log.Debug("completed test  %v ", res)
		})
	}
}

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

	handler := &catalogHandler{Log: clog.New("debug")}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {

			var res map[string][]v2alpha1.RelatedImage
			var err error
			copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: make(map[string]map[string]struct{}), BundlesByImage: make(map[string]map[string]string)}

			res, err = handler.getRelatedImagesFromCatalog(testCase.cfg, copyImageSchemaMap)

			assert.Equal(t, len(testCase.expectedBundles), len(res), "the number of expected bundles is different from the one returned")

			if testCase.expectedError != nil {
				assert.EqualError(t, err, testCase.expectedError.Error())
			}
		})
	}
}

func TestTypesOnRelatedImages(t *testing.T) {

	type testCase struct {
		caseName string
		cfg      v2alpha1.Operator
	}

	testCases := []testCase{
		{
			caseName: "only catalog (no filtering) - only the head of the default channel - should pass",
			cfg: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{},
			},
		},
	}

	log := clog.New("debug")

	handler := &catalogHandler{Log: log}

	operatorCatalog, err := handler.getCatalog(filepath.Join(common.TestFolder, "configs"))
	assert.NoError(t, err)

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {

			var bundles map[string][]v2alpha1.RelatedImage
			var err error
			copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: make(map[string]map[string]struct{}), BundlesByImage: make(map[string]map[string]string)}

			bundles, err = handler.filterRelatedImagesFromCatalog(operatorCatalog, testCase.cfg, copyImageSchemaMap)

			assert.NoError(t, err)

			for _, relatedImages := range bundles {
				for _, ri := range relatedImages {
					assert.NotEqual(t, v2alpha1.TypeInvalid, ri.Type, "Type should be catalog")
				}
			}

			log.Debug("completed test  %v ", bundles)
		})
	}

}

func TestFilterCatalog(t *testing.T) {
	type testCase struct {
		caseName        string
		cfg             v2alpha1.Operator
		expectedBundles []string
		expectedError   error
		expectedWarning string
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
			expectedError:   errors.New("package \"3scale-operator\" at index [0] is invalid: package specifies a VersionRange, while channel \"threescale-2.11\" at index [0] equally specifies one: package.VersionRange and channel.VersionRange are exclusive"),
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
			expectedError:   errors.New("Full: true cannot be mixed with versionRange"),
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
			expectedWarning: "package chocolate-factory-operator not found in catalog registry.redhat.io/redhat/redhat-operator-index:v4.16",
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
			expectedError:   errors.New("package \"3scale-operator\" channel \"threescale-mas\" has version range \">=77.77.77 <=77.77.77\" that results in an empty channel"),
			expectedWarning: ""},
	}

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(os.Stderr)
	}()

	log := clog.New("debug")
	handler := &catalogHandler{Log: log}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			dc, err := handler.getDeclarativeConfig(filepath.Join(common.TestFolder, "configs"))
			assert.NoError(t, err)
			res, err := filterCatalog(context.TODO(), *dc, testCase.cfg)
			if testCase.expectedError != nil && (err == nil || err.Error() != testCase.expectedError.Error()) {
				assert.EqualError(t, err, testCase.expectedError.Error())
			}
			if testCase.expectedError == nil {
				if err != nil {
					t.Fatalf("unexpected failure with error: %v", err)
					t.FailNow()
				}

				allPresent := true
				assert.Equal(t, len(testCase.expectedBundles), len(res.Bundles))

				for _, val := range testCase.expectedBundles {
					isPresent := func(b declcfg.Bundle) bool {
						return b.Name == val
					}
					present := slices.ContainsFunc(res.Bundles, isPresent)
					if !present {
						allPresent = false
						break
					}
				}

				assert.True(t, allPresent, "Not all expected bundles are present in the result")

			}
			// if testCase.expectedWarning != "" {
			// 	assert.Contains(t, buf.String(), testCase.expectedWarning)

			// }
		})
	}
}
