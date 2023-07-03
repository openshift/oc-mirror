package mirror

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/image"
)

func TestMirrorComplete(t *testing.T) {
	type spec struct {
		name     string
		args     []string
		opts     *MirrorOptions
		expOpts  *MirrorOptions
		expError string
	}

	cases := []spec{
		{
			name: "Valid/FileDest",
			args: []string{"file://foo"},
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expOpts: &MirrorOptions{
				OutputDir: "foo",
				RootOptions: &cli.RootOptions{
					Dir: "foo/bar",
				},
			},
		},
		{
			name: "Valid/FileDestRel",
			args: []string{"file://./foo"},
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expOpts: &MirrorOptions{
				OutputDir: "foo",
				RootOptions: &cli.RootOptions{
					Dir: "foo/bar",
				},
			},
		},
		{
			name: "Valid/EmptyFileDest",
			args: []string{"file://"},
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expOpts: &MirrorOptions{
				OutputDir: ".",
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
		},
		{
			name: "Valid/RegDest",
			args: []string{"docker://reg.com"},
			opts: &MirrorOptions{MaxNestedPaths: 0},
			expOpts: &MirrorOptions{
				ToMirror:       "reg.com",
				MaxNestedPaths: 0,
			},
		},
		{
			name: "Valid/LocalhostRegDest",
			args: []string{"docker://localhost"},
			opts: &MirrorOptions{MaxNestedPaths: 0},
			expOpts: &MirrorOptions{
				ToMirror:       "localhost",
				MaxNestedPaths: 0,
			},
		},
		{
			name: "Valid/FqdnRegPortDest",
			args: []string{"docker://reg.com:5000"},
			opts: &MirrorOptions{MaxNestedPaths: 0},
			expOpts: &MirrorOptions{
				ToMirror:       "reg.com:5000",
				MaxNestedPaths: 0,
			},
		},
		{
			name: "Valid/LocalhostRegPortDest",
			args: []string{"docker://localhost:5000"},
			opts: &MirrorOptions{MaxNestedPaths: 0},
			expOpts: &MirrorOptions{
				ToMirror:       "localhost:5000",
				MaxNestedPaths: 0,
			},
		},
		{
			name: "Valid/RegNamespace",
			args: []string{"docker://reg.com/foo/bar"},
			opts: &MirrorOptions{MaxNestedPaths: 0},
			expOpts: &MirrorOptions{
				ToMirror:       "reg.com",
				UserNamespace:  "foo/bar",
				MaxNestedPaths: 0,
			},
		},
		{
			name: "Valid/LocalhostRegNamespace",
			args: []string{"docker://localhost/foo/bar"},
			opts: &MirrorOptions{MaxNestedPaths: 0},
			expOpts: &MirrorOptions{
				ToMirror:       "localhost",
				UserNamespace:  "foo/bar",
				MaxNestedPaths: 0,
			},
		},
		{
			name: "Valid/NonFqdnRegPortNamespace",
			args: []string{"docker://reg:5000/foo"},
			opts: &MirrorOptions{MaxNestedPaths: 0},
			expOpts: &MirrorOptions{
				ToMirror:       "reg:5000",
				UserNamespace:  "foo",
				MaxNestedPaths: 0,
			},
		},
		{
			name: "Valid/NonFqdnRegPortNamespaceName",
			args: []string{"docker://reg:5000/foo/bar"},
			opts: &MirrorOptions{MaxNestedPaths: 0},
			expOpts: &MirrorOptions{
				ToMirror:       "reg:5000",
				UserNamespace:  "foo/bar",
				MaxNestedPaths: 0,
			},
		},
		{
			name: "Valid/SetFilterOps",
			args: []string{"file://foo"},
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expOpts: &MirrorOptions{
				OutputDir: "foo",
				RootOptions: &cli.RootOptions{
					Dir: "foo/bar",
				},
			},
		},
		{
			name:     "Invalid/NonFqdnRegDest",
			args:     []string{"docker://reg"}, // warning message for parsing
			opts:     &MirrorOptions{},
			expError: "destination registry must consist of registry host and namespace(s) only, and must not include an image tag or ID",
		},
		{
			name:     "Invalid/NonFqdnRegPortDest",
			args:     []string{"docker://reg:5000"}, // warning message for parsing
			opts:     &MirrorOptions{},
			expError: "destination registry must consist of registry host and namespace(s) only, and must not include an image tag or ID",
		},
		{
			name:     "Invalid/NonFqdnRegNamespaceName",
			args:     []string{"docker://reg/foo/bar"}, // warning message for parsing
			opts:     &MirrorOptions{},
			expError: "destination registry must consist of registry host and namespace(s) only, and must not include an image tag or ID",
		},
		{
			name:     "Invalid/NonFqdnRegNamespace",
			args:     []string{"docker://reg/foo"}, // warning message for parsing
			opts:     &MirrorOptions{},
			expError: "destination registry must consist of registry host and namespace(s) only, and must not include an image tag or ID",
		},
		{
			name:     "Invalid/TaggedReg",
			args:     []string{"docker://reg.com/foo/bar:latest"},
			opts:     &MirrorOptions{},
			expError: "destination registry must consist of registry host and namespace(s) only, and must not include an image tag or ID",
		},
		{
			name:     "Invalid/TaggedNonFqdnReg",
			args:     []string{"docker://reg/foo/bar:latest"}, // warning message for parsing
			opts:     &MirrorOptions{},
			expError: "destination registry must consist of registry host and namespace(s) only, and must not include an image tag or ID",
		},
		{
			name:     "Invalid/TaggedNonFqdnRegPort",
			args:     []string{"docker://reg:5000/foo/bar:latest"},
			opts:     &MirrorOptions{},
			expError: "destination registry must consist of registry host and namespace(s) only, and must not include an image tag or ID",
		},
		{
			name:     "Invalid/EmptyRegistry",
			args:     []string{"docker://"},
			opts:     &MirrorOptions{},
			expError: `"" is not a valid image reference: repository name must have at least one component`,
		},
		{
			name:     "Invalid/EmptyScheme",
			args:     []string{"://foo"},
			opts:     &MirrorOptions{},
			expError: `unknown destination scheme ""`,
		},
		{
			name:     "Invalid/NoSchemeDelim",
			args:     []string{"foo"},
			opts:     &MirrorOptions{},
			expError: "no scheme delimiter in destination argument",
		},
		{
			name:     "Invalid/ExceedsNestedPathsLength",
			args:     []string{"docker://reg.com/foo/bar/baz"},
			opts:     &MirrorOptions{MaxNestedPaths: 2},
			expError: "the max-nested-paths value (2) must be strictly higher than the number of path-components in the destination foo/bar/baz - try increasing the value",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.opts.Complete(&cobra.Command{}, c.args)
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expOpts, c.opts)
			}
		})
	}
}

