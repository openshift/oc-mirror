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

// func TestPlanFull(t *testing.T) {
// 	mo := &MirrorOptions{
// 		RootOptions: &cli.RootOptions{},
// 	}
// 	o := NewOperatorOptions(mo)
// 	o.complete()
// 	o.PlanFull(context.TODO(), v1alpha2.ImageSetConfiguration{
// 		TypeMeta: v1alpha2.NewMetadata().TypeMeta,
// 		ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
// 			Mirror: v1alpha2.Mirror{
// 				Platform: v1alpha2.Platform{
// 					Channels: []v1alpha2.ReleaseChannel{
// 						{
// 							Name: "stable-4.10",
// 							Type: v1alpha2.TypeOCP,
// 						},
// 					},
// 				},
// 				Operators: []v1alpha2.Operator{
// 					{
// 						//Catalog: "file:///home/skhoury/go/src/github.com/openshift/oc-mirror/rhopid",
// 						Catalog: "oci:/home/skhoury/go/src/github.com/openshift/oc-mirror/rhopid",
// 						IncludeConfig: v1alpha2.IncludeConfig{

// 							Packages: []v1alpha2.IncludePackage{
// 								{
// 									Name: "serverless-operator",
// 									Channels: []v1alpha2.IncludeChannel{
// 										{
// 											Name: "stable",
// 										},
// 									},
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			StorageConfig: v1alpha2.StorageConfig{
// 				Local: &v1alpha2.LocalConfig{
// 					Path: "./dest",
// 				},
// 			},
// 		},
// 	})
// }

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
