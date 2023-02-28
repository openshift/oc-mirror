package mirror

import (
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

	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/operator/diff"
)

func TestOperatorCatalogPlatform(t *testing.T) {
	type test struct {
		name              string                  // test case name
		platform          OperatorCatalogPlatform // platform under test
		expected          string                  // expected value when converted to string
		expectedRoundTrip OperatorCatalogPlatform // expected value when string is converted back to platform
	}
	tests := []test{
		{
			name:              "empty",
			platform:          OperatorCatalogPlatform{isIndex: false},
			expected:          "",
			expectedRoundTrip: OperatorCatalogPlatform{isIndex: false},
		},
		{
			name:              "os only",
			platform:          OperatorCatalogPlatform{os: "linux", isIndex: true},
			expected:          "linux",
			expectedRoundTrip: OperatorCatalogPlatform{os: "linux", isIndex: true},
		},
		{
			name:              "architecture only",
			platform:          OperatorCatalogPlatform{architecture: "arm64", isIndex: true},
			expected:          "arm64",
			expectedRoundTrip: OperatorCatalogPlatform{architecture: "arm64", isIndex: true},
		},
		{
			name:              "variant only",
			platform:          OperatorCatalogPlatform{variant: "v8", isIndex: true},
			expected:          "v8",
			expectedRoundTrip: OperatorCatalogPlatform{variant: "v8", isIndex: true},
		},
		{
			name:              "os/arch (most common case)",
			platform:          OperatorCatalogPlatform{os: "linux", architecture: "arm64", isIndex: true},
			expected:          "linux-arm64",
			expectedRoundTrip: OperatorCatalogPlatform{os: "linux", architecture: "arm64", isIndex: true},
		},
		{
			name:              "arch/variant",
			platform:          OperatorCatalogPlatform{architecture: "arm64", variant: "v8", isIndex: true},
			expected:          "arm64-v8",
			expectedRoundTrip: OperatorCatalogPlatform{architecture: "arm64", variant: "v8", isIndex: true},
		},
		{
			name:              "os/arch/variant",
			platform:          OperatorCatalogPlatform{os: "linux", architecture: "arm64", variant: "v8", isIndex: true},
			expected:          "linux-arm64-v8",
			expectedRoundTrip: OperatorCatalogPlatform{os: "linux", architecture: "arm64", variant: "v8", isIndex: true},
		},
		{
			name:              "empty - single arch",
			platform:          OperatorCatalogPlatform{isIndex: false},
			expected:          "",
			expectedRoundTrip: OperatorCatalogPlatform{isIndex: false},
		},
		{
			name:              "os only - single arch",
			platform:          OperatorCatalogPlatform{os: "linux", isIndex: false},
			expected:          "",
			expectedRoundTrip: OperatorCatalogPlatform{isIndex: false},
		},
		{
			name:              "architecture only - single arch",
			platform:          OperatorCatalogPlatform{architecture: "arm64", isIndex: false},
			expected:          "",
			expectedRoundTrip: OperatorCatalogPlatform{isIndex: false},
		},
		{
			name:              "variant only - single arch",
			platform:          OperatorCatalogPlatform{variant: "v8", isIndex: false},
			expected:          "",
			expectedRoundTrip: OperatorCatalogPlatform{isIndex: false},
		},
		{
			name:              "os/arch (most common case) - single arch",
			platform:          OperatorCatalogPlatform{os: "linux", architecture: "arm64", isIndex: false},
			expected:          "",
			expectedRoundTrip: OperatorCatalogPlatform{isIndex: false},
		},
		{
			name:              "arch/variant - single arch",
			platform:          OperatorCatalogPlatform{architecture: "arm64", variant: "v8", isIndex: false},
			expected:          "",
			expectedRoundTrip: OperatorCatalogPlatform{isIndex: false},
		},
		{
			name:              "os/arch/variant - single arch",
			platform:          OperatorCatalogPlatform{os: "linux", architecture: "arm64", variant: "v8", isIndex: false},
			expected:          "",
			expectedRoundTrip: OperatorCatalogPlatform{isIndex: false},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := test.platform.String()
			require.Equal(t, test.expected, actual)
			require.Equal(t, test.expectedRoundTrip, *NewOperatorCatalogPlatform(test.expected))
		})
	}
}

func TestPinImages(t *testing.T) {

	type spec struct {
		desc                     string
		opts                     *OperatorOptions
		renderResultsPerPlatform map[OperatorCatalogPlatform]CatalogMetadata
		resolver                 remotes.Resolver
		expErrorStr              string
	}
	amd64SingleArchPlatform := OperatorCatalogPlatform{
		os:           "linux",
		architecture: "amd64",
		variant:      "",
		isIndex:      false,
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
			renderResultsPerPlatform: map[OperatorCatalogPlatform]CatalogMetadata{
				amd64SingleArchPlatform: {
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
			renderResultsPerPlatform: map[OperatorCatalogPlatform]CatalogMetadata{
				amd64SingleArchPlatform: {
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
			renderResultsPerPlatform: map[OperatorCatalogPlatform]CatalogMetadata{
				amd64SingleArchPlatform: {
					dc: nil,
				},
			},
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
			renderResultsPerPlatform: map[OperatorCatalogPlatform]CatalogMetadata{
				amd64SingleArchPlatform: {
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
			renderResultsPerPlatform: map[OperatorCatalogPlatform]CatalogMetadata{
				amd64SingleArchPlatform: {
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
				},
			},
			resolver: mockResolver{digestMapping: map[string]string{}},
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			ctx := context.TODO()
			err := c.opts.pinImages(ctx, c.renderResultsPerPlatform, c.resolver)
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
		RootOptions: &cli.RootOptions{},
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
