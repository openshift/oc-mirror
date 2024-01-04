package mirror

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/reference"
	"github.com/containerd/containerd/remotes"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"
	ktest "k8s.io/klog/v2/test"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/operator/diff"
)

func TestPinImages(t *testing.T) {

	type spec struct {
		desc        string
		opts        *OperatorOptions
		dc          *declcfg.DeclarativeConfig
		resolver    remotes.Resolver
		expErrorStr string
	}

	cases := []spec{
		{
			desc: "Success/Resolved",
			opts: &OperatorOptions{
				MirrorOptions: &MirrorOptions{
					ContinueOnError: false,
					SkipMissing:     false,
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Name:  "foo.v1.0.0",
						Image: "regx1203109.com/ns/exist-bundle:latest",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "relatedimage1", Image: "regx1203109.com/ns/exist-relatedimage:latest"},
						},
					},
					{
						Name:  "bar.v1.0.0",
						Image: "regx1203109.com/ns/exist-bundle-notag",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "relatedimage1", Image: "regx1203109.com/ns/exist-relatedimage-notag"},
						},
					},
				},
			},
			resolver: mockResolver{
				digestMapping: map[string]string{
					"regx1203109.com/ns/exist-bundle:latest":       "sha256:1234",
					"regx1203109.com/ns/exist-relatedimage:latest": "sha256:5678",
				},
			},
		},
		{
			desc: "Error/NotFound",
			opts: &OperatorOptions{
				MirrorOptions: &MirrorOptions{
					ContinueOnError: false,
					SkipMissing:     false,
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Name:  "foo.v1.0.0",
						Image: "regx1203109.com/ns/notexist-bundle:latest",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "relatedimage1", Image: "regx1203109.com/ns/notexist-relatedimage:latest"},
						},
					},
				},
			},
			resolver:    mockResolver{digestMapping: map[string]string{}},
			expErrorStr: "not found",
		},
		{
			desc: "Error/NilConfig",
			opts: &OperatorOptions{
				MirrorOptions: &MirrorOptions{
					ContinueOnError: false,
					SkipMissing:     false,
				},
			},
			dc:          nil,
			resolver:    mockResolver{digestMapping: map[string]string{}},
			expErrorStr: "bug: nil declarative config",
		},
		{
			desc: "Success/ContinueOnError",
			opts: &OperatorOptions{
				MirrorOptions: &MirrorOptions{
					ContinueOnError: true,
					SkipMissing:     false,
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Name:  "foo.v1.0.0",
						Image: "docker.io/library/notexist-bundle:latest",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "relatedimage1", Image: "docker.io/library/notexist-relatedimage:latest"},
						},
					},
				},
			},
			resolver: mockResolver{digestMapping: map[string]string{}},
		},
		{
			desc: "Success/SkipMissing",
			opts: &OperatorOptions{
				MirrorOptions: &MirrorOptions{
					ContinueOnError: false,
					SkipMissing:     true,
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Name:  "foo.v1.0.0",
						Image: "docker.io/library/notexist-bundle:latest",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "relatedimage1", Image: "docker.io/library/notexist-relatedimage:latest"},
						},
					},
				},
			},
			resolver: mockResolver{digestMapping: map[string]string{}},
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			ctx := context.TODO()
			err := c.opts.pinImages(ctx, c.dc, c.resolver)
			if c.expErrorStr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, c.expErrorStr)
			}
		})
	}

}

