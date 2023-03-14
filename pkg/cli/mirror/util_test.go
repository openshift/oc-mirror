package mirror

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"

	oc "github.com/openshift/oc-mirror/pkg/cli/mirror/operatorcatalog"
)

func TestGetCatalogMetadataByPlatform(t *testing.T) {

	type test struct {
		name                  string // name of test
		layoutPath            layout.Path
		repo                  string   // the repo for the test
		topLevelTagOrDigest   string   // tag or digest for the test
		expectedDigests       []string // the expected digest values for the test
		expectedArchitectures int      // number of expected architectures
	}

	tests := []test{
		{
			name:                "single image",
			repo:                "foo/bar",
			layoutPath:          singleTestData,
			topLevelTagOrDigest: "latest",
			expectedDigests: []string{
				"sha256:a0aae779d7da2bb33c2d06f49510a50ec612b8cd1fb81f6ff4625bde497289a3",
			},
			expectedArchitectures: 1,
		},
		{
			name:                "manifest list image with tag",
			repo:                "foo/bar",
			layoutPath:          multiTestData,
			topLevelTagOrDigest: "latest",
			expectedDigests: []string{
				"sha256:8e7779499445140ccf598227b2211d973bf4fe1440262072633b9b11b5605d58",
				"sha256:6b2012214d36a888aef3812050cce5593de111181ba60a6ec4d68a3901367790",
				"sha256:0f443780f39cdfebb924e92f9fce6f05831e9bf6b6a7dbb0c09fe0086358a2ac",
			},
			expectedArchitectures: 3,
		},
		{
			name:                "manifest list image with digest",
			repo:                "foo/bar",
			layoutPath:          multiTestData,
			topLevelTagOrDigest: "sha256:5f6c1eeb1a6580d1ac4eb41587a6e1040f2a6ed5e8e84d5f332af1a7fd6227bd",
			expectedDigests: []string{
				"sha256:8e7779499445140ccf598227b2211d973bf4fe1440262072633b9b11b5605d58",
				"sha256:6b2012214d36a888aef3812050cce5593de111181ba60a6ec4d68a3901367790",
				"sha256:0f443780f39cdfebb924e92f9fce6f05831e9bf6b6a7dbb0c09fe0086358a2ac",
			},
			expectedArchitectures: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// create a http server
			server := httptest.NewServer(registry.New())
			defer server.Close()
			url, err := url.Parse(server.URL)
			require.NoError(t, err)

			// setup the source as an image index
			imgIdx, err := test.layoutPath.ImageIndex()
			require.NoError(t, err)

			// define the destination in terms of the http server taking into account the test
			// preference for tag or digest fetching
			rep, err := name.NewRepository(test.repo, name.WithDefaultRegistry(url.Host))
			require.NoError(t, err)
			var remoteRef name.Reference
			if strings.HasPrefix(test.topLevelTagOrDigest, "sha256") {
				remoteRef = rep.Digest(test.topLevelTagOrDigest)
			} else {
				remoteRef = rep.Tag(test.topLevelTagOrDigest)
			}

			// "push" into the fake server
			err = remote.WriteIndex(remoteRef, imgIdx)
			require.NoError(t, err)

			// run the thing we actually want to test
			digestsMap, err := getCatalogMetadataByPlatform(context.TODO(), remoteRef.String(), nil, false)
			require.NoError(t, err)
			require.Len(t, digestsMap, test.expectedArchitectures)

			// create expected / actual values in common format... lump all of the architectures together in this slice
			actualDigestsAsString := []string{}
			for platform, catalogMetadata := range digestsMap {
				// we should have a platform value that's not a "zero" value
				require.NotZero(t, platform)
				actualDigestsAsString = append(actualDigestsAsString, catalogMetadata.CatalogRef.Name())
			}

			expectedDigestsAsString := []string{}
			for _, expectedDigest := range test.expectedDigests {
				expectedDigestsAsString = append(expectedDigestsAsString, fmt.Sprintf("%s/%s@%s", url.Host, test.repo, expectedDigest))
			}

			// check expected / actual values
			require.ElementsMatch(t, expectedDigestsAsString, actualDigestsAsString)
		})
	}
}

