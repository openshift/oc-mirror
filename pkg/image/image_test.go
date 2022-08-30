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