func TestMirrorValidate(t *testing.T) {

	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Error(err)
	}

	type spec struct {
		name     string
		opts     *MirrorOptions
		expError string
	}

	cases := []spec{
		{
			name: "Invalid/NoDest",
			opts: &MirrorOptions{
				From: "dir",
			},
			expError: "must specify a registry destination",
		},
		{
			name: "Invalid/NoFromWithManifestOnly",
			opts: &MirrorOptions{
				ManifestsOnly: true,
			},
			expError: "must specify a path to an archive with --from with --manifest-only",
		},
		{
			name: "Invalid/NoSource",
			opts: &MirrorOptions{
				ToMirror: u.Host,
			},
			expError: `must specify --config or --from with registry destination`,
		},
		{
			name: "Invalid/NoConfig",
			opts: &MirrorOptions{
				OutputDir: "dir",
			},
			expError: `must specify a configuration file with --config`,
		},
		{
			name: "Valid/MirrortoDisk/OCIFlag",
			opts: &MirrorOptions{
				OutputDir:               t.TempDir(),
				ConfigPath:              "testdata/configs/iscfg_oci_ops.yaml",
				IncludeLocalOCICatalogs: true,
			},
			expError: "",
		},
		{
			name: "Invalid/DisktoMirror/OCIFlag",
			opts: &MirrorOptions{
				From:                    t.TempDir(),
				ToMirror:                u.Host,
				IncludeLocalOCICatalogs: true,
			},
			expError: "oci feature cannot be used when publishing from a local archive to a registry",
		},
		{
			name: "Invalid/MirrorToMirror/ImageSetConfigWithOCI",
			opts: &MirrorOptions{
				ConfigPath:              "testdata/configs/iscfg_oci_ops.yaml",
				ToMirror:                u.Host,
				IncludeLocalOCICatalogs: false,
			},
			expError: "use of OCI FBC catalogs (prefix oci://) in configuration file is authorized only with flag --include-local-oci-catalogs",
		},
		{
			name: "Invalid/MirrorToMirror/ImageSetConfigWithoutOCI",
			opts: &MirrorOptions{
				ConfigPath:              "testdata/configs/iscfg.yaml",
				ToMirror:                u.Host,
				IncludeLocalOCICatalogs: true,
			},
			expError: "no operator found with OCI FBC catalog prefix (oci://) in configuration file, please execute without the --include-local-oci-catalogs flag",
		},
		{
			name: "Valid/ManifestOnlyWithFakeMirror",
			opts: &MirrorOptions{
				ManifestsOnly: true,
				From:          t.TempDir(),
				ToMirror:      "test.mirror.com",
			},
			expError: "",
		},
		{
			name: "Valid/MirrortoDisk",
			opts: &MirrorOptions{
				OutputDir:  t.TempDir(),
				ConfigPath: "testdata/configs/iscfg.yaml",
			},
			expError: "",
		},
		{
			name: "Valid/DisktoMirror",
			opts: &MirrorOptions{
				From:     t.TempDir(),
				ToMirror: u.Host,
			},
			expError: "",
		},
		{
			name: "Valid/MirrorToMirror",
			opts: &MirrorOptions{
				ConfigPath: "testdata/configs/iscfg.yaml",
				ToMirror:   u.Host,
			},
			expError: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.opts.Validate()
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRemovePreviouslyMirrored(t *testing.T) {
	type spec struct {
		name     string
		opts     *MirrorOptions
		meta     v1alpha2.Metadata
		images   image.TypedImageMapping
		expSet   image.AssociationSet
		expError string
	}

	cases := []spec{
		{
			name: "Valid/OneNewImage",
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expSet: image.AssociationSet{"test-registry/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": image.Associations{
				"test-registry/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": {
					Name:            "test-registry/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Path:            "single_manifest",
					TagSymlink:      "latest",
					ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Type:            v1alpha2.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests: []string{
						"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
						"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
						"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
						"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
					},
				},
			}},
			images: image.TypedImageMapping{
				{TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "imgname",
						ID:       "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOCPRelease}: {
					TypedImageReference: image.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry: "test-registry",
							Name:     "imgname",
							ID:       "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOCPRelease},
				{TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "imgname",
						ID:       "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df17",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOCPRelease}: {
					TypedImageReference: image.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry: "test-registry",
							Name:     "imgname",
							ID:       "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df17",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOCPRelease},
			},
			meta: v1alpha2.Metadata{
				MetadataSpec: v1alpha2.MetadataSpec{
					PastAssociations: []v1alpha2.Association{
						{
							Name:            "test-registry/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
							Path:            "single_manifest",
							TagSymlink:      "latest",
							ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
							Type:            v1alpha2.TypeGeneric,
							ManifestDigests: nil,
							LayerDigests: []string{
								"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
								"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
								"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
								"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
								"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
								"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
							},
						},
					},
				},
			},
		},
		{
			name: "Failure/NoNewImages",
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expSet:   image.AssociationSet{},
			expError: ErrNoUpdatesExist.Error(),
			images: image.TypedImageMapping{
				{TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "imgname",
						ID:       "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOCPRelease}: {
					TypedImageReference: image.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry: "test-registry",
							Name:     "imgname",
							ID:       "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOCPRelease},
			},
			meta: v1alpha2.Metadata{
				MetadataSpec: v1alpha2.MetadataSpec{
					PastAssociations: []v1alpha2.Association{
						{
							Name:            "test-registry/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
							Path:            "single_manifest",
							TagSymlink:      "latest",
							ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
							Type:            v1alpha2.TypeGeneric,
							ManifestDigests: nil,
							LayerDigests: []string{
								"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
								"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
								"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
								"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
								"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
								"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
							},
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			set, err := c.opts.removePreviouslyMirrored(c.images, c.meta)
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expSet, set)
			}
		})
	}
}

func TestNestedPaths(t *testing.T) {

	// test processNestedPaths
	o := &MirrorOptions{
		MaxNestedPaths: 2,
		ToMirror:       "localhost:5000",
		UserNamespace:  "ocpbugs-11922/mirror-release",
	}

	img := &image.TypedImage{
		TypedImageReference: image.TypedImageReference{
			Ref: reference.DockerImageReference{
				Name:      "ci-cd/gitlab-runner-ubi-images/gitlab-runner-helper-ocp",
				Namespace: "ocpbugs-11922/mirror-release/gitlab-org",
				Registry:  "localhost:5000",
			},
		},
	}

	t.Run("Testing processNestedPaths (2) : should fail", func(t *testing.T) {
		res := o.processNestedPaths(img)
		dst := strings.Join([]string{res.Ref.Registry, res.Ref.Namespace, res.Ref.Name}, "/")
		require.NotEqual(t, "localhost:5000/ocpbugs-11922/mirror-release/gitlab-org-ci-cd-gitlab-runner-ubi-images-gitlab-runner-helper-ocp", dst)
	})
	o.MaxNestedPaths = 3
	t.Run("Testing processNestedPaths (3) : should pass", func(t *testing.T) {
		res := o.processNestedPaths(img)
		dst := strings.Join([]string{res.Ref.Registry, res.Ref.Namespace, res.Ref.Name}, "/")
		require.Equal(t, "localhost:5000/ocpbugs-11922/mirror-release/gitlab-org-ci-cd-gitlab-runner-ubi-images-gitlab-runner-helper-ocp", dst)
	})

	o.MaxNestedPaths = 4
	t.Run("Testing processNestedPaths (4) : should pass", func(t *testing.T) {
		res := o.processNestedPaths(img)
		dst := strings.Join([]string{res.Ref.Registry, res.Ref.Namespace, res.Ref.Name}, "/")
		require.Equal(t, "localhost:5000/ocpbugs-11922/mirror-release/gitlab-org/ci-cd-gitlab-runner-ubi-images-gitlab-runner-helper-ocp", dst)
	})

	o.MaxNestedPaths = 5
	t.Run("Testing processNestedPaths (5) : should pass", func(t *testing.T) {
		res := o.processNestedPaths(img)
		dst := strings.Join([]string{res.Ref.Registry, res.Ref.Namespace, res.Ref.Name}, "/")
		require.Equal(t, "localhost:5000/ocpbugs-11922/mirror-release/gitlab-org/ci-cd/gitlab-runner-ubi-images-gitlab-runner-helper-ocp", dst)
	})

	o.MaxNestedPaths = 6
	t.Run("Testing processNestedPaths (6) : should pass", func(t *testing.T) {
		res := o.processNestedPaths(img)
		dst := strings.Join([]string{res.Ref.Registry, res.Ref.Namespace, res.Ref.Name}, "/")
		require.Equal(t, "localhost:5000/ocpbugs-11922/mirror-release/gitlab-org/ci-cd/gitlab-runner-ubi-images/gitlab-runner-helper-ocp", dst)
	})

	// change image
	img = &image.TypedImage{
		TypedImageReference: image.TypedImageReference{
			Ref: reference.DockerImageReference{
				Name:      "openshift4/ose-kube-rbac-proxy",
				Namespace: "ocpbugs-11922/mirror-release",
				Registry:  "localhost:5000",
			},
		},
	}

	o.MaxNestedPaths = 2
	t.Run("Testing processNestedPaths (2) new image : should pass", func(t *testing.T) {
		res := o.processNestedPaths(img)
		dst := strings.Join([]string{res.Ref.Registry, res.Ref.Namespace, res.Ref.Name}, "/")
		require.Equal(t, "localhost:5000/ocpbugs-11922/mirror-release-openshift4-ose-kube-rbac-proxy", dst)
	})

	o.MaxNestedPaths = 5
	t.Run("Testing processNestedPaths (5) new image : should pass", func(t *testing.T) {
		res := o.processNestedPaths(img)
		dst := strings.Join([]string{res.Ref.Registry, res.Ref.Namespace, res.Ref.Name}, "/")
		require.Equal(t, "localhost:5000/ocpbugs-11922/mirror-release/openshift4/ose-kube-rbac-proxy", dst)
	})

	o.MaxNestedPaths = 10
	t.Run("Testing processNestedPaths (10) new image : should pass", func(t *testing.T) {
		res := o.processNestedPaths(img)
		dst := strings.Join([]string{res.Ref.Registry, res.Ref.Namespace, res.Ref.Name}, "/")
		require.Equal(t, "localhost:5000/ocpbugs-11922/mirror-release/openshift4/ose-kube-rbac-proxy", dst)
	})

}