func TestGetCatalogMetadataByPlatformFromOCILayout(t *testing.T) {

	type test struct {
		name                  string // name of test
		imageRef              string
		layoutPath            layout.Path
		expectedArchitectures int      // number of expected architectures
		expectedDigests       []string // the expected digest values for the test
	}

	tests := []test{
		{
			name:                  "single image",
			imageRef:              "index.docker.io/foo/bar",
			layoutPath:            singleTestData,
			expectedArchitectures: 1,
			expectedDigests: []string{
				"sha256:a0aae779d7da2bb33c2d06f49510a50ec612b8cd1fb81f6ff4625bde497289a3",
			},
		},
		{
			name:                  "manifest list",
			imageRef:              "index.docker.io/foo/bar",
			layoutPath:            multiTestData,
			expectedArchitectures: 3,
			expectedDigests: []string{
				"sha256:8e7779499445140ccf598227b2211d973bf4fe1440262072633b9b11b5605d58",
				"sha256:6b2012214d36a888aef3812050cce5593de111181ba60a6ec4d68a3901367790",
				"sha256:0f443780f39cdfebb924e92f9fce6f05831e9bf6b6a7dbb0c09fe0086358a2ac",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			digestsMap, err := getCatalogMetadataByPlatform(context.TODO(), test.imageRef, &test.layoutPath, false)
			require.NoError(t, err)
			require.Len(t, digestsMap, test.expectedArchitectures)
			// create expected / actual values in common format... lump all of the architectures together in this slice
			actualDigestsAsString := []string{}
			for platform, catalogMetadata := range digestsMap {
				// we should have a platform value that's not a "zero" value
				require.NotZero(t, platform)

				actualDigestsAsString = append(actualDigestsAsString, catalogMetadata.CatalogRef.Name())
			}

			expectedDigestsAsString := []string{}
			for _, expectedDigest := range test.expectedDigests {
				expectedDigestsAsString = append(expectedDigestsAsString, fmt.Sprintf("%s@%s", test.imageRef, expectedDigest))
			}

			// check expected / actual values (order does not matter)
			require.ElementsMatch(t, expectedDigestsAsString, actualDigestsAsString)
		})
	}
}

func TestGetDigestFromOCILayout(t *testing.T) {

	type test struct {
		name         string // name of test
		layoutPath   layout.Path
		platformIn   oc.OperatorCatalogPlatform
		expectedHash *v1.Hash
	}

	tests := []test{
		{
			name:       "manifest list - amd64",
			layoutPath: layout.Path("testdata/manifestlist/hello"),
			platformIn: oc.OperatorCatalogPlatform{
				Os:           "linux",
				Architecture: "amd64",
				IsIndex:      true,
			},
			expectedHash: &v1.Hash{
				Algorithm: "sha256",
				Hex:       "f54a58bc1aac5ea1a25d796ae155dc228b3f0e11d046ae276b39c4bf2f13d8c4",
			},
		},
		{
			name:       "manifest list - s390x",
			layoutPath: layout.Path("testdata/manifestlist/hello"),
			platformIn: oc.OperatorCatalogPlatform{
				Os:           "linux",
				Architecture: "s390x",
				IsIndex:      true,
			},
			expectedHash: &v1.Hash{
				Algorithm: "sha256",
				Hex:       "c7b6944911848ce39b44ed660d95fb54d69bbd531de724c7ce6fc9f743c0b861",
			},
		},
		{
			name:       "single arch",
			layoutPath: layout.Path("testdata/artifacts/rhop-ctlg-oci"),
			platformIn: oc.OperatorCatalogPlatform{
				IsIndex: false,
			},
			expectedHash: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hash, err := getDigestFromOCILayout(context.TODO(), test.layoutPath, test.platformIn)
			require.NoError(t, err)
			require.Equal(t, test.expectedHash, hash)
		})
	}
}
