package operator

import (
	"testing"

	"github.com/blang/semver/v4"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/stretchr/testify/require"
)

func TestConvertDCToIncludeConfig(t *testing.T) {
	type spec struct {
		name     string
		cfg      declcfg.DeclarativeConfig
		strategy IncludeConfigManager
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
			strategy: &packageStrategy{
				curr: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "bar",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name: "stable",
									IncludeBundle: v1alpha2.IncludeBundle{
										MinVersion: "0.1.2",
									},
								},
								{
									Name: "alpha",
								},
							},
						},
						{
							Name: "baz",
							IncludeBundle: v1alpha2.IncludeBundle{
								MinVersion: "0.1.2",
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
					{Schema: "olm.package", Name: "baz", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.1", Skips: []string{"bar.v0.1.0"}},
						{Name: "bar.v0.1.2", Skips: []string{"bar.v0.1.1"}},
						{Name: "bar.v0.1.3", Skips: []string{"bar.v0.1.2"}},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "baz", Entries: []declcfg.ChannelEntry{
						{Name: "baz.v0.1.1", Skips: []string{"baz.v0.1.0"}},
						{Name: "baz.v0.1.2", Skips: []string{"baz.v0.1.1"}},
						{Name: "baz.v0.1.3", Skips: []string{"baz.v0.1.2"}},
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
						Name:    "baz.v0.1.1",
						Package: "baz",
						Image:   "reg/baz:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("baz", "0.1.1"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "baz.v0.1.2",
						Package: "baz",
						Image:   "reg/baz:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("baz", "0.1.2"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "baz.v0.1.3",
						Package: "baz",
						Image:   "reg/baz:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("baz", "0.1.3"),
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
									MinVersion: "0.1.2",
								},
							},
						},
					},
					{
						Name: "baz",
						IncludeBundle: v1alpha2.IncludeBundle{
							MinVersion: "0.1.2",
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.0.1",
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

func TestUpdateIncludeConfig_Catalog(t *testing.T) {
	type spec struct {
		name     string
		cfg      declcfg.DeclarativeConfig
		strategy IncludeConfigManager
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
						{Name: "bar.v0.2.2", Skips: []string{"bar.v0.0.2"}},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.0.2"},
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
						Name:    "bar.v0.2.2",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.2.2"),
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
									MinVersion: "0.2.2",
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
									MinVersion: "0.0.2",
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

func TestUpdateIncludeConfig_Package(t *testing.T) {
	type spec struct {
		name     string
		cfg      declcfg.DeclarativeConfig
		strategy IncludeConfigManager
		in       v1alpha2.IncludeConfig
		exp      v1alpha2.IncludeConfig
		expErr   string
	}

	specs := []spec{
		{
			name: "Success/NewPackages",
			strategy: &packageStrategy{
				curr: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "bar",
							IncludeBundle: v1alpha2.IncludeBundle{
								MinVersion: "0.1.0",
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
						IncludeBundle: v1alpha2.IncludeBundle{
							MinVersion: "0.1.0",
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
			name: "Success/NewChannels",
			strategy: &packageStrategy{
				curr: v1alpha2.IncludeConfig{
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
								{
									Name: "alpha",
								},
							},
						},
						{
							Name: "foo",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name: "alpha",
								},
							},
						},
					},
				},
			},
			cfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: "olm.package", Name: "bar", DefaultChannel: "stable"},
					{Schema: "olm.package", Name: "foo", DefaultChannel: "alpha"},
				},
				Channels: []declcfg.Channel{
					{Schema: "olm.channel", Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
						{Name: "bar.v0.1.1", Replaces: "bar.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "alpha", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
					{Schema: "olm.channel", Name: "alpha", Package: "foo", Entries: []declcfg.ChannelEntry{
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
								Name: "alpha",
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
			name: "Success/PruneChannelHead",
			strategy: &packageStrategy{
				curr: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "bar",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name: "stable",
									IncludeBundle: v1alpha2.IncludeBundle{
										MinVersion: "0.1.3",
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
					{Schema: "olm.channel", Name: "alpha", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0", Skips: []string{"foo.v0.1.0"}},
						{Name: "foo.v0.2.1", Skips: []string{"foo.v0.2.0"}},
					}},
					{Schema: "olm.channel", Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.0.1"},
						{Name: "foo.v0.0.2", Replaces: "foo.v0.0.1"},
						{Name: "foo.v0.0.3", Replaces: "foo.v0.0.2"},
						{Name: "foo.v0.2.0", Replaces: "foo.v0.0.3"},
						{Name: "foo.v0.2.1", Replaces: "foo.v0.2.0"},
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
					{
						Schema:  "olm.bundle",
						Name:    "foo.v0.2.1",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.1"),
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
									MinVersion: "0.1.2",
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
									MinVersion: "0.1.3",
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "alpha",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.2.1",
								},
							},
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
			name: "Success/NoNextBundle",
			strategy: &packageStrategy{
				curr: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "bar",
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
									MinVersion: "0.0.2",
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

func TestSearch(t *testing.T) {
	type spec struct {
		name            string
		versions        []semver.Version
		target          semver.Version
		expectedVersion semver.Version
	}

	cases := []spec{
		{
			name: "Valid/TargetExistsInVersions",
			versions: []semver.Version{
				semver.MustParse("0.0.1"),
				semver.MustParse("0.0.2"),
				semver.MustParse("0.1.0"),
				semver.MustParse("0.2.0"),
				semver.MustParse("0.3.0"),
				semver.MustParse("0.4.0"),
			},
			target:          semver.MustParse("0.1.0"),
			expectedVersion: semver.MustParse("0.2.0"),
		},
		{
			name: "Valid/TargetDoesNotExistInVersions",
			versions: []semver.Version{
				semver.MustParse("0.0.1"),
				semver.MustParse("0.0.2"),
				semver.MustParse("0.2.0"),
				semver.MustParse("0.3.0"),
				semver.MustParse("0.4.0"),
			},
			target:          semver.MustParse("0.1.0"),
			expectedVersion: semver.MustParse("0.2.0"),
		},
		{
			name: "Valid/OneHigherAndOneLower",
			versions: []semver.Version{
				semver.MustParse("0.0.2"),
				semver.MustParse("0.2.0"),
			},
			target:          semver.MustParse("0.1.0"),
			expectedVersion: semver.MustParse("0.2.0"),
		},
		{
			name: "Valid/TargetIsLatestVersion",
			versions: []semver.Version{
				semver.MustParse("0.0.1"),
				semver.MustParse("0.0.2"),
				semver.MustParse("0.1.0"),
			},
			target:          semver.MustParse("0.1.0"),
			expectedVersion: semver.Version{},
		},
		{
			name: "Valid/OneBundleInVersions",
			versions: []semver.Version{
				semver.MustParse("0.0.1"),
			},
			target:          semver.MustParse("0.1.0"),
			expectedVersion: semver.MustParse("0.0.1"),
		},
		{
			name:            "Valid/NoBundlesInVersions",
			versions:        []semver.Version{},
			target:          semver.MustParse("0.1.0"),
			expectedVersion: semver.Version{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := search(c.versions, c.target, 0, len(c.versions)-1)
			require.Equal(t, c.expectedVersion, actual)
		})
	}

}
