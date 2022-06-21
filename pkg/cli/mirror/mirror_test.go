package mirror

import (
	"net/http/httptest"
	"net/url"
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
			opts: &MirrorOptions{},
			expOpts: &MirrorOptions{
				ToMirror: "reg.com",
			},
		},
		{
			name: "Valid/LocalhostRegDest",
			args: []string{"docker://localhost"},
			opts: &MirrorOptions{},
			expOpts: &MirrorOptions{
				ToMirror: "localhost",
			},
		},
		{
			name: "Valid/FqdnRegPortDest",
			args: []string{"docker://reg.com:5000"},
			opts: &MirrorOptions{},
			expOpts: &MirrorOptions{
				ToMirror: "reg.com:5000",
			},
		},
		{
			name: "Valid/LocalhostRegPortDest",
			args: []string{"docker://localhost:5000"},
			opts: &MirrorOptions{},
			expOpts: &MirrorOptions{
				ToMirror: "localhost:5000",
			},
		},
		{
			name: "Valid/RegNamespace",
			args: []string{"docker://reg.com/foo/bar"},
			opts: &MirrorOptions{},
			expOpts: &MirrorOptions{
				ToMirror:      "reg.com",
				UserNamespace: "foo/bar",
			},
		},
		{
			name: "Valid/LocalhostRegNamespace",
			args: []string{"docker://localhost/foo/bar"},
			opts: &MirrorOptions{},
			expOpts: &MirrorOptions{
				ToMirror:      "localhost",
				UserNamespace: "foo/bar",
			},
		},
		{
			name: "Valid/NonFqdnRegPortNamespace",
			args: []string{"docker://reg:5000/foo"},
			opts: &MirrorOptions{},
			expOpts: &MirrorOptions{
				ToMirror:      "reg:5000",
				UserNamespace: "foo",
			},
		},
		{
			name: "Valid/NonFqdnRegPortNamespaceName",
			args: []string{"docker://reg:5000/foo/bar"},
			opts: &MirrorOptions{},
			expOpts: &MirrorOptions{
				ToMirror:      "reg:5000",
				UserNamespace: "foo/bar",
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
				ConfigPath: "foo",
				ToMirror:   u.Host,
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
				ConfigPath: "foo",
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
				{TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "imgname",
						ID:       "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOCPRelease}: {
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry: "test-registry",
							Name:     "imgname",
							ID:       "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOCPRelease},
				{TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "imgname",
						ID:       "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df17",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOCPRelease}: {
					TypedImageReference: imagesource.TypedImageReference{
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
				{TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "imgname",
						ID:       "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOCPRelease}: {
					TypedImageReference: imagesource.TypedImageReference{
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
