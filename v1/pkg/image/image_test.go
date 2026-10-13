package image

import (
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
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
					Registry:  "oci://home/user",
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
			registry, org, repo, tag, sha := v1alpha2.ParseImageReference(c.imageName)
			require.Equal(t, c.expReg, registry)
			require.Equal(t, c.expOrg, org)
			require.Equal(t, c.expRepo, repo)
			require.Equal(t, c.expDigest, sha)
			require.Equal(t, c.expTag, tag)
		})
	}
}
func TestV1A2ParseImageReferenceOCIRefs(t *testing.T) {
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
			desc:      "no path at all",
			imageName: "oci:",
			expReg:    "",
			expOrg:    "",
			expRepo:   "",
			expDigest: "",
			expTag:    "",
		},
		{
			desc:      "single dir at relative path",
			imageName: "oci://foo",
			expReg:    "",
			expOrg:    "",
			expRepo:   "foo",
			expDigest: "",
			expTag:    "",
		},
		{
			desc:      "no dir relative path",
			imageName: "oci://",
			expReg:    "",
			expOrg:    "",
			expRepo:   "",
			expDigest: "",
			expTag:    "",
		},
		{
			desc:      "two levels deep at relative path",
			imageName: "oci://foo/bar",
			expReg:    "foo",
			expOrg:    "",
			expRepo:   "bar",
			expDigest: "",
			expTag:    "",
		},
		{
			desc:      "three levels deep at relative path",
			imageName: "oci://foo/bar/baz",
			expReg:    "foo",
			expOrg:    "bar",
			expRepo:   "baz",
			expDigest: "",
			expTag:    "",
		},
		{
			desc:      "three levels deep at relative path",
			imageName: "oci://foo/bar/baz/blah",
			expReg:    "foo",
			expOrg:    "bar/baz",
			expRepo:   "blah",
			expDigest: "",
			expTag:    "",
		},
		{
			desc:      "no dir at root",
			imageName: "oci:///",
			expReg:    "",
			expOrg:    "",
			expRepo:   "",
			expDigest: "",
			expTag:    "",
		},
		{
			desc:      "single dir at root",
			imageName: "oci:///foo",
			expReg:    "",
			expOrg:    "",
			expRepo:   "foo",
			expDigest: "",
			expTag:    "",
		},
		{
			desc:      "two levels deep at root",
			imageName: "oci:///foo/bar",
			expReg:    "foo",
			expOrg:    "",
			expRepo:   "bar",
			expDigest: "",
			expTag:    "",
		},
		{
			desc:      "three levels deep at root",
			imageName: "oci:///foo/bar/baz",
			expReg:    "foo",
			expOrg:    "bar",
			expRepo:   "baz",
			expDigest: "",
			expTag:    "",
		},
		{
			desc:      "three levels deep at root",
			imageName: "oci:///foo/bar/baz/blah",
			expReg:    "foo",
			expOrg:    "bar/baz",
			expRepo:   "blah",
			expDigest: "",
			expTag:    "",
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			registry, org, repo, tag, sha := v1alpha2.ParseImageReference(c.imageName)
			require.Equal(t, c.expReg, registry)
			require.Equal(t, c.expOrg, org)
			require.Equal(t, c.expRepo, repo)
			require.Equal(t, c.expDigest, sha)
			require.Equal(t, c.expTag, tag)
		})
	}
}
func TestGetFirstDigestFromPath(t *testing.T) {
	type spec struct {
		desc           string
		inRef          string
		errorFunc      require.ErrorAssertionFunc
		expectedDigest *v1.Hash
	}
	makeRef := func(path string) string {
		absPath, err := filepath.Abs(path)
		require.NoError(t, err)
		return v1alpha2.OCITransportPrefix + "//" + absPath
	}
	cases := []spec{
		{
			desc:           "single arch case one",
			inRef:          makeRef("../cli/mirror/testdata/artifacts/rhop-ctlg-oci"),
			errorFunc:      require.NoError,
			expectedDigest: &v1.Hash{Algorithm: "sha256", Hex: "3986c6e039692ada9b5fa79ce51ce49bf6b24bc3af91d96e6c9d3d72f8077401"},
		},
		{
			desc:           "single arch case two",
			inRef:          makeRef("../cli/mirror/testdata/single/testonly/layout"),
			errorFunc:      require.NoError,
			expectedDigest: &v1.Hash{Algorithm: "sha256", Hex: "a0aae779d7da2bb33c2d06f49510a50ec612b8cd1fb81f6ff4625bde497289a3"},
		},
		{
			desc:           "multi arch case one",
			inRef:          makeRef("../cli/mirror/testdata/manifestlist/hello"),
			errorFunc:      require.NoError,
			expectedDigest: &v1.Hash{Algorithm: "sha256", Hex: "d0c9de6b9869c144aca831898c562d01169b740e50a73b8893cdd05ab94c64b7"},
		},
		{
			desc:           "multi arch case two",
			inRef:          makeRef("../cli/mirror/testdata/manifestlist/testonly/layout"),
			errorFunc:      require.NoError,
			expectedDigest: &v1.Hash{Algorithm: "sha256", Hex: "f8859996f481d0332f486fca612ac64f4fc31b94d03f45086ed3e1aa3df3f5f7"},
		},
		{
			desc:           "multi arch case three - manifest at root",
			inRef:          makeRef("../cli/mirror/testdata/manifestlist/manifestlist-at-root/layout"),
			errorFunc:      require.NoError,
			expectedDigest: &v1.Hash{Algorithm: "sha256", Hex: "f8859996f481d0332f486fca612ac64f4fc31b94d03f45086ed3e1aa3df3f5f7"},
		},
		{
			desc:           "nonexistent directory",
			inRef:          makeRef("foo"),
			errorFunc:      require.Error,
			expectedDigest: nil,
		},
		{
			desc:           "index is unmarshallable fails",
			inRef:          makeRef("../cli/mirror/testdata/artifacts/rhop-rotten-manifest"),
			errorFunc:      require.Error,
			expectedDigest: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			actualDigest, err := getFirstDigestFromPath(c.inRef)
			require.Equal(t, c.expectedDigest, actualDigest)
			c.errorFunc(t, err)
		})
	}
}

