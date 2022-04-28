package operator

import (
	"testing"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/stretchr/testify/require"
)

func TestConvertDCToIncludeConfig(t *testing.T) {
	type spec struct {
		name     string
		cfg      declcfg.DeclarativeConfig
		strategy IncludeConfigConverter
		exp      v1alpha2.IncludeConfig
	}

	specs := []spec{
		{
			name:     "Success/HeadsOnlyCatalog",
			strategy: &catalogStrategy{},
			cfg: declcfg.DeclarativeConfig{
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
			exp: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "bar",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Success/HeadsOnlyPackages",
			strategy: &includeStrategy{
				curr: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "bar",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name: "stable",
									IncludeBundle: v1alpha2.IncludeBundle{
										StartingVersion: semver.MustParse("0.1.2"),
									},
								},
							},
						},
						{
							Name: "foo",
						},
					},
				},
			},
			cfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.1", Skips: []string{"bar.v0.1.0"}},
						{Name: "bar.v0.1.2", Skips: []string{"bar.v0.1.1"}},
						{Name: "bar.v0.1.3", Skips: []string{"bar.v0.1.2"}},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.0.1"},
						{Name: "foo.v0.0.2", Replaces: "foo.v0.0.1"},
						{Name: "foo.v0.0.3", Replaces: "foo.v0.0.2"},
						{Name: "foo.v0.2.0", Replaces: "foo.v0.0.3"},
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
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.3",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.3"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.0.1",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.0.1"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.0.2",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.0.2"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.0.3",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.0.3"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
				},
			},
			exp: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "bar",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									StartingVersion: semver.MustParse("0.1.2"),
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									StartingVersion: semver.MustParse("0.0.1"),
								},
							},
						},
					},
				},
			},
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			ic, err := s.strategy.ConvertDCToIncludeConfig(s.cfg)
			require.NoError(t, err)
			require.Equal(t, s.exp, ic)
		})
	}
}

func TestUpdateIncludeConfig(t *testing.T) {
	type spec struct {
		name     string
		cfg      declcfg.DeclarativeConfig
		strategy IncludeConfigConverter
		in       v1alpha2.IncludeConfig
		exp      v1alpha2.IncludeConfig
		expErr   string
	}

	specs := []spec{
		{
			name:     "Success/NewPackages",
			strategy: &catalogStrategy{},
			cfg: declcfg.DeclarativeConfig{
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
			in: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "bar",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
				},
			},
			exp: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "bar",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
				},
			},
		},
		{
			name:     "Success/NewChannels",
			strategy: &catalogStrategy{},
			cfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
						{Name: "bar.v0.1.1", Replaces: "bar.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "alpha", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
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
			in: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "bar",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
				},
			},
			exp: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "bar",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "alpha",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
				},
			},
		},
		{
			name:     "Success/PruneChannelHead",
			strategy: &catalogStrategy{},
			cfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.1", Skips: []string{"bar.v0.1.0"}},
						{Name: "bar.v0.1.2", Skips: []string{"bar.v0.1.1"}},
						{Name: "bar.v0.1.3", Skips: []string{"bar.v0.1.2"}},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.0.1"},
						{Name: "foo.v0.0.2", Replaces: "foo.v0.0.1"},
						{Name: "foo.v0.0.3", Replaces: "foo.v0.0.2"},
						{Name: "foo.v0.2.0", Replaces: "foo.v0.0.3"},
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
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.3",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.3"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.0.1",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.0.1"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.0.2",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.0.2"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.0.3",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.0.3"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
				},
			},
			in: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "bar",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
				},
			},
			exp: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "bar",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.1",
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.2.0",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Success/PruneChannelHeadWithPackages",
			strategy: &includeStrategy{
				curr: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "bar",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name: "stable",
									IncludeBundle: v1alpha2.IncludeBundle{
										StartingVersion: semver.MustParse("0.1.3"),
									},
								},
							},
						},
						{
							Name: "foo",
						},
					},
				},
			},
			cfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.1", Skips: []string{"bar.v0.1.0"}},
						{Name: "bar.v0.1.2", Skips: []string{"bar.v0.1.1"}},
						{Name: "bar.v0.1.3", Skips: []string{"bar.v0.1.2"}},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.0.1"},
						{Name: "foo.v0.0.2", Replaces: "foo.v0.0.1"},
						{Name: "foo.v0.0.3", Replaces: "foo.v0.0.2"},
						{Name: "foo.v0.2.0", Replaces: "foo.v0.0.3"},
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
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.1.3",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.3"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.0.1",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.0.1"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.0.2",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.0.2"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.0.3",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.0.3"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
				},
			},
			in: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "bar",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									StartingVersion: semver.MustParse("0.1.2"),
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									StartingVersion: semver.MustParse("0.1.0"),
								},
							},
						},
					},
				},
			},
			exp: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "bar",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									StartingVersion: semver.MustParse("0.1.3"),
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									StartingVersion: semver.MustParse("0.2.0"),
								},
							},
						},
					},
				},
			},
		},
		{
			name:     "Success/NoNextBundle",
			strategy: &catalogStrategy{},
			cfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.0.2", Skips: []string{"bar.v0.0.1"}},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "bar.v0.0.2",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.0.2"),
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
			in: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "bar",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
				},
			},
			exp: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "bar",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.0.0",
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			ic, err := s.strategy.UpdateIncludeConfig(s.cfg, s.in)
			if s.expErr != "" {
				require.EqualError(t, err, s.expErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, s.exp, ic)
			}
		})
	}
}
