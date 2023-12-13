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
			name: "Success/OCPBUGS-21865/filteringByChannelMinAndMax",
			cfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{
						Schema:         "olm.package",
						Name:           "cincinnati-operator",
						DefaultChannel: "v1",
					},
					{
						Schema:         "olm.package",
						Name:           "local-storage-operator",
						DefaultChannel: "stable",
					},
					{
						Schema:         "olm.package",
						Name:           "odf-operator",
						DefaultChannel: "stable-4.12",
					},
					{
						Schema:         "olm.package",
						Name:           "rhsso-operator",
						DefaultChannel: "stable",
					},
				},
				Channels: []declcfg.Channel{
					{
						Schema:  "olm.channel",
						Name:    "v1",
						Package: "cincinnati-operator",
						Entries: []declcfg.ChannelEntry{
							{Name: "update-service-operator.v5.0.1", Replaces: "update-service-operator.v4.9.1", Skips: []string{"update-service-operator.v5.0.0"}, SkipRange: ""},
						},
					},
					{
						Schema:  "olm.channel",
						Name:    "stable",
						Package: "local-storage-operator",
						Entries: []declcfg.ChannelEntry{
							{Name: "local-storage-operator.v4.12.0-202305262042", Replaces: "", Skips: []string{}, SkipRange: ">=4.3.0 <4.12.0-202305262042"},
						},
					},
					{
						Schema:  "olm.channel",
						Name:    "stable-4.12",
						Package: "odf-operator",
						Entries: []declcfg.ChannelEntry{
							{Name: "odf-operator.v4.12.4-rhodf", Replaces: "odf-operator.v4.12.3-rhodf", Skips: []string{}, SkipRange: ">=4.2.0 <4.12.4-rhodf"},
						},
					},
					{
						Schema:  "olm.channel",
						Name:    "stable",
						Package: "rhsso-operator",
						Entries: []declcfg.ChannelEntry{
							{Name: "rhsso-operator.7.6.4-opr-002", Replaces: "rhsso-operator.7.6.4-opr-001", Skips: []string{"rhsso-operator.7.6.0-opr-001"}, SkipRange: ""},
						},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "update-service-operator.v5.0.1",
						Package: "cincinnati-operator",
						Image:   "registry.redhat.io/openshift-update-service/cincinnati-operator-bundle@sha256:15f80efb399fc33a80e9979df4e8044501737eee335a2a5034c8de98957a87b9",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "", Image: "registry.redhat.io/openshift-update-service/cincinnati-operator-bundle@sha256:15f80efb399fc33a80e9979df4e8044501737eee335a2a5034c8de98957a87b9"},
							{Name: "updateservice-operator", Image: "registry.redhat.io/openshift-update-service/openshift-update-service-rhel8-operator@sha256:8e9553e1041cd685239ef131e5b1b8797e3c033f008a122770a1ebfa01670d89"},
							{Name: "operand", Image: "registry.redhat.io/openshift-update-service/openshift-update-service-rhel8@sha256:fefab6832db6353886ed34aad76d1698532a1927160b8f8d34b3491c6025bae8"},
						},
						Properties: []property.Property{
							property.MustBuildGVKRequired("updateservice.operator.openshift.io", "v1", "UpdateService"),
							property.MustBuildPackage("cincinnati-operator", "5.0.1"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "local-storage-operator.v4.12.0-202305262042",
						Package: "local-storage-operator",
						Image:   "registry.redhat.io/openshift4/ose-local-storage-operator-bundle@sha256:b0f47e35fbd4e8d0053a28a79d8d59d92458d14f31e9d7a1ffbd71416da7a660",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "ose-kube-rbac-proxy", Image: "registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:9b60df6bdda991189abc6f164db78479ae7dc7127c747a5cf8fa566a910b264d"},
							{Name: "", Image: "registry.redhat.io/openshift4/ose-local-storage-operator-bundle@sha256:b0f47e35fbd4e8d0053a28a79d8d59d92458d14f31e9d7a1ffbd71416da7a660"},
							{Name: "ose-local-storage-diskmaker", Image: "registry.redhat.io/openshift4/ose-local-storage-diskmaker@sha256:815f2401b118ff0c14b87398e90f2c6e458c6f79a902bcd4b5fb6db2cbcc1e58"},
							{Name: "", Image: "registry.redhat.io/openshift4/ose-local-storage-operator-bundle@sha256:b0f47e35fbd4e8d0053a28a79d8d59d92458d14f31e9d7a1ffbd71416da7a660"},
							{Name: "ose-local-storage-operator", Image: "registry.redhat.io/openshift4/ose-local-storage-operator@sha256:cf52453c4eecb4a85f6dd121b7019ca7f886fcd7e21a8a5d4f66a03f18221f90"},
						},
						Properties: []property.Property{
							property.MustBuildGVKRequired("local.storage.openshift.io", "v1alpha1", "LocalVolumeSet"),
							property.MustBuildPackage("local-storage-operator", "4.12.0-202305262042"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "odf-operator.v4.12.4-rhodf",
						Package: "odf-operator",
						Image:   "registry.redhat.io/odf4/odf-operator-bundle@sha256:5ccc9c304ce9aa903bafcbfc6013f6608eb1311724a89da4fb9b674d713c4c6c",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "odf-console", Image: "registry.redhat.io/odf4/odf-console-rhel8@sha256:097a4814ec5fa79124bc30222a02bac788e94d6324ab11c2fb2aadb8a848028c"},
							{Name: "", Image: "registry.redhat.io/odf4/odf-operator-bundle@sha256:5ccc9c304ce9aa903bafcbfc6013f6608eb1311724a89da4fb9b674d713c4c6c"},
							{Name: "odf-operator", Image: "registry.redhat.io/odf4/odf-rhel8-operator@sha256:f419962667078f7c86ea5728f9fa3bb33a55ad031f87bd476cbb575f5159bf94"},
							{Name: "rbac-proxy", Image: "registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:116dbeb371324607d361d4f1a9daeb5890e44d29b96c9800b2b1d93d21635dec"},
						},
						Properties: []property.Property{
							property.MustBuildGVKRequired("odf.openshift.io", "v1alpha1", "StorageSystem"),
							property.MustBuildPackage("odf-operator", "4.12.4-rhodf"),
							property.MustBuildPackageRequired("odf-csi-addons-operator", ">=4.9.0 <=4.12.4"),
							property.MustBuildPackageRequired("ocs-operator", ">=4.9.0 <=4.12.4"),
							property.MustBuildPackageRequired("mcg-operator", ">=4.9.0 <=4.12.4"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "rhsso-operator.7.6.4-opr-002",
						Package: "rhsso-operator",
						Image:   "registry.redhat.io/rh-sso-7/sso7-rhel8-operator-bundle@sha256:0678aad21b12b433e29d8a7e6280e7b2ea2c0251a3e27a60d32ba6c877a7051d",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "keycloak_init_container", Image: "registry.redhat.io/rh-sso-7/sso7-rhel8-init-container@sha256:342e1875e3d041c388ccd1ae0fc59bb890bc3ad443af04cac2cf3477f69725fa"},
							{Name: "rhsso_init_container", Image: "registry.redhat.io/rh-sso-7/sso7-rhel8-init-container@sha256:342e1875e3d041c388ccd1ae0fc59bb890bc3ad443af04cac2cf3477f69725fa"},
							{Name: "", Image: "registry.redhat.io/rh-sso-7/sso7-rhel8-operator-bundle@sha256:0678aad21b12b433e29d8a7e6280e7b2ea2c0251a3e27a60d32ba6c877a7051d"},
							{Name: "sso7-rhel8-operator-6c276c81a823c390f78e842f06225738d531e6f54a51282384a82fb3ebd2f356-annotation", Image: "registry.redhat.io/rh-sso-7/sso7-rhel8-operator@sha256:6c276c81a823c390f78e842f06225738d531e6f54a51282384a82fb3ebd2f356"},
						},
						Properties: []property.Property{
							property.MustBuildGVKRequired("keycloak.org", "v1alpha1", "Keycloak"),
							property.MustBuildGVKRequired("keycloak.org", "v1alpha1", "KeycloakClient"),
							property.MustBuildGVKRequired("keycloak.org", "v1alpha1", "KeycloakBackup"),
							property.MustBuildGVKRequired("keycloak.org", "v1alpha1", "KeycloakRealm"),
							property.MustBuildGVKRequired("keycloak.org", "v1alpha1", "KeycloakUser"),
							property.MustBuildPackage("rhsso-operator", "7.6.4-opr-002"),
						},
					},
				},
			},

			strategy: &packageStrategy{
				curr: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "cincinnati-operator",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name:          "v1",
									IncludeBundle: v1alpha2.IncludeBundle{MinVersion: "5.0.1", MaxVersion: "5.0.1", MinBundle: ""},
								},
							},
						},
						{
							Name: "local-storage-operator",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name:          "stable",
									IncludeBundle: v1alpha2.IncludeBundle{MinVersion: "4.12.0-202305262042", MaxVersion: "4.12.0-202305262042", MinBundle: ""},
								},
							},
						},
						{
							Name: "odf-operator",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name:          "stable-4.12",
									IncludeBundle: v1alpha2.IncludeBundle{MinVersion: "4.12.4-rhodf", MaxVersion: "4.12.4-rhodf", MinBundle: ""},
								},
							},
						},
						{
							Name: "rhsso-operator",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name:          "stable",
									IncludeBundle: v1alpha2.IncludeBundle{MinVersion: "7.6.4-opr-002", MaxVersion: "7.6.4-opr-002", MinBundle: ""},
								},
							},
						},
					},
				},
			},
			in: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "cincinnati-operator",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "v1",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "5.0.1",
									MaxVersion: "5.0.1",
									MinBundle:  "",
								},
							},
						},
					},
				},
			},
			expErr: "",
			exp: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "cincinnati-operator",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "v1",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "5.0.1",
									MaxVersion: "5.0.1",
									MinBundle:  "",
								},
							},
						},
					},
					{
						Name: "local-storage-operator",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "4.12.0-202305262042",
									MaxVersion: "4.12.0-202305262042",
									MinBundle:  "",
								},
							},
						},
					},
					{
						Name: "odf-operator",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable-4.12",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "4.12.4-rhodf",
									MaxVersion: "4.12.4-rhodf",
									MinBundle:  "",
								},
							},
						},
					},
					{
						Name: "rhsso-operator",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "7.6.4-opr-002",
									MaxVersion: "7.6.4-opr-002",
									MinBundle:  "",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Success/OCPBUGS-21865/filteringByChannelMinOnly",
			cfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{
						Schema:         "olm.package",
						Name:           "cincinnati-operator",
						DefaultChannel: "v1",
					},
					{
						Schema:         "olm.package",
						Name:           "local-storage-operator",
						DefaultChannel: "stable",
					},
					{
						Schema:         "olm.package",
						Name:           "odf-operator",
						DefaultChannel: "stable-4.12",
					},
					{
						Schema:         "olm.package",
						Name:           "rhsso-operator",
						DefaultChannel: "stable",
					},
				},
				Channels: []declcfg.Channel{
					{
						Schema:  "olm.channel",
						Name:    "v1",
						Package: "cincinnati-operator",
						Entries: []declcfg.ChannelEntry{
							{Name: "update-service-operator.v5.0.1", Replaces: "update-service-operator.v4.9.1", Skips: []string{"update-service-operator.v5.0.0"}, SkipRange: ""},
						},
					},
					{
						Schema:  "olm.channel",
						Name:    "stable",
						Package: "local-storage-operator",
						Entries: []declcfg.ChannelEntry{
							{Name: "local-storage-operator.v4.12.0-202305262042", Replaces: "", Skips: []string{}, SkipRange: ">=4.3.0 <4.12.0-202305262042"},
						},
					},
					{
						Schema:  "olm.channel",
						Name:    "stable-4.12",
						Package: "odf-operator",
						Entries: []declcfg.ChannelEntry{
							{Name: "odf-operator.v4.12.4-rhodf", Replaces: "odf-operator.v4.12.3-rhodf", Skips: []string{}, SkipRange: ">=4.2.0 <4.12.4-rhodf"},
						},
					},
					{
						Schema:  "olm.channel",
						Name:    "stable",
						Package: "rhsso-operator",
						Entries: []declcfg.ChannelEntry{
							{Name: "rhsso-operator.7.6.4-opr-002", Replaces: "rhsso-operator.7.6.4-opr-001", Skips: []string{"rhsso-operator.7.6.0-opr-001"}, SkipRange: ""},
						},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  "olm.bundle",
						Name:    "update-service-operator.v5.0.1",
						Package: "cincinnati-operator",
						Image:   "registry.redhat.io/openshift-update-service/cincinnati-operator-bundle@sha256:15f80efb399fc33a80e9979df4e8044501737eee335a2a5034c8de98957a87b9",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "", Image: "registry.redhat.io/openshift-update-service/cincinnati-operator-bundle@sha256:15f80efb399fc33a80e9979df4e8044501737eee335a2a5034c8de98957a87b9"},
							{Name: "updateservice-operator", Image: "registry.redhat.io/openshift-update-service/openshift-update-service-rhel8-operator@sha256:8e9553e1041cd685239ef131e5b1b8797e3c033f008a122770a1ebfa01670d89"},
							{Name: "operand", Image: "registry.redhat.io/openshift-update-service/openshift-update-service-rhel8@sha256:fefab6832db6353886ed34aad76d1698532a1927160b8f8d34b3491c6025bae8"},
						},
						Properties: []property.Property{
							property.MustBuildGVKRequired("updateservice.operator.openshift.io", "v1", "UpdateService"),
							property.MustBuildPackage("cincinnati-operator", "5.0.1"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "local-storage-operator.v4.12.0-202305262042",
						Package: "local-storage-operator",
						Image:   "registry.redhat.io/openshift4/ose-local-storage-operator-bundle@sha256:b0f47e35fbd4e8d0053a28a79d8d59d92458d14f31e9d7a1ffbd71416da7a660",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "ose-kube-rbac-proxy", Image: "registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:9b60df6bdda991189abc6f164db78479ae7dc7127c747a5cf8fa566a910b264d"},
							{Name: "", Image: "registry.redhat.io/openshift4/ose-local-storage-operator-bundle@sha256:b0f47e35fbd4e8d0053a28a79d8d59d92458d14f31e9d7a1ffbd71416da7a660"},
							{Name: "ose-local-storage-diskmaker", Image: "registry.redhat.io/openshift4/ose-local-storage-diskmaker@sha256:815f2401b118ff0c14b87398e90f2c6e458c6f79a902bcd4b5fb6db2cbcc1e58"},
							{Name: "", Image: "registry.redhat.io/openshift4/ose-local-storage-operator-bundle@sha256:b0f47e35fbd4e8d0053a28a79d8d59d92458d14f31e9d7a1ffbd71416da7a660"},
							{Name: "ose-local-storage-operator", Image: "registry.redhat.io/openshift4/ose-local-storage-operator@sha256:cf52453c4eecb4a85f6dd121b7019ca7f886fcd7e21a8a5d4f66a03f18221f90"},
						},
						Properties: []property.Property{
							property.MustBuildGVKRequired("local.storage.openshift.io", "v1alpha1", "LocalVolumeSet"),
							property.MustBuildPackage("local-storage-operator", "4.12.0-202305262042"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "odf-operator.v4.12.4-rhodf",
						Package: "odf-operator",
						Image:   "registry.redhat.io/odf4/odf-operator-bundle@sha256:5ccc9c304ce9aa903bafcbfc6013f6608eb1311724a89da4fb9b674d713c4c6c",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "odf-console", Image: "registry.redhat.io/odf4/odf-console-rhel8@sha256:097a4814ec5fa79124bc30222a02bac788e94d6324ab11c2fb2aadb8a848028c"},
							{Name: "", Image: "registry.redhat.io/odf4/odf-operator-bundle@sha256:5ccc9c304ce9aa903bafcbfc6013f6608eb1311724a89da4fb9b674d713c4c6c"},
							{Name: "odf-operator", Image: "registry.redhat.io/odf4/odf-rhel8-operator@sha256:f419962667078f7c86ea5728f9fa3bb33a55ad031f87bd476cbb575f5159bf94"},
							{Name: "rbac-proxy", Image: "registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:116dbeb371324607d361d4f1a9daeb5890e44d29b96c9800b2b1d93d21635dec"},
						},
						Properties: []property.Property{
							property.MustBuildGVKRequired("odf.openshift.io", "v1alpha1", "StorageSystem"),
							property.MustBuildPackage("odf-operator", "4.12.4-rhodf"),
							property.MustBuildPackageRequired("odf-csi-addons-operator", ">=4.9.0 <=4.12.4"),
							property.MustBuildPackageRequired("ocs-operator", ">=4.9.0 <=4.12.4"),
							property.MustBuildPackageRequired("mcg-operator", ">=4.9.0 <=4.12.4"),
						},
					},
					{
						Schema:  "olm.bundle",
						Name:    "rhsso-operator.7.6.4-opr-002",
						Package: "rhsso-operator",
						Image:   "registry.redhat.io/rh-sso-7/sso7-rhel8-operator-bundle@sha256:0678aad21b12b433e29d8a7e6280e7b2ea2c0251a3e27a60d32ba6c877a7051d",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "keycloak_init_container", Image: "registry.redhat.io/rh-sso-7/sso7-rhel8-init-container@sha256:342e1875e3d041c388ccd1ae0fc59bb890bc3ad443af04cac2cf3477f69725fa"},
							{Name: "rhsso_init_container", Image: "registry.redhat.io/rh-sso-7/sso7-rhel8-init-container@sha256:342e1875e3d041c388ccd1ae0fc59bb890bc3ad443af04cac2cf3477f69725fa"},
							{Name: "", Image: "registry.redhat.io/rh-sso-7/sso7-rhel8-operator-bundle@sha256:0678aad21b12b433e29d8a7e6280e7b2ea2c0251a3e27a60d32ba6c877a7051d"},
							{Name: "sso7-rhel8-operator-6c276c81a823c390f78e842f06225738d531e6f54a51282384a82fb3ebd2f356-annotation", Image: "registry.redhat.io/rh-sso-7/sso7-rhel8-operator@sha256:6c276c81a823c390f78e842f06225738d531e6f54a51282384a82fb3ebd2f356"},
						},
						Properties: []property.Property{
							property.MustBuildGVKRequired("keycloak.org", "v1alpha1", "Keycloak"),
							property.MustBuildGVKRequired("keycloak.org", "v1alpha1", "KeycloakClient"),
							property.MustBuildGVKRequired("keycloak.org", "v1alpha1", "KeycloakBackup"),
							property.MustBuildGVKRequired("keycloak.org", "v1alpha1", "KeycloakRealm"),
							property.MustBuildGVKRequired("keycloak.org", "v1alpha1", "KeycloakUser"),
							property.MustBuildPackage("rhsso-operator", "7.6.4-opr-002"),
						},
					},
				},
			},

			strategy: &packageStrategy{
				curr: v1alpha2.IncludeConfig{
					Packages: []v1alpha2.IncludePackage{
						{
							Name: "cincinnati-operator",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name:          "v1",
									IncludeBundle: v1alpha2.IncludeBundle{MinVersion: "5.0.1", MaxVersion: "5.0.1", MinBundle: ""},
								},
							},
						},
						{
							Name: "local-storage-operator",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name:          "stable",
									IncludeBundle: v1alpha2.IncludeBundle{MinVersion: "4.12.0-202305262042", MaxVersion: "", MinBundle: ""},
								},
							},
						},
						{
							Name: "odf-operator",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name:          "stable-4.12",
									IncludeBundle: v1alpha2.IncludeBundle{MinVersion: "4.12.4-rhodf", MaxVersion: "", MinBundle: ""},
								},
							},
						},
						{
							Name: "rhsso-operator",
							Channels: []v1alpha2.IncludeChannel{
								{
									Name:          "stable",
									IncludeBundle: v1alpha2.IncludeBundle{MinVersion: "7.6.4-opr-002", MaxVersion: "", MinBundle: ""},
								},
							},
						},
					},
				},
			},
			in: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "cincinnati-operator",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "v1",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "5.0.1",
									MaxVersion: "5.0.1",
									MinBundle:  "",
								},
							},
						},
					},
				},
			},
			expErr: "",
			exp: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "cincinnati-operator",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "v1",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "5.0.1",
									MaxVersion: "5.0.1",
									MinBundle:  "",
								},
							},
						},
					},
					{
						Name: "local-storage-operator",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "4.12.0-202305262042",
									MaxVersion: "",
									MinBundle:  "",
								},
							},
						},
					},
					{
						Name: "odf-operator",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable-4.12",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "4.12.4-rhodf",
									MaxVersion: "",
									MinBundle:  "",
								},
							},
						},
					},
					{
						Name: "rhsso-operator",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "7.6.4-opr-002",
									MaxVersion: "",
									MinBundle:  "",
								},
							},
						},
					},
				},
			},
		},
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
