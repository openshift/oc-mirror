package manifest

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/stretchr/testify/assert"
)

func TestGetAllManifests(t *testing.T) {

	log := clog.New("debug")

	// these tests should cover over 80%
	t.Run("Testing GetImageIndex : should pass", func(t *testing.T) {
		manifest := &Manifest{Log: log}
		res, err := manifest.GetImageIndex("../../tests/")
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test  %v ", res)
	})

	t.Run("Testing GetImageManifest : should pass", func(t *testing.T) {
		manifest := &Manifest{Log: log}
		res, err := manifest.GetImageManifest("../../tests/image-manifest.json")
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test  %v ", res)
	})

	t.Run("Testing GetOperatorConfig : should pass", func(t *testing.T) {
		manifest := &Manifest{Log: log}
		res, err := manifest.GetOperatorConfig("../../tests/operator-config.json")
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test  %v ", res)
	})

	t.Run("Testing GetReleaseSchema : should pass", func(t *testing.T) {
		manifest := &Manifest{Log: log}
		res, err := manifest.GetReleaseSchema("../../tests/release-schema.json")
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
		log.Debug("completed test  %v ", res)
	})
}

func TestGetRelatedImagesFromCatalog(t *testing.T) {
	type testCase struct {
		caseName        string
		cfg             v1alpha2.Operator
		expectedBundles []string
		expectedError   error
	}

	testCases := []testCase{
		{
			caseName: "only catalog (no filtering) - only the head of the default channel - should pass",
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{},
			},
			expectedBundles: []string{
				"3scale-operator.v0.11.0-mas",
				"devworkspace-operator.v0.19.1-0.1682321189.p",
				"jaeger-operator.v1.51.0-1",
			},
		},
		{
			caseName: "only catalog with full: true - all bundles of all channels of the specified catalog - should pass",
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{},
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
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
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
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
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
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "3scale-operator",
						},
						{
							Name:          "devworkspace-operator",
							IncludeBundle: v1alpha2.IncludeBundle{MinVersion: "0.18.1"},
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
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "3scale-operator",
						},
						{
							Name:          "devworkspace-operator",
							IncludeBundle: v1alpha2.IncludeBundle{MaxVersion: "0.18.1"},
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
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "3scale-operator",
						},
						{
							Name: "devworkspace-operator",
							IncludeBundle: v1alpha2.IncludeBundle{
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
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "3scale-operator",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name:          "threescale-2.11",
									IncludeBundle: v1alpha2.IncludeBundle{MinVersion: "0.8.3"},
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
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "3scale-operator",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name: "threescale-2.11",
								},
							},
						},
						{
							Name: "devworkspace-operator",
							Channels: []v1alpha2.IncludeChannel{
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
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "3scale-operator",
							Channels: []v1alpha2.IncludeChannel{
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
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "3scale-operator",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name:          "threescale-2.11",
									IncludeBundle: v1alpha2.IncludeBundle{MaxVersion: "0.8.2"},
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
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "3scale-operator",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name:          "threescale-2.11",
									IncludeBundle: v1alpha2.IncludeBundle{MinVersion: "0.8.1", MaxVersion: "0.8.3"},
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
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "3scale-operator",
							Channels: []v1alpha2.IncludeChannel{
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
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "3scale-operator",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name: "threescale-2.11",
								},
							},
							IncludeBundle: v1alpha2.IncludeBundle{
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
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "3scale-operator",
							IncludeBundle: v1alpha2.IncludeBundle{
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
	}

	log := clog.New("debug")
	manifest := &Manifest{Log: log}

	operatorCatalog, err := manifest.GetCatalog(filepath.Join("../../tests/", "configs"))
	assert.NoError(t, err)

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {

			var res map[string][]v1alpha3.RelatedImage
			var err error

			res, err = manifest.GetRelatedImagesFromCatalog(operatorCatalog, testCase.cfg)

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

			if testCase.expectedError != nil && err.Error() != testCase.expectedError.Error() {
				assert.EqualError(t, err, testCase.expectedError.Error())
			}

			log.Debug("completed test  %v ", res)
		})
	}
}

func TestTypesOnRelatedImages(t *testing.T) {

	type testCase struct {
		caseName string
		cfg      v1alpha2.Operator
	}

	testCases := []testCase{
		{
			caseName: "only catalog (no filtering) - only the head of the default channel - should pass",
			cfg: v1alpha2.Operator{
				IncludeConfig: v1alpha2.IncludeConfig{},
			},
		},
	}

	log := clog.New("debug")

	manifest := &Manifest{Log: log}

	operatorCatalog, err := manifest.GetCatalog(filepath.Join("../../tests/", "configs"))
	assert.NoError(t, err)

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {

			var bundles map[string][]v1alpha3.RelatedImage
			var err error

			bundles, err = manifest.GetRelatedImagesFromCatalog(operatorCatalog, testCase.cfg)

			assert.NoError(t, err)

			for _, relatedImages := range bundles {
				for _, ri := range relatedImages {
					assert.NotEqual(t, v1alpha2.TypeInvalid, ri.Type, "Type should be catalog")
				}
			}

			log.Debug("completed test  %v ", bundles)
		})
	}

}

func TestExtractOCILayers(t *testing.T) {

	log := clog.New("debug")
	t.Run("Testing ExtractOCILayers : should pass", func(t *testing.T) {
		oci := &v1alpha3.OCISchema{
			SchemaVersion: 2,
			Manifests: []v1alpha3.OCIManifest{
				{
					MediaType: "application/vnd.oci.image.manifest.v1+json",
					Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
					Size:      567,
				},
			},
			Config: v1alpha3.OCIManifest{
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
				Size:      567,
			},
			Layers: []v1alpha3.OCIManifest{
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    "sha256:d8190195889efb5333eeec18af9b6c82313edd4db62989bd3a357caca4f13f0e",
					Size:      1438,
				},
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    "sha256:5b2ca04f694b70c8b41f1c2a40b7e95643181a1d037b115149ecc243324c513d",
					Size:      955593,
				},
			},
		}
		manifest := &Manifest{Log: log}
		// this should do a nop (directory exists)
		err := manifest.ExtractLayersOCI("../../tests/test-untar/blobs/sha256", "../../tests/test-untar", "release-manifests/", oci)
		if err != nil {
			t.Fatal("should not fail")
		}
		_, err = os.Stat("../../tests/hold-test-untar/release-manifests/")
		if err == nil {
			t.Fatalf("should fail")
		}

		err = manifest.ExtractLayersOCI("../../tests/test-untar/blobs/sha256", "../../tests/hold-test-untar", "release-manifests/", oci)
		if err != nil {
			t.Fatalf("should not fail")
		}
		defer os.RemoveAll("../../tests/hold-test-untar")
		_, err = os.Stat("../../tests/hold-test-untar/release-manifests/")
		if err != nil {
			t.Fatalf("should not fail")
		}
	})
}
