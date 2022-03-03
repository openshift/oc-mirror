package operator

import (
	"testing"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/stretchr/testify/require"
)

func TestMerge(t *testing.T) {
	type spec struct {
		name      string
		mt        Merger
		dc, expDC *declcfg.DeclarativeConfig
		expError  string
	}

	cases := []spec{
		{
			name:  "TwoWay/Empty",
			mt:    &TwoWayStrategy{},
			dc:    &declcfg.DeclarativeConfig{},
			expDC: &declcfg.DeclarativeConfig{},
		},
		{
			name: "TwoWay/NoMergeNeeded",
			mt:   &TwoWayStrategy{},
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "foo", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
				},
			},
			expDC: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
		},
		{
			name: "TwoWay/MergePackagesChannelsBundles",
			mt:   &TwoWayStrategy{},
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "foo", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "baz", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.0.5"},
						{Name: "foo.v0.1.0"},
						{Name: "foo.v0.1.0", Replaces: "foo.v0.0.5"},
						{Name: "foo.v0.1.0", Skips: []string{"foo.v0.0.4"}},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.1", Replaces: "foo.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "baz", Entries: []declcfg.ChannelEntry{
						{Name: "baz.v2.0.0", SkipRange: ">=0.1.17 <2.0.0"},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "baz", Entries: []declcfg.ChannelEntry{
						{Name: "baz.v3.0.0", SkipRange: ">=0.1.17 <3.0.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.0.5",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.0.5"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.1.1",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.1"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("foo.example.com", "v1", "Foo"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "baz.v3.0.0",
						Package: "baz",
						Image:   "reg/baz:latest",
						Properties: []property.Property{
							property.MustBuildPackage("baz", "3.0.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "baz.v2.0.0",
						Package: "baz",
						Image:   "reg/baz:latest",
						Properties: []property.Property{
							property.MustBuildPackage("baz", "2.0.0"),
						},
					},
				},
			},
			expDC: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.0.5"},
						{Name: "foo.v0.1.0", Replaces: "foo.v0.0.5", Skips: []string{"foo.v0.0.4"}},
						{Name: "foo.v0.1.1", Replaces: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							// Can't merge properties since their keys are unknown.
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.0.5",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.0.5"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.1.1",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.1"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "baz.v3.0.0",
						Package: "baz",
						Image:   "reg/baz:latest",
						Properties: []property.Property{
							property.MustBuildPackage("baz", "3.0.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "baz.v2.0.0",
						Package: "baz",
						Image:   "reg/baz:latest",
						Properties: []property.Property{
							property.MustBuildPackage("baz", "2.0.0"),
						},
					},
				},
			},
		},
		{
			name:  "PreferLast/Empty",
			mt:    &PreferLastStrategy{},
			dc:    &declcfg.DeclarativeConfig{},
			expDC: &declcfg.DeclarativeConfig{},
		},
		{
			name: "PreferLast/NoMergeNeeded",
			mt:   &PreferLastStrategy{},
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "foo", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
				},
			},
			expDC: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
		},
		{
			name: "PreferLast/MergePackagesChannelsBundles",
			mt:   &PreferLastStrategy{},
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "foo", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "foo", DefaultChannel: "alpha"},
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "alpha", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0", Replaces: "foo.v0.0.5"},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("foo.example.com", "v1", "Foo"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
				},
			},
			expDC: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "foo", DefaultChannel: "alpha"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "alpha", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0", Replaces: "foo.v0.0.5"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.mt.Merge(c.dc)
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expDC, c.dc)
			}
		})
	}
}