func TestVerifyDC(t *testing.T) {

	mo := &MirrorOptions{
		RootOptions:     &cli.RootOptions{},
		ContinueOnError: true,
	}
	o := NewOperatorOptions(mo)
	o.complete()
	hook := test.NewLocal(o.Logger.Logger)

	type testvopf struct {
		desc           string
		dic            diff.DiffIncludeConfig
		dc             *declcfg.DeclarativeConfig
		logCount       int
		expErrorStr    string
		expErrReturned string
	}

	cases := []testvopf{
		{
			desc: "SuccessWithWarning/PackageNotFoundInDC",
			dic: diff.DiffIncludeConfig{
				Packages: []diff.DiffIncludePackage{
					{
						Name: "foo",
					},
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{
						Name:           "bar",
						DefaultChannel: "stable",
					},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.1", Skips: []string{"bar.v0.1.0"}},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.1",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.1"),
						},
					},
				},
			},
			logCount:    1,
			expErrorStr: "Operator foo was not found",
		},
		{
			desc: "Success/PackageFoundInDC",
			dic: diff.DiffIncludeConfig{
				Packages: []diff.DiffIncludePackage{
					{
						Name: "bar",
					},
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{
						Name:           "bar",
						DefaultChannel: "stable",
					},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.1", Skips: []string{"bar.v0.1.0"}},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.1",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.1"),
						},
					},
				},
			},
			logCount:    0,
			expErrorStr: "",
		},
		{
			desc: "Failure/DefaultChannelNotInDC",
			dic: diff.DiffIncludeConfig{
				Packages: []diff.DiffIncludePackage{
					{
						Name: "bar",
					},
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{
						Name:           "bar",
						DefaultChannel: "stable",
					},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "alpha", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.1", Skips: []string{"bar.v0.1.0"}},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.1",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.1"),
						},
					},
				},
			},
			logCount:    0,
			expErrorStr: "",
			expErrReturned: `invalid index:
└── invalid package "bar":
    └── invalid channel "stable":
        └── channel must contain at least one bundle`,
		},
		{
			desc: "Failure/InvalidSemverOrdering",
			dic: diff.DiffIncludeConfig{
				Packages: []diff.DiffIncludePackage{
					{
						Name: "bar",
					},
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{
						Name:           "bar",
						DefaultChannel: "stable",
					},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.1", Skips: []string{"bar.v0.1.0"}},
						{Name: "bar.v0.1.2", Replaces: "bar.v0.1.5"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.1",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.1"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.2",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.2"),
						},
					},
				},
			},
			logCount:    0,
			expErrorStr: "",
			expErrReturned: `invalid index:
└── invalid package "bar":
    └── invalid channel "stable":
        └── multiple channel heads found in graph: bar.v0.1.1, bar.v0.1.2`,
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			err := o.verifyDC(c.dic, c.dc)
			if c.expErrReturned != "" {
				require.EqualError(t, err, c.expErrReturned)
			} else {
				require.NoError(t, err)
				assert.Equal(t, c.logCount, len(hook.AllEntries()))
				if c.logCount > 0 && len(hook.Entries) > 0 {
					assert.Contains(t, hook.LastEntry().Message, c.expErrorStr)
				}
			}

		})
		hook.Reset()
	}
}

