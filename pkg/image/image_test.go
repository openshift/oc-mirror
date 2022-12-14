package image

import (
	"testing"

	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/stretchr/testify/require"
)

func TestParseReference(t *testing.T) {
	type spec struct {
		desc      string
		inRef     string
		expImgRef imagesource.TypedImageReference
		err       string
	}

	cases := []spec{
		{
			desc:  "remote catalog OK",
			inRef: "registry.redhat.io/redhat/redhat-operator-index:v4.11",
			expImgRef: imagesource.TypedImageReference{
				Type: imagesource.DestinationRegistry,
				Ref: reference.DockerImageReference{
					Registry:  "registry.redhat.io",
					Namespace: "redhat",
					Tag:       "v4.11",
					Name:      "redhat-operator-index",
					ID:        "",
				},
			},
		},
		{
			desc:  "local file catalog OK",
			inRef: "file:///home/user/catalogs/redhat-operator-index:v4.11",
			expImgRef: imagesource.TypedImageReference{
				Type: imagesource.DestinationFile,
				Ref: reference.DockerImageReference{
					Registry:  "home/user",
					Namespace: "catalogs",
					Tag:       "v4.11",
					Name:      "redhat-operator-index",
					ID:        "",
				},
			},
		},
		{
			desc:  "oci local catalog OK",
			inRef: "oci:///home/user/catalogs/redhat-operator-index:v4.11",
			expImgRef: imagesource.TypedImageReference{
				Type: DestinationOCI,
				Ref: reference.DockerImageReference{
					Registry:  "home/user",
					Namespace: "catalogs",
					Tag:       "v4.11",
					Name:      "redhat-operator-index",
					ID:        "",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			imgRef, err := ParseReference(c.inRef)
			if c.err != "" {
				require.EqualError(t, err, c.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expImgRef.String(), imgRef.String())
			}
		})
	}
}

func TestParseImageName(t *testing.T) {
	type spec struct {
		desc      string
		imageName string
		expReg    string
		expOrg    string
		expRepo   string
		expTag    string
		expDigest string
	}
	cases := []spec{
		{
			desc:      "remote image with tag",
			imageName: "quay.io/redhatgov/oc-mirror-dev:foo-bundle-v0.3.1",
			expReg:    "quay.io",
			expOrg:    "redhatgov",
			expRepo:   "oc-mirror-dev",
			expDigest: "",
			expTag:    "foo-bundle-v0.3.1",
		},
		{
			desc:      "remote image with digest",
			imageName: "quay.io/redhatgov/oc-mirror-dev@sha256:7e1e74b87a503e95db5203334917856f61aece90a72e8d53a9fd903344eb78a5",
			expReg:    "quay.io",
			expOrg:    "redhatgov",
			expRepo:   "oc-mirror-dev",
			expDigest: "7e1e74b87a503e95db5203334917856f61aece90a72e8d53a9fd903344eb78a5",
			expTag:    "",
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			registry, org, repo, tag, sha := ParseImageReference(c.imageName)
			require.Equal(t, c.expReg, registry)
			require.Equal(t, c.expOrg, org)
			require.Equal(t, c.expRepo, repo)
			require.Equal(t, c.expDigest, sha)
			require.Equal(t, c.expTag, tag)
		})
	}
}
