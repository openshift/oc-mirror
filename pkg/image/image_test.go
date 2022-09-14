package image

import (
	"context"
	"fmt"
	"testing"

	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/stretchr/testify/require"
)

// TODO: add preparation step that saves a catalog locally before testing
// see maybe contents of pkg/image/testdata
func TestGetManifestFromIndex(t *testing.T) {
	type spec struct {
		desc  string
		inRef string
		err   string
	}

	cases := []spec{
		{
			desc:  "Nominal case",
			inRef: "oci:/home/skhoury/go/src/github.com/openshift/oc-mirror/rhop-ctlg-oci",
			err:   "",
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			manifest, err := GetConfigDirFromOCICatalog(context.TODO(), c.inRef)
			if c.err != "" {
				require.EqualError(t, err, c.err)
			} else {
				require.NoError(t, err)
				fmt.Printf("manifest: %v\n", manifest)
			}
		})
	}
}

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

func TestCopyFromRemote(t *testing.T) {
	type spec struct {
		desc string
		src  string
		dst  string
		err  string
	}

	cases := []spec{
		{
			desc: "Nominal case",
			src:  "docker://registry.redhat.io/noo/node-observability-operator-bundle-rhel8@sha256:25b8e1c8ed635364d4dcba7814ad504570b1c6053d287ab7e26c8d6a97ae3f6a",
			// src: "registry.redhat.io/noo/node-observability-operator-bundle-rhel8@sha256:25b8e1c8ed635364d4dcba7814ad504570b1c6053d287ab7e26c8d6a97ae3f6a",
			dst: "oci:/home/skhoury/go/src/github.com/openshift/oc-mirror/cmd/oc-mirror/oc-mirror-workspace/src/catalogs/home/skhoury/go/src/github.com/openshift/oc-mirror/rhop-ctlg-oci/index/noo/node-observability-operator-bundle-rhel8/sha256:25b8e1c8ed635364d4dcba7814ad504570b1c6053d287ab7e26c8d6a97ae3f6a",
			err: "",
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			err := CopyFromRemote(c.src, c.dst)
			if c.err != "" {
				require.EqualError(t, err, c.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
