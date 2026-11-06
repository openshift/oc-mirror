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
			res, err := handler.getRelatedImagesFromCatalog(testCase.cfg, copyImageSchemaMap)
			if testCase.expectedError != nil {
				assert.EqualError(t, err, testCase.expectedError.Error())
			} else {
				assert.NoError(t, err)
				assert.Len(t, testCase.expectedBundles, len(res), "the number of expected bundles is different from the one returned")
			}
		})
	}
}

// Images referenced by the bundle image tests.
const (
	testBarBundleImage = "quay.io/oc-mirror/oc-mirror-dev:bar-bundle-v0.1.0"
	testOperandImage   = "quay.io/oc-mirror/oc-mirror-dev@sha256:1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58"
)

// OCPBUGS-132865: the bundle image (olm.bundle.image) must be collected even when it is not
// repeated in the bundle's relatedImages.
func TestRelatedImagesFromCatalogBundleImage(t *testing.T) {
	const (
		barBundle   = testBarBundleImage
		bazBundle   = "quay.io/oc-mirror/oc-mirror-dev:baz-bundle-v1.0.0"
		operand     = testOperandImage
		barName     = "bar.v0.1.0"
		bazName     = "baz.v1.0.0"
		missingTag  = "quay.io/oc-mirror/oc-mirror-dev/bundle"
		ociBundle   = "oci:///tmp/bundle"
		badDigest   = "quay.io/oc-mirror/oc-mirror-dev@sha256:bad"
		dockerProto = consts.DockerProtocol
	)

	type testCase struct {
		caseName          string
		bundles           []declcfg.Bundle
		expectedImages    map[string][]v2alpha1.RelatedImage
		expectedOperators map[string]map[string]struct{}
		expectedBundles   map[string]map[string]string
		expectedError     string
	}

	// validBaz lists its bundle image in relatedImages, as opm renders it, so it is
	// collected the same way with or without the explicit bundle image handling.
	validBaz := declcfg.Bundle{Name: bazName, Package: "baz", Image: bazBundle, RelatedImages: []declcfg.RelatedImage{{Image: bazBundle}}}
	validBazImages := []v2alpha1.RelatedImage{{Image: bazBundle, Type: v2alpha1.TypeOperatorBundle}}

	testCases := []testCase{
		{
			caseName: "bundle image missing from related images is collected as the bundle",
			bundles: []declcfg.Bundle{{
				Name: barName, Package: "bar", Image: barBundle,
				RelatedImages: []declcfg.RelatedImage{{Name: "operator", Image: operand}},
			}},
			expectedImages: map[string][]v2alpha1.RelatedImage{barName: {
				{Image: barBundle, Type: v2alpha1.TypeOperatorBundle},
				{Name: "operator", Image: operand, Type: v2alpha1.TypeOperatorRelatedImage},
			}},
			expectedOperators: map[string]map[string]struct{}{
				dockerProto + barBundle: {"bar": {}},
				dockerProto + operand:   {"bar": {}},
			},
			expectedBundles: map[string]map[string]string{
				dockerProto + barBundle: {barBundle: barName},
				dockerProto + operand:   {barBundle: barName},
			},
		},
		{
			caseName: "bundle image is collected when related images are empty",
			bundles:  []declcfg.Bundle{{Name: bazName, Package: "baz", Image: bazBundle, RelatedImages: []declcfg.RelatedImage{}}},
			expectedImages: map[string][]v2alpha1.RelatedImage{bazName: {
				{Image: bazBundle, Type: v2alpha1.TypeOperatorBundle},
			}},
			expectedOperators: map[string]map[string]struct{}{dockerProto + bazBundle: {"baz": {}}},
			expectedBundles:   map[string]map[string]string{dockerProto + bazBundle: {bazBundle: bazName}},
		},
		{
			caseName:          "bundle image is collected when related images are omitted",
			bundles:           []declcfg.Bundle{{Name: bazName, Package: "baz", Image: bazBundle}},
			expectedImages:    map[string][]v2alpha1.RelatedImage{bazName: validBazImages},
			expectedOperators: map[string]map[string]struct{}{dockerProto + bazBundle: {"baz": {}}},
			expectedBundles:   map[string]map[string]string{dockerProto + bazBundle: {bazBundle: bazName}},
		},
		{
			caseName: "bundle image listed in related images keeps that entry",
			bundles: []declcfg.Bundle{{
				Name: barName, Package: "bar", Image: barBundle,
				RelatedImages: []declcfg.RelatedImage{{Name: "bundle", Image: barBundle}, {Name: "operator", Image: operand}},
			}},
			expectedImages: map[string][]v2alpha1.RelatedImage{barName: {
				{Name: "bundle", Image: barBundle, Type: v2alpha1.TypeOperatorBundle},
				{Name: "operator", Image: operand, Type: v2alpha1.TypeOperatorRelatedImage},
			}},
			expectedOperators: map[string]map[string]struct{}{
				dockerProto + barBundle: {"bar": {}},
				dockerProto + operand:   {"bar": {}},
			},
			expectedBundles: map[string]map[string]string{
				dockerProto + barBundle: {barBundle: barName},
				dockerProto + operand:   {barBundle: barName},
			},
		},
		{
			caseName: "bundle image listed twice in related images is collected once",
			bundles: []declcfg.Bundle{{
				Name: barName, Package: "bar", Image: barBundle,
				RelatedImages: []declcfg.RelatedImage{
					{Name: "first", Image: barBundle},
					{Name: "operator", Image: operand},
					{Name: "second", Image: barBundle},
				},
			}},
			expectedImages: map[string][]v2alpha1.RelatedImage{barName: {
				{Name: "first", Image: barBundle, Type: v2alpha1.TypeOperatorBundle},
				{Name: "operator", Image: operand, Type: v2alpha1.TypeOperatorRelatedImage},
			}},
			expectedOperators: map[string]map[string]struct{}{
				dockerProto + barBundle: {"bar": {}},
				dockerProto + operand:   {"bar": {}},
			},
			expectedBundles: map[string]map[string]string{
				dockerProto + barBundle: {barBundle: barName},
				dockerProto + operand:   {barBundle: barName},
			},
		},
		{
			caseName: "without a bundle image, related images are collected as related images",
			bundles: []declcfg.Bundle{{
				Name: barName, Package: "bar",
				RelatedImages: []declcfg.RelatedImage{{Name: "bundle", Image: barBundle}, {Name: "operator", Image: operand}},
			}},
			expectedImages: map[string][]v2alpha1.RelatedImage{barName: {
				{Name: "bundle", Image: barBundle, Type: v2alpha1.TypeOperatorRelatedImage},
				{Name: "operator", Image: operand, Type: v2alpha1.TypeOperatorRelatedImage},
			}},
			expectedOperators: map[string]map[string]struct{}{
				dockerProto + barBundle: {"bar": {}},
				dockerProto + operand:   {"bar": {}},
			},
			expectedBundles: map[string]map[string]string{
				dockerProto + barBundle: {"": barName},
				dockerProto + operand:   {"": barName},
			},
		},
		{
			caseName:          "without a bundle image or related images, nothing is collected for the bundle",
			bundles:           []declcfg.Bundle{{Name: barName, Package: "bar"}},
			expectedImages:    map[string][]v2alpha1.RelatedImage{barName: nil},
			expectedOperators: map[string]map[string]struct{}{},
			expectedBundles:   map[string]map[string]string{},
		},
		{
			caseName: "bundles of different packages sharing a related image are all associated with it",
			bundles: []declcfg.Bundle{
				{Name: barName, Package: "bar", Image: barBundle, RelatedImages: []declcfg.RelatedImage{{Name: "operator", Image: operand}}},
				{Name: bazName, Package: "baz", Image: bazBundle, RelatedImages: []declcfg.RelatedImage{{Name: "operator", Image: operand}}},
			},
			expectedImages: map[string][]v2alpha1.RelatedImage{
				barName: {
					{Image: barBundle, Type: v2alpha1.TypeOperatorBundle},
					{Name: "operator", Image: operand, Type: v2alpha1.TypeOperatorRelatedImage},
				},
				bazName: {
					{Image: bazBundle, Type: v2alpha1.TypeOperatorBundle},
					{Name: "operator", Image: operand, Type: v2alpha1.TypeOperatorRelatedImage},
				},
			},
			expectedOperators: map[string]map[string]struct{}{
				dockerProto + barBundle: {"bar": {}},
				dockerProto + bazBundle: {"baz": {}},
				dockerProto + operand:   {"bar": {}, "baz": {}},
			},
			expectedBundles: map[string]map[string]string{
				dockerProto + barBundle: {barBundle: barName},
				dockerProto + bazBundle: {bazBundle: bazName},
				dockerProto + operand:   {barBundle: barName, bazBundle: bazName},
			},
		},
		{
			caseName: "bundle image without tag or digest skips the bundle",
			bundles: []declcfg.Bundle{
				{Name: barName, Package: "bar", Image: missingTag, RelatedImages: []declcfg.RelatedImage{{Name: "operator", Image: operand}}},
				validBaz,
			},
			expectedImages:    map[string][]v2alpha1.RelatedImage{bazName: validBazImages},
			expectedOperators: map[string]map[string]struct{}{dockerProto + bazBundle: {"baz": {}}},
			expectedBundles:   map[string]map[string]string{dockerProto + bazBundle: {bazBundle: bazName}},
			expectedError:     "error parsing image " + missingTag + ": " + missingTag + " unable to parse image correctly : tag and digest are empty",
		},
		{
			caseName: "bundle image with the oci transport skips the bundle",
			bundles: []declcfg.Bundle{
				{Name: barName, Package: "bar", Image: ociBundle, RelatedImages: []declcfg.RelatedImage{{Name: "operator", Image: operand}}},
				validBaz,
			},
			expectedImages:    map[string][]v2alpha1.RelatedImage{bazName: validBazImages},
			expectedOperators: map[string]map[string]struct{}{dockerProto + bazBundle: {"baz": {}}},
			expectedBundles:   map[string]map[string]string{dockerProto + bazBundle: {bazBundle: bazName}},
			expectedError:     "invalid image: " + ociBundle + " 'oci' is not supported in operator catalogs",
		},
		{
			caseName: "bundle image with an invalid digest skips the bundle",
			bundles: []declcfg.Bundle{
				{Name: barName, Package: "bar", Image: badDigest},
				validBaz,
			},
			expectedImages:    map[string][]v2alpha1.RelatedImage{bazName: validBazImages},
			expectedOperators: map[string]map[string]struct{}{dockerProto + bazBundle: {"baz": {}}},
			expectedBundles:   map[string]map[string]string{dockerProto + bazBundle: {bazBundle: bazName}},
			expectedError:     "error parsing image " + badDigest + ": " + badDigest + " unable to parse image correctly : invalid digest",
		},
		{
			caseName: "invalid bundle image also listed in related images skips the bundle",
			bundles: []declcfg.Bundle{
				{Name: barName, Package: "bar", Image: missingTag, RelatedImages: []declcfg.RelatedImage{{Image: missingTag}}},
				validBaz,
			},
			expectedImages:    map[string][]v2alpha1.RelatedImage{bazName: validBazImages},
			expectedOperators: map[string]map[string]struct{}{dockerProto + bazBundle: {"baz": {}}},
			expectedBundles:   map[string]map[string]string{dockerProto + bazBundle: {bazBundle: bazName}},
			expectedError:     "error parsing image " + missingTag + ": " + missingTag + " unable to parse image correctly : tag and digest are empty",
		},
		{
			caseName: "invalid bundle image is reported before invalid related images",
			bundles: []declcfg.Bundle{
				{Name: barName, Package: "bar", Image: ociBundle, RelatedImages: []declcfg.RelatedImage{{Name: "operator", Image: missingTag}}},
				validBaz,
			},
			expectedImages:    map[string][]v2alpha1.RelatedImage{bazName: validBazImages},
			expectedOperators: map[string]map[string]struct{}{dockerProto + bazBundle: {"baz": {}}},
			expectedBundles:   map[string]map[string]string{dockerProto + bazBundle: {bazBundle: bazName}},
			expectedError:     "invalid image: " + ociBundle + " 'oci' is not supported in operator catalogs",
		},
	}

	handler := &CatalogHandler{Log: clog.New("error")}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: make(map[string]map[string]struct{}), BundlesByImage: make(map[string]map[string]string)}
			res, err := handler.getRelatedImagesFromCatalog(&declcfg.DeclarativeConfig{Bundles: testCase.bundles}, copyImageSchemaMap)
			if testCase.expectedError != "" {
				assert.EqualError(t, err, testCase.expectedError)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, testCase.expectedImages, res)
			assert.Equal(t, testCase.expectedOperators, copyImageSchemaMap.OperatorsByImage)
			assert.Equal(t, testCase.expectedBundles, copyImageSchemaMap.BundlesByImage)
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