func mustParseAsBundle(t *testing.T, img string) (typedImage image.TypedImage) {
	t.Helper()

	// create a image with bundle type
	typedImage, err := image.ParseTypedImage(img, v1alpha2.TypeOperatorBundle)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestValidateMapping(t *testing.T) {

	type testCase struct {
		name                string                    // test case name
		dc                  declcfg.DeclarativeConfig // catalog to test with
		mapping             image.TypedImageMapping   // the source/destination mapping to search through looking for a match based on the catalog content
		expectedWarningMsgs []string                  // any warning messages that would be generated in the output
	}
	tests := []testCase{
		{
			name: "exact match with tags",
			dc: declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Image: "foo.com/example/bundle:v1",
						RelatedImages: []declcfg.RelatedImage{
							{
								Image: "foo.com/example/related:v1",
							},
						},
					},
				},
			},
			mapping: image.TypedImageMapping{
				mustParseAsBundle(t, "foo.com/example/bundle:v1"):  image.TypedImage{},
				mustParseAsBundle(t, "foo.com/example/related:v1"): image.TypedImage{},
			},
		},
		{
			name: "exact match with sha",
			dc: declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Image: "foo.com/example/bundle@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e",
						RelatedImages: []declcfg.RelatedImage{
							{
								Image: "foo.com/example/related@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3",
							},
						},
					},
				},
			},
			mapping: image.TypedImageMapping{
				mustParseAsBundle(t, "foo.com/example/bundle@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e"):  image.TypedImage{},
				mustParseAsBundle(t, "foo.com/example/related@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3"): image.TypedImage{},
			},
		},
		{
			name: "exact match with tag & sha",
			dc: declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Image: "foo.com/example/bundle:v1@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e",
						RelatedImages: []declcfg.RelatedImage{
							{
								Image: "foo.com/example/related:v1@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3",
							},
						},
					},
				},
			},
			mapping: image.TypedImageMapping{
				mustParseAsBundle(t, "foo.com/example/bundle:v1@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e"):  image.TypedImage{},
				mustParseAsBundle(t, "foo.com/example/related:v1@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3"): image.TypedImage{},
			},
		},
		{
			name: "partial match with tags - mapping was redirected to baz.com",
			dc: declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Image: "foo.com/example/bundle:v1",
						RelatedImages: []declcfg.RelatedImage{
							{
								Image: "foo.com/example/related:v1",
							},
						},
					},
				},
			},
			mapping: image.TypedImageMapping{
				mustParseAsBundle(t, "baz.com/example/bundle:v1"):  image.TypedImage{},
				mustParseAsBundle(t, "baz.com/example/related:v1"): image.TypedImage{},
			},
		},
		{
			name: "partial match with sha - mapping was redirected to baz.com",
			dc: declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Image: "foo.com/example/bundle@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e",
						RelatedImages: []declcfg.RelatedImage{
							{
								Image: "foo.com/example/related@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3",
							},
						},
					},
				},
			},
			mapping: image.TypedImageMapping{
				mustParseAsBundle(t, "baz.com/example/bundle@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e"):  image.TypedImage{},
				mustParseAsBundle(t, "baz.com/example/related@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3"): image.TypedImage{},
			},
		},
		{
			name: "partial match with tag & sha - mapping was redirected to baz.com",
			dc: declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Image: "foo.com/example/bundle:v1@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e",
						RelatedImages: []declcfg.RelatedImage{
							{
								Image: "foo.com/example/related:v1@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3",
							},
						},
					},
				},
			},
			mapping: image.TypedImageMapping{
				mustParseAsBundle(t, "baz.com/example/bundle:v1@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e"):  image.TypedImage{},
				mustParseAsBundle(t, "baz.com/example/related:v1@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3"): image.TypedImage{},
			},
		},
		{
			name: "partial match with tags - mapping was redirected to baz.com/foo/bar",
			dc: declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Image: "foo.com/example/bundle:v1",
						RelatedImages: []declcfg.RelatedImage{
							{
								Image: "foo.com/example/related:v1",
							},
						},
					},
				},
			},
			mapping: image.TypedImageMapping{
				mustParseAsBundle(t, "baz.com/foo/bar/example/bundle:v1"):  image.TypedImage{},
				mustParseAsBundle(t, "baz.com/foo/bar/example/related:v1"): image.TypedImage{},
			},
		},
		{
			name: "partial match with sha - mapping was redirected to baz.com/foo/bar",
			dc: declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Image: "foo.com/example/bundle@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e",
						RelatedImages: []declcfg.RelatedImage{
							{
								Image: "foo.com/example/related@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3",
							},
						},
					},
				},
			},
			mapping: image.TypedImageMapping{
				mustParseAsBundle(t, "baz.com/foo/bar/example/bundle@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e"):  image.TypedImage{},
				mustParseAsBundle(t, "baz.com/foo/bar/example/related@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3"): image.TypedImage{},
			},
		},
		{
			name: "partial match with tag & sha - mapping was redirected to baz.com/foo/bar",
			dc: declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Image: "foo.com/example/bundle:v1@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e",
						RelatedImages: []declcfg.RelatedImage{
							{
								Image: "foo.com/example/related:v1@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3",
							},
						},
					},
				},
			},
			mapping: image.TypedImageMapping{
				mustParseAsBundle(t, "baz.com/foo/bar/example/bundle:v1@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e"):  image.TypedImage{},
				mustParseAsBundle(t, "baz.com/foo/bar/example/related:v1@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3"): image.TypedImage{},
			},
		},
		{
			name: "no match with sha, tag, or sha & tag",
			dc: declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Image: "foo.com/example/bundle@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e",
						RelatedImages: []declcfg.RelatedImage{
							{
								Image: "foo.com/example/related:v1",
							},
							{
								Image: "foo.com/example/relatedtoo:v1@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3",
							},
						},
					},
				},
			},
			mapping: image.TypedImageMapping{
				mustParseAsBundle(t, "foo.com/notfound@sha256:fe19778cf1ce280658154f2b9c01ffbccd825a23460141dcf3794e7a2c0eb629"): image.TypedImage{},
			},
			expectedWarningMsgs: []string{
				"image foo.com/example/related:v1 is not included in mapping",
				"image foo.com/example/bundle@sha256:2db967de122e3b71a54d1fef109925d45aab481dbd3c8d4bc18948848102e27e is not included in mapping",
				"image foo.com/example/relatedtoo:v1@sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3 is not included in mapping",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ktest.InitKlog(t)
			// This test needs to validate that the log generates the right output
			// so restore the global klog state after each test
			defer klog.CaptureState().Restore()

			// override the logger so we can capture its output
			var buf bytes.Buffer
			klog.SetOutput(&buf)

			// run the function we need to test
			err := validateMapping(test.dc, test.mapping)
			assert.NoError(t, err)

			// grab the log output
			got := buf.String()

			// did we get output when there were no expected messages?
			if len(test.expectedWarningMsgs) == 0 {
				assert.Empty(t, got)
			}
			// if we have expected warning messages, are they present in the log output?
			for _, expectedWarningMsg := range test.expectedWarningMsgs {
				assert.Contains(t, got, expectedWarningMsg)
			}

		})
	}
}

type mockResolver struct {
	digestMapping map[string]string
}

func (r mockResolver) Resolve(ctx context.Context, ref string) (name string, desc ocispec.Descriptor, err error) {
	if r.digestMapping == nil {
		panic("mockResolver has not been initialized")
	}

	spec, err := reference.Parse(ref)
	if err != nil {
		return name, desc, err
	}

	fmt.Printf("%#v\n", spec)
	if d, ok := r.digestMapping[spec.String()]; ok {
		desc.Digest = digest.Digest(d)
	} else {
		err = errdefs.ErrNotFound
	}

	return spec.String(), desc, err
}

func (r mockResolver) Fetcher(ctx context.Context, ref string) (remotes.Fetcher, error) {
	return nil, nil
}

func (r mockResolver) Pusher(ctx context.Context, ref string) (remotes.Pusher, error) {
	return nil, nil
}