func TestString(t *testing.T) {
	type spec struct {
		desc           string
		img            TypedImageReference
		expectedString string
	}
	cases := []spec{
		{
			desc: "docker remote image with tag",
			img: TypedImageReference{
				Type: imagesource.DestinationRegistry,
				Ref: reference.DockerImageReference{
					Registry:  "quay.io",
					Namespace: "okd-project",
					Name:      "okd-scos",
					Tag:       "latest",
				},
			},
			expectedString: "quay.io/okd-project/okd-scos:latest",
		},
		{
			desc: "docker remote image with tag and digest",
			img: TypedImageReference{
				Type: imagesource.DestinationRegistry,
				Ref: reference.DockerImageReference{
					Registry:  "quay.io",
					Namespace: "okd-project",
					Name:      "okd-scos",
					Tag:       "latest",
					ID:        "sha256:2881fc4ddeea9a1d244c37c0216c7d6c79a572757bce007520523c9120e66429",
				},
			},
			expectedString: "quay.io/okd-project/okd-scos@sha256:2881fc4ddeea9a1d244c37c0216c7d6c79a572757bce007520523c9120e66429",
		},
		{
			desc: "oci local image",
			img: TypedImageReference{
				Type: DestinationOCI,
				Ref: reference.DockerImageReference{
					Registry:  "",
					Namespace: "/tmp/oci",
					Name:      "redhat-operator-index",
					Tag:       "",
					ID:        "sha256:2881fc4ddeea9a1d244c37c0216c7d6c79a572757bce007520523c9120e66429",
				},
			},
			expectedString: "oci:///tmp/oci/redhat-operator-index@sha256:2881fc4ddeea9a1d244c37c0216c7d6c79a572757bce007520523c9120e66429",
		},
		{
			desc: "on disk (file) image",
			img: TypedImageReference{
				Type: imagesource.DestinationFile,
				Ref: reference.DockerImageReference{
					Registry:  "",
					Namespace: "openshift4",
					Name:      "ose-kube-rbac-proxy",
					Tag:       "",
					ID:        "sha256:2881fc4ddeea9a1d244c37c0216c7d6c79a572757bce007520523c9120e66429",
				},
			},
			expectedString: "file://openshift4/ose-kube-rbac-proxy@sha256:2881fc4ddeea9a1d244c37c0216c7d6c79a572757bce007520523c9120e66429",
		},
		{
			desc: "s3 remote image",
			img: TypedImageReference{
				Type: imagesource.DestinationS3,
				Ref: reference.DockerImageReference{
					Registry:  "mybucket",
					Namespace: "tmp/oci",
					Name:      "redhat-operator-index",
					Tag:       "",
					ID:        "sha256:2881fc4ddeea9a1d244c37c0216c7d6c79a572757bce007520523c9120e66429",
				},
			},
			expectedString: "s3://mybucket/tmp/oci/redhat-operator-index@sha256:2881fc4ddeea9a1d244c37c0216c7d6c79a572757bce007520523c9120e66429",
		},
		{
			desc: "remote image on docker hub",
			img: TypedImageReference{
				Type: imagesource.DestinationRegistry,
				Ref: reference.DockerImageReference{
					Registry:  "docker.io",
					Namespace: "",
					Name:      "alpine",
					Tag:       "",
					ID:        "sha256:2881fc4ddeea9a1d244c37c0216c7d6c79a572757bce007520523c9120e66429",
				},
			},
			expectedString: "docker.io/library/alpine@sha256:2881fc4ddeea9a1d244c37c0216c7d6c79a572757bce007520523c9120e66429",
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			actualString := c.img.String()
			require.Equal(t, c.expectedString, actualString)
		})
	}
}
