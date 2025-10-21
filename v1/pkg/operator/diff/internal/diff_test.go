package internal

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/stretchr/testify/require"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/model"
	"github.com/operator-framework/operator-registry/alpha/property"
)

type deprecated struct{}

const deprecatedType = "olm.deprecated"

func assertCfg(t *testing.T, outputCfg declcfg.DeclarativeConfig, expCfg declcfg.DeclarativeConfig) {
	require.EqualValues(t, expCfg.Bundles, outputCfg.Bundles)
	require.EqualValues(t, expCfg.Packages, outputCfg.Packages)
	require.EqualValues(t, expCfg.Others, outputCfg.Others)
	require.Len(t, outputCfg.Channels, len(expCfg.Channels))
	for i, expectedChannel := range expCfg.Channels {
		require.Equal(t, expectedChannel.Name, outputCfg.Channels[i].Name)
		require.Equal(t, expectedChannel.Package, outputCfg.Channels[i].Package)
		require.Equal(t, expectedChannel.Schema, outputCfg.Channels[i].Schema)
		require.Equal(t, expectedChannel.Entries, outputCfg.Channels[i].Entries)
		require.Len(t, outputCfg.Channels[i].Properties, len(expectedChannel.Properties))
		for j, expectedProperty := range expectedChannel.Properties {
			require.Equal(t, expectedProperty.Type, outputCfg.Channels[i].Properties[j].Type)
			require.JSONEq(t, string(expectedProperty.Value), string(outputCfg.Channels[i].Properties[j].Value))
		}
	}
}

func init() {
	property.AddToScheme(deprecatedType, &deprecated{})
}

func TestDiffLatest(t *testing.T) {
	type spec struct {
		name      string
		g         *DiffGenerator
		oldCfg    declcfg.DeclarativeConfig
		newCfg    declcfg.DeclarativeConfig
		expCfg    declcfg.DeclarativeConfig
		assertion require.ErrorAssertionFunc
	}

	specs := []spec{
		{
			name:   "NoDiff/Empty",
			oldCfg: declcfg.DeclarativeConfig{},
			newCfg: declcfg.DeclarativeConfig{},
			g:      &DiffGenerator{},
			expCfg: declcfg.DeclarativeConfig{},
		},
		{
			name: "NoDiff/OneEqualBundle",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
			g:      &DiffGenerator{},
			expCfg: declcfg.DeclarativeConfig{},
		},
		{
			name: "NoDiff/UnsortedBundleProps",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
			g:      &DiffGenerator{},
			expCfg: declcfg.DeclarativeConfig{},
		},
		{
			name:   "HasDiff/EmptyChannel",
			oldCfg: declcfg.DeclarativeConfig{},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "v1", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
				},
			},
			g: &DiffGenerator{
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{
							Name: "foo",
							AllChannels: DiffIncludeChannel{
								Versions: []semver.Version{semver.MustParse("0.2.0")},
							},
						},
					},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/OneModifiedBundle",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("bar", ">=1.0.0"),
						},
					},
				},
			},
			g: &DiffGenerator{},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("bar", ">=1.0.0"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/ManyBundlesAndChannels",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "fast", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0-alpha.0"},
						{Name: "foo.v0.2.0-alpha.1", Replaces: "foo.v0.2.0-alpha.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0-alpha.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0-alpha.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0-alpha.1",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0-alpha.1"),
						},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
						{Name: "foo.v0.2.0", Skips: []string{"foo.v0.1.0"}},
					}},
					{Schema: declcfg.SchemaChannel, Name: "fast", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0-alpha.0"},
						{Name: "foo.v0.2.0-alpha.1", Replaces: "foo.v0.2.0-alpha.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "clusterwide", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0-clusterwide"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuild(&deprecated{}),
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0-alpha.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0-alpha.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0-alpha.1",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0-alpha.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0-clusterwide",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0-clusterwide"),
						},
					},
				},
			},
			g: &DiffGenerator{},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "clusterwide", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0-clusterwide"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
						{Name: "foo.v0.2.0", Skips: []string{"foo.v0.1.0"}},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuild(&deprecated{}),
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0-clusterwide",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0-clusterwide"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/OldBundleUpdatedDependencyRange",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("etcd", ">=0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
				},
			},
			g: &DiffGenerator{},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("etcd", ">=0.9.0"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/BundleNewDependencyRange",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("etcd", ">=0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
				},
			},
			g: &DiffGenerator{},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("etcd", ">=0.9.0"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/NewBundleNewDependencyRange",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 2),
					}},
					{Schema: declcfg.SchemaChannel, Name: "clusterwide", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0-clusterwide"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("clusterwide", 1),
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 1),
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0-clusterwide",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0-clusterwide"),
							property.MustBuildPackageRequired("etcd", ">=0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
				},
			},
			g: &DiffGenerator{},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "clusterwide"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 1),
					}},
					{Schema: declcfg.SchemaChannel, Name: "clusterwide", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0-clusterwide"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("clusterwide", 1),
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0-clusterwide",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0-clusterwide"),
							property.MustBuildPackageRequired("etcd", ">=0.9.0"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/OneNewDependencyRange",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("etcd", ">=0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("etcd", ">=0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
				},
			},
			g: &DiffGenerator{},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/TwoDependencyRanges",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
						{Name: "foo.v0.2.0", Replaces: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("etcd", ">=0.9.0 <0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
							property.MustBuildPackageRequired("etcd", ">=0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
				},
			},
			g: &DiffGenerator{},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
						{Name: "foo.v0.2.0", Replaces: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("etcd", ">=0.9.0 <0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
							property.MustBuildPackageRequired("etcd", ">=0.9.2"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/BundleNewDependencyGVK",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
				},
			},
			g: &DiffGenerator{},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/IncludePackage",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{{Name: "foo.v0.1.0"}}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{{Name: "bar.v0.1.0"}}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "bar.v0.1.0", Package: "bar", Image: "reg/bar:latest",
						Properties: []property.Property{property.MustBuildPackage("bar", "0.1.0")},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{{Name: "foo.v0.1.0"}}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"}, {Name: "bar.v0.2.0", Replaces: "bar.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "bar.v0.1.0", Package: "bar", Image: "reg/bar:latest",
						Properties: []property.Property{property.MustBuildPackage("bar", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "bar.v0.2.0", Package: "bar", Image: "reg/bar:latest",
						Properties: []property.Property{property.MustBuildPackage("bar", "0.2.0")},
					},
				},
			},
			g: &DiffGenerator{
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{{Name: "bar"}},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.2.0", Replaces: "bar.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "bar.v0.2.0", Package: "bar", Image: "reg/bar:latest",
						Properties: []property.Property{property.MustBuildPackage("bar", "0.2.0")},
					},
				},
			},
		},
		{
			name: "HasDiff/IncludeChannel",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{{Name: "foo.v0.1.0"}}},
					{Schema: declcfg.SchemaChannel, Name: "alpha", Package: "foo", Entries: []declcfg.ChannelEntry{{Name: "foo.v0.1.0-alpha.0"}}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0-alpha.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0-alpha.0")},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "alpha"}, // Make sure the default channel is still updated.
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"}, {Name: "foo.v0.2.0", Replaces: "foo.v0.1.0"}}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 1),
					},
					},
					{Schema: declcfg.SchemaChannel, Name: "alpha", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0-alpha.0"}, {Name: "foo.v0.2.0-alpha.0", Replaces: "foo.v0.1.0-alpha.0"}}, Properties: []property.Property{
						property.MustBuildChannelPriority("alpha", 2),
					},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.2.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.2.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0-alpha.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0-alpha.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.2.0-alpha.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.2.0-alpha.0")},
					},
				},
			},
			g: &DiffGenerator{
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{{Name: "foo", Channels: []DiffIncludeChannel{{Name: "stable"}}}},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0", Replaces: "foo.v0.1.0"}}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 1),
					},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.2.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.2.0")},
					},
				},
			},
		},
		{
			name: "HasDiff/IncludeVersion",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"}, {Name: "foo.v0.2.0", Replaces: "foo.v0.1.0"}},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.2.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.2.0")},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"}, {Name: "foo.v0.1.1", Replaces: "foo.v0.1.0"},
						{Name: "foo.v0.2.0", Replaces: "foo.v0.1.1"}, {Name: "foo.v0.3.0", Replaces: "foo.v0.2.0"}},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.1", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.1")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.2.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.2.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.3.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.3.0")},
					},
				},
			},
			g: &DiffGenerator{
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{Name: "foo", Channels: []DiffIncludeChannel{
							{Name: "stable", Versions: []semver.Version{{Major: 0, Minor: 2, Patch: 0}}}},
						}},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.3.0", Replaces: "foo.v0.2.0"}},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.3.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.3.0")},
					},
				},
			},
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			if s.assertion == nil {
				s.assertion = require.NoError
			}

			oldModel, err := declcfg.ConvertToModel(s.oldCfg)
			require.NoError(t, err)

			newModel, err := declcfg.ConvertToModel(s.newCfg)
			require.NoError(t, err)

			outputModel, err := s.g.Run(oldModel, newModel)
			s.assertion(t, err)

			outputCfg := declcfg.ConvertFromModel(outputModel)
			assertCfg(t, outputCfg, s.expCfg)
		})
	}
}

func TestDiffHeadsOnly(t *testing.T) {
	type spec struct {
		name      string
		g         *DiffGenerator
		newCfg    declcfg.DeclarativeConfig
		expCfg    declcfg.DeclarativeConfig
		assertion require.ErrorAssertionFunc
	}

	specs := []spec{
		{
			name:   "NoDiff/Empty",
			newCfg: declcfg.DeclarativeConfig{},
			g:      &DiffGenerator{},
			expCfg: declcfg.DeclarativeConfig{},
		},
		{
			name: "NoDiff/EmptyBundleWithInclude",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "clusterwide", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1-clusterwide"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1-clusterwide",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1-clusterwide"),
						},
					},
				},
			},
			g: &DiffGenerator{
				IncludeAdditively: false,
				HeadsOnly:         true,
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{
							Name: "etcd",
							AllChannels: DiffIncludeChannel{
								Versions: []semver.Version{{Major: 0, Minor: 9, Patch: 2}},
							},
						},
					},
				},
			},
			expCfg: declcfg.DeclarativeConfig{},
		},
		{
			name: "HasDiff/EmptyChannel",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "v1", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
				},
			},
			g: &DiffGenerator{
				HeadsOnly: true,
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{
							Name: "foo",
							AllChannels: DiffIncludeChannel{
								Versions: []semver.Version{semver.MustParse("0.2.0")},
							},
						},
					},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/OneBundle",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
			g: &DiffGenerator{
				HeadsOnly: true,
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
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
			name: "HasDiff/Graph",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "clusterwide", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1-clusterwide"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
						{Name: "foo.v0.2.0", Replaces: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "alpha", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0-alpha.0"},
						{Name: "foo.v0.2.0-alpha.1", Replaces: "foo.v0.2.0-alpha.0"},
						{Name: "foo.v0.2.0", Replaces: "foo.v0.2.0-alpha.1"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0-alpha.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0-alpha.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0-alpha.1",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0-alpha.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1-clusterwide",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1-clusterwide"),
						},
					},
				},
			},
			g: &DiffGenerator{
				HeadsOnly: true,
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "clusterwide", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1-clusterwide"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "alpha", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0", Replaces: "foo.v0.2.0-alpha.1"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0", Replaces: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1-clusterwide",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1-clusterwide"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/CyclicDependencyGraph",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v4.9.3"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v4.9.3"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v4.9.3",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildGVK("foo", "v1alpha1", "Foo"),
							property.MustBuildGVKRequired("bar", "v1alpha1", "Bar"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "bar.v4.9.3",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildPackage("bar", "4.9.3"),
							property.MustBuildGVK("bar", "v1alpha1", "Bar"),
							property.MustBuildGVKRequired("foo", "v1alpha1", "Foo"),
						},
					},
				},
			},
			g: &DiffGenerator{
				HeadsOnly: true,
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v4.9.3"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v4.9.3"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "bar.v4.9.3",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVK("bar", "v1alpha1", "Bar"),
							property.MustBuildGVKRequired("foo", "v1alpha1", "Foo"),
							property.MustBuildPackage("bar", "4.9.3"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v4.9.3",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildGVK("foo", "v1alpha1", "Foo"),
							property.MustBuildGVKRequired("bar", "v1alpha1", "Bar"),
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
				},
			},
		},
		{
			// Testing SkipDependencies only really makes sense in heads-only mode,
			// since new dependencies are always added.
			name: "HasDiff/SkipDependencies",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackageRequired("etcd", "<=0.9.1"),
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
				},
			},
			g: &DiffGenerator{
				SkipDependencies: true,
				HeadsOnly:        true,
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("etcd", "<=0.9.1"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/SelectDependencies",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.3", Replaces: "etcd.v0.9.2"},
						{Name: "etcd.v1.0.0", Replaces: "etcd.v0.9.3"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackageRequired("etcd", "<0.9.2"),
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.3",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.3"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v1.0.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("etcd", "1.0.0"),
						},
					},
				},
			},
			g: &DiffGenerator{
				HeadsOnly: true,
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.3", Replaces: "etcd.v0.9.2"},
						{Name: "etcd.v1.0.0", Replaces: "etcd.v0.9.3"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.3",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.3"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v1.0.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "1.0.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("etcd", "<0.9.2"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/SelectDependencies/SkipHeadSkipsList",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.3", Replaces: "etcd.v0.9.2"},
						{Name: "etcd.v1.0.0", Replaces: "etcd.v0.9.3", Skips: []string{"etcd.v0.9.1", "etcd.v0.9.2", "etcd.v0.9.3"}},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackageRequired("etcd", "<0.9.2"),
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.3",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.3"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v1.0.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("etcd", "1.0.0"),
						},
					},
				},
			},
			g: &DiffGenerator{
				HeadsOnly: true,
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v1.0.0", Replaces: "etcd.v0.9.3", Skips: []string{"etcd.v0.9.1", "etcd.v0.9.2", "etcd.v0.9.3"}},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v1.0.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "1.0.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("etcd", "<0.9.2"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/SelectDependenciesInclude",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.3", Replaces: "etcd.v0.9.2"},
						{Name: "etcd.v1.0.0", Replaces: "etcd.v0.9.3", Skips: []string{"etcd.v0.9.1", "etcd.v0.9.2", "etcd.v0.9.3"}},
					}},
					{Schema: declcfg.SchemaChannel, Name: "alpha", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.3", Replaces: "etcd.v0.9.2"},
						{Name: "etcd.v1.0.0", Replaces: "etcd.v0.9.3", Skips: []string{"etcd.v0.9.1", "etcd.v0.9.2", "etcd.v0.9.3"}},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackageRequired("etcd", "<0.9.2"),
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.3",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.3"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v1.0.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("etcd", "1.0.0"),
						},
					},
				},
			},
			g: &DiffGenerator{
				IncludeAdditively: false,
				HeadsOnly:         true,
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{
							Name:     "bar",
							Channels: []DiffIncludeChannel{{Name: "stable"}},
						},
					},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v1.0.0", Replaces: "etcd.v0.9.3", Skips: []string{"etcd.v0.9.1", "etcd.v0.9.2", "etcd.v0.9.3"}},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v1.0.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "1.0.0"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/IncludeAdditive",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.3", Replaces: "etcd.v0.9.2"},
						{Name: "etcd.v1.0.0", Replaces: "etcd.v0.9.3", Skips: []string{"etcd.v0.9.2", "etcd.v0.9.3"}},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackageRequired("etcd", "<0.9.2"),
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.3",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.3"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v1.0.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("etcd", "1.0.0"),
						},
					},
				},
			},
			g: &DiffGenerator{
				IncludeAdditively: true,
				HeadsOnly:         true,
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{
							Name: "etcd",
							Channels: []DiffIncludeChannel{{
								Name:     "stable",
								Versions: []semver.Version{{Major: 0, Minor: 9, Patch: 2}}},
							}},
						{
							Name:     "bar",
							Channels: []DiffIncludeChannel{{Name: "stable"}},
						},
					},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.3", Replaces: "etcd.v0.9.2"},
						{Name: "etcd.v1.0.0", Replaces: "etcd.v0.9.3", Skips: []string{"etcd.v0.9.2", "etcd.v0.9.3"}},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.3",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.3"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v1.0.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "1.0.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
							property.MustBuildPackageRequired("etcd", "<0.9.2"),
						},
					},
				},
			},
		},
		{
			name: "HasDiff/IncludePackage",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{{Name: "foo.v0.1.0"}}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"}, {Name: "bar.v0.2.0", Replaces: "bar.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "bar.v0.1.0", Package: "bar", Image: "reg/bar:latest",
						Properties: []property.Property{property.MustBuildPackage("bar", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "bar.v0.2.0", Package: "bar", Image: "reg/bar:latest",
						Properties: []property.Property{property.MustBuildPackage("bar", "0.2.0")},
					},
				},
			},
			g: &DiffGenerator{
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{{Name: "bar"}},
				},
				HeadsOnly: false,
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"}, {Name: "bar.v0.2.0", Replaces: "bar.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "bar.v0.1.0", Package: "bar", Image: "reg/bar:latest",
						Properties: []property.Property{property.MustBuildPackage("bar", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "bar.v0.2.0", Package: "bar", Image: "reg/bar:latest",
						Properties: []property.Property{property.MustBuildPackage("bar", "0.2.0")},
					},
				},
			},
		},
		{
			name: "HasDiff/IncludeChannel",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "alpha"}, // Make sure the default channel is still updated.
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"}, {Name: "foo.v0.2.0", Replaces: "foo.v0.1.0"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("stable", 1),
						},
					},
					{Schema: declcfg.SchemaChannel, Name: "alpha", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0-alpha.0"}, {Name: "foo.v0.2.0-alpha.0", Replaces: "foo.v0.1.0-alpha.0"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("alpha", 2),
						},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.2.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.2.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0-alpha.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0-alpha.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.2.0-alpha.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.2.0-alpha.0")},
					},
				},
			},
			g: &DiffGenerator{
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{{Name: "foo", Channels: []DiffIncludeChannel{{Name: "stable"}}}},
				},
				HeadsOnly: false,
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"}, {Name: "foo.v0.2.0", Replaces: "foo.v0.1.0"}}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 1),
					},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.2.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.2.0")},
					},
				},
			},
		},
		{
			name: "HasDiff/IncludePackageHeadsOnly",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{{Name: "foo.v0.1.0"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("stable", 1),
						}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"}, {Name: "bar.v0.2.0", Replaces: "bar.v0.1.0"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 1),
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "bar.v0.1.0", Package: "bar", Image: "reg/bar:latest",
						Properties: []property.Property{property.MustBuildPackage("bar", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "bar.v0.2.0", Package: "bar", Image: "reg/bar:latest",
						Properties: []property.Property{property.MustBuildPackage("bar", "0.2.0")},
					},
				},
			},
			g: &DiffGenerator{
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{{Name: "bar"}},
				},
				HeadsOnly: true,
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.2.0", Replaces: "bar.v0.1.0"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 1),
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "bar.v0.2.0", Package: "bar", Image: "reg/bar:latest",
						Properties: []property.Property{property.MustBuildPackage("bar", "0.2.0")},
					},
				},
			},
		},
		{
			name: "HasDiff/IncludeChannelHeadsOnly",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "alpha"}, // Make sure the default channel is still updated.
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"}, {Name: "foo.v0.2.0", Replaces: "foo.v0.1.0"}}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 2),
					},
					},
					{Schema: declcfg.SchemaChannel, Name: "alpha", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0-alpha.0"}, {Name: "foo.v0.2.0-alpha.0", Replaces: "foo.v0.1.0-alpha.0"}}, Properties: []property.Property{
						property.MustBuildChannelPriority("alpha", 1),
					},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.2.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.2.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0-alpha.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0-alpha.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.2.0-alpha.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.2.0-alpha.0")},
					},
				},
			},
			g: &DiffGenerator{
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{{Name: "foo", Channels: []DiffIncludeChannel{{Name: "stable"}}}},
				},
				HeadsOnly: true,
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0", Replaces: "foo.v0.1.0"}}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 2),
					},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.2.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.2.0")},
					},
				},
			},
		},
		{
			name: "HasDiff/IncludeVersion",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"}, {Name: "foo.v0.1.1", Replaces: "foo.v0.1.0"},
						{Name: "foo.v0.2.0", Replaces: "foo.v0.1.1"}, {Name: "foo.v0.3.0", Replaces: "foo.v0.2.0"}}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 2),
					},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.1.1", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.1.1")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.2.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.2.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.3.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.3.0")},
					},
				},
			},
			g: &DiffGenerator{
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{Name: "foo", Channels: []DiffIncludeChannel{
							{Name: "stable", Versions: []semver.Version{{Major: 0, Minor: 2, Patch: 0}}}},
						}},
				},
				HeadsOnly: true,
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0", Replaces: "foo.v0.1.1"}, {Name: "foo.v0.3.0", Replaces: "foo.v0.2.0"}}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 2),
					},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.2.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.2.0")},
					},
					{
						Schema: declcfg.SchemaBundle,
						Name:   "foo.v0.3.0", Package: "foo", Image: "reg/foo:latest",
						Properties: []property.Property{property.MustBuildPackage("foo", "0.3.0")},
					},
				},
			},
		},
		{
			name: "HasDiff/IncludeNonAdditive",
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.3", Replaces: "etcd.v0.9.2"},
						{Name: "etcd.v1.0.0", Replaces: "etcd.v0.9.3", Skips: []string{"etcd.v0.9.1", "etcd.v0.9.2", "etcd.v0.9.3"}},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackageRequired("etcd", "<0.9.2"),
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.3",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.3"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v1.0.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("etcd", "1.0.0"),
						},
					},
				},
			},
			g: &DiffGenerator{
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{
							Name: "etcd",
							Channels: []DiffIncludeChannel{{
								Name:     "stable",
								Versions: []semver.Version{{Major: 0, Minor: 9, Patch: 3}}},
							}},
						{
							Name:     "bar",
							Channels: []DiffIncludeChannel{{Name: "stable"}},
						},
					},
				},
				HeadsOnly: true,
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "bar", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "bar", Entries: []declcfg.ChannelEntry{
						{Name: "bar.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.3", Replaces: "etcd.v0.9.2"},
						{Name: "etcd.v1.0.0", Replaces: "etcd.v0.9.3", Skips: []string{"etcd.v0.9.1", "etcd.v0.9.2", "etcd.v0.9.3"}},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "bar.v0.1.0",
						Package: "bar",
						Image:   "reg/bar:latest",
						Properties: []property.Property{
							property.MustBuildGVKRequired("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildPackage("bar", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.3",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "0.9.3"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v1.0.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildGVK("etcd.database.coreos.com", "v1", "EtcdBackup"),
							property.MustBuildGVK("etcd.database.coreos.com", "v1beta2", "EtcdBackup"),
							property.MustBuildPackage("etcd", "1.0.0"),
						},
					},
				},
			},
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			if s.assertion == nil {
				s.assertion = require.NoError
			}

			newModel, err := declcfg.ConvertToModel(s.newCfg)
			require.NoError(t, err)

			outputModel, err := s.g.Run(model.Model{}, newModel)
			s.assertion(t, err)

			outputCfg := declcfg.ConvertFromModel(outputModel)
			assertCfg(t, outputCfg, s.expCfg)
		})
	}
}

func TestDiffRange(t *testing.T) {
	type spec struct {
		name      string
		g         *DiffGenerator
		oldCfg    declcfg.DeclarativeConfig
		newCfg    declcfg.DeclarativeConfig
		expCfg    declcfg.DeclarativeConfig
		assertion require.ErrorAssertionFunc
	}

	specs := []spec{
		{
			name: "OnlyPackageRange",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "fast", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 2),
					}},
					{Schema: declcfg.SchemaChannel, Name: "fast", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0"},
						{Name: "foo.v0.3.0", Replaces: "foo.v0.2.0"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("fast", 1),
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.3", Replaces: "etcd.v0.9.2"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 1),
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.3.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.3.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.3",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.3"),
						},
					},
				},
			},
			g: &DiffGenerator{
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{
							Name:  "foo",
							Range: semver.MustParseRange("<=0.3.0"),
						},
						{
							Name:  "etcd",
							Range: semver.MustParseRange(">0.9.0 <0.9.3"),
						},
					},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "fast"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 1),
					}},
					{Schema: declcfg.SchemaChannel, Name: "fast", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.3.0", Replaces: "foo.v0.2.0"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("fast", 1),
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.3.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.3.0"),
						},
					},
				},
			},
		},
		{
			name: "OnlyChannelRange",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "fast", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.3.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.3.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.3.0"),
						},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
						{Name: "foo.v0.2.0", Replaces: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "fast", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.3.0"},
						{Name: "foo.v0.4.0", Replaces: "foo.v0.3.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.3.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.3.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.4.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.4.0"),
						},
					},
				},
			},
			g: &DiffGenerator{
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{Name: "foo", Channels: []DiffIncludeChannel{
							{Name: "stable", Range: semver.MustParseRange("0.2.0")},
							{Name: "fast", Range: semver.MustParseRange("0.4.0")},
						}},
					},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "fast", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.4.0", Replaces: "foo.v0.3.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0", Replaces: "foo.v0.1.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.4.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.4.0"),
						},
					},
				},
			},
		},
		{
			name: "CombinationPackageChannelRange/Valid",
			oldCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "fast", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
				},
			},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.1.0"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 1),
					}},
					{Schema: declcfg.SchemaChannel, Name: "fast", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.2.0"},
						{Name: "foo.v0.3.0", Replaces: "foo.v0.2.0"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("fast", 2),
					}},
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.1", Replaces: "etcd.v0.9.0"},
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.3", Replaces: "etcd.v0.9.2"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 1),
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.1.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.1.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.2.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.2.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.3.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.3.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.0",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.1",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.1"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.3",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.3"),
						},
					},
				},
			},
			g: &DiffGenerator{
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{
							Name:  "foo",
							Range: semver.MustParseRange("<=0.3.0"),
						},
						{Name: "etcd", Channels: []DiffIncludeChannel{
							{Name: "stable", Range: semver.MustParseRange(">0.9.1")},
						}},
					},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "etcd", DefaultChannel: "stable"},
					{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "fast"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "stable", Package: "etcd", Entries: []declcfg.ChannelEntry{
						{Name: "etcd.v0.9.2", Replaces: "etcd.v0.9.1"},
						{Name: "etcd.v0.9.3", Replaces: "etcd.v0.9.2"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("stable", 1),
					}},
					{Schema: declcfg.SchemaChannel, Name: "fast", Package: "foo", Entries: []declcfg.ChannelEntry{
						{Name: "foo.v0.3.0", Replaces: "foo.v0.2.0"},
					}, Properties: []property.Property{
						property.MustBuildChannelPriority("fast", 2),
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.2",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.2"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "etcd.v0.9.3",
						Package: "etcd",
						Image:   "reg/etcd:latest",
						Properties: []property.Property{
							property.MustBuildPackage("etcd", "0.9.3"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "foo.v0.3.0",
						Package: "foo",
						Image:   "reg/foo:latest",
						Properties: []property.Property{
							property.MustBuildPackage("foo", "0.3.0"),
						},
					},
				},
			},
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			if s.assertion == nil {
				s.assertion = require.NoError
			}

			oldModel, err := declcfg.ConvertToModel(s.oldCfg)
			require.NoError(t, err)

			newModel, err := declcfg.ConvertToModel(s.newCfg)
			require.NoError(t, err)

			outputModel, err := s.g.Run(oldModel, newModel)
			s.assertion(t, err)

			outputCfg := declcfg.ConvertFromModel(outputModel)
			assertCfg(t, outputCfg, s.expCfg)
		})
	}
}

func TestSetDefaultChannelRange(t *testing.T) {
	type spec struct {
		name      string
		g         *DiffGenerator
		oldCfg    declcfg.DeclarativeConfig
		newCfg    declcfg.DeclarativeConfig
		expCfg    declcfg.DeclarativeConfig
		assertion require.ErrorAssertionFunc
	}

	specs := []spec{
		{
			name:   "ibm-mq-test/Valid",
			oldCfg: declcfg.DeclarativeConfig{},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "ibm-mq", DefaultChannel: "v1.8"},
				},
				Channels: []declcfg.Channel{

					{Schema: declcfg.SchemaChannel, Name: "v1.8", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.8.1"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("v1.8", 3),
						},
					},

					{Schema: declcfg.SchemaChannel, Name: "v1.7", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.7.0"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("v1.7", 1),
						},
					},

					{Schema: declcfg.SchemaChannel, Name: "v1.6", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.6.0"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("v1.6", 2),
						},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.6.0",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.6.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.7.0",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.7.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.8.1",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.8.1"),
						},
					},
				}},
			g: &DiffGenerator{
				IncludeAdditively: false,
				HeadsOnly:         false,
				SkipDependencies:  true,
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{
							//HeadsOnly: false,

							Name:  "ibm-mq",
							Range: semver.MustParseRange("<=1.7.0"),
							//Channels: []DiffIncludeChannel{
							//	//	{
							//	//		Name:  "v1.7",
							//	//		Range: semver.MustParseRange("<=1.7.0"),
							//	//	},
							//	{
							//		Name:  "v1.6",
							//		Range: semver.MustParseRange("<=1.6.0"),
							//	},
							//},
						},
					},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "ibm-mq", DefaultChannel: "v1.6"},
				},
				Channels: []declcfg.Channel{

					{Schema: declcfg.SchemaChannel, Name: "v1.6", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.6.0"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("v1.6", 2),
						},
					},
					{Schema: declcfg.SchemaChannel, Name: "v1.7", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.7.0"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("v1.7", 1),
						},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.6.0",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.6.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.7.0",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.7.0"),
						},
					},
				},
			},
		},

		{
			name:   "ibm-mq-test/Valid/OverrideDefaultChannel",
			oldCfg: declcfg.DeclarativeConfig{},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "ibm-mq", DefaultChannel: "v1.8"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "v1.8", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.8.1"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("v1.8", 3),
						},
					},
					{Schema: declcfg.SchemaChannel, Name: "v1.7", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.7.0"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("v1.7", 1),
						},
					},
					{Schema: declcfg.SchemaChannel, Name: "v1.6", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.6.0"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("v1.6", 2),
						},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.6.0",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.6.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.7.0",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.7.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.8.1",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.8.1"),
						},
					},
				}},
			g: &DiffGenerator{
				IncludeAdditively: false,
				HeadsOnly:         false,
				SkipDependencies:  true,
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{
							Name: "ibm-mq",
							Channels: []DiffIncludeChannel{
								{
									Name: "v1.7",
								},
							},
							DefaultChannel: "v1.7",
						},
					},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "ibm-mq", DefaultChannel: "v1.7"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "v1.7", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.7.0"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("v1.7", 1),
						},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.7.0",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.7.0"),
						},
					},
				},
			},
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			if s.assertion == nil {
				s.assertion = require.NoError
			}

			newModel, err := declcfg.ConvertToModel(s.newCfg)
			require.NoError(t, err)

			outputModel, err := s.g.Run(model.Model{}, newModel)
			s.assertion(t, err)

			if err := outputModel.Validate(); err != nil {
				fmt.Println(err)
			}

			outputCfg := declcfg.ConvertFromModel(outputModel)
			assertCfg(t, outputCfg, s.expCfg)
		})
	}
}

func TestSetDefaultChannelRange2(t *testing.T) {
	type spec struct {
		name   string
		g      *DiffGenerator
		fsys   fs.FS
		oldCfg declcfg.DeclarativeConfig
		//newCfg    declcfg.DeclarativeConfig
		expCfg    declcfg.DeclarativeConfig
		assertion require.ErrorAssertionFunc
	}

	specs := []spec{
		{
			name:   "ibm-mq-test/Valid",
			oldCfg: declcfg.DeclarativeConfig{},
			fsys:   os.DirFS("./testdata/"),
			//inputFBC: "./testdata/index.json",
			g: &DiffGenerator{
				IncludeAdditively: false,
				HeadsOnly:         false,
				SkipDependencies:  true,
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{
							//HeadsOnly: false,

							Name:  "ibm-mq",
							Range: semver.MustParseRange("<=1.7.0"),
							//Channels: []DiffIncludeChannel{
							//	//	{
							//	//		Name:  "v1.7",
							//	//		Range: semver.MustParseRange("<=1.7.0"),
							//	//	},
							//	{
							//		Name:  "v1.6",
							//		Range: semver.MustParseRange("<=1.6.0"),
							//	},
							//},
						},
					},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "ibm-mq", DefaultChannel: "v1.7"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "v1.6", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.6.0", SkipRange: ">=1.0.0 <1.6.0"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("v1.6", 2),
						},
					},
					{Schema: declcfg.SchemaChannel, Name: "v1.7", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.7.0", SkipRange: ">=1.0.0 <1.7.0"}},
						Properties: []property.Property{
							property.MustBuildChannelPriority("v1.7", 3),
						},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.6.0",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.6.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.7.0",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.7.0"),
						},
					},
				},
			},
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			if s.assertion == nil {
				s.assertion = require.NoError
			}

			//oldModel, err := declcfg.ConvertToModel(s.oldCfg)
			//require.NoError(t, err)

			newCfg, err := declcfg.LoadFS(context.Background(), s.fsys)
			if err != nil {
				fmt.Println(err)
			}

			newModel, err := declcfg.ConvertToModel(*newCfg)
			require.NoError(t, err)

			outputModel, err := s.g.Run(model.Model{}, newModel)
			s.assertion(t, err)

			if err := outputModel.Validate(); err != nil {
				fmt.Println(err)
				//return nil, err
			}

			outputCfg := declcfg.ConvertFromModel(outputModel)
			assertCfg(t, outputCfg, s.expCfg)
		})
	}
}

func TestSetDefaultChannelError(t *testing.T) {
	type spec struct {
		name      string
		g         *DiffGenerator
		oldCfg    declcfg.DeclarativeConfig
		newCfg    declcfg.DeclarativeConfig
		expCfg    declcfg.DeclarativeConfig
		assertion require.ErrorAssertionFunc
	}

	specs := []spec{
		{
			name:   "ibm-mq-test/Valid",
			oldCfg: declcfg.DeclarativeConfig{},
			newCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "ibm-mq", DefaultChannel: "v1.8"},
				},
				Channels: []declcfg.Channel{

					{Schema: declcfg.SchemaChannel, Name: "v1.8", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.8.1"}},
					},

					{Schema: declcfg.SchemaChannel, Name: "v1.7", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.7.0"}},
					},

					{Schema: declcfg.SchemaChannel, Name: "v1.6", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.6.0"}},
					},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.6.0",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.6.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.7.0",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.7.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.8.1",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.8.1"),
						},
					},
				}},
			g: &DiffGenerator{
				IncludeAdditively: false,
				HeadsOnly:         false,
				SkipDependencies:  true,
				Includer: DiffIncluder{
					Packages: []DiffIncludePackage{
						{
							Name:  "ibm-mq",
							Range: semver.MustParseRange("<=1.7.0"),
						},
					},
				},
			},
			expCfg: declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{Schema: declcfg.SchemaPackage, Name: "ibm-mq", DefaultChannel: "v1.8"},
				},
				Channels: []declcfg.Channel{
					{Schema: declcfg.SchemaChannel, Name: "v1.7", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.7.0"},
					}},
					{Schema: declcfg.SchemaChannel, Name: "v1.6", Package: "ibm-mq", Entries: []declcfg.ChannelEntry{
						{Name: "ibm-mq.v1.6.0"},
					}},
				},
				Bundles: []declcfg.Bundle{
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.6.0",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.6.0"),
						},
					},
					{
						Schema:  declcfg.SchemaBundle,
						Name:    "ibm-mq.v1.7.0",
						Package: "ibm-mq",
						Image:   "reg/ibm-mq:latest",
						Properties: []property.Property{
							property.MustBuildPackage("ibm-mq", "1.7.0"),
						},
					},
				},
			},
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			if s.assertion == nil {
				s.assertion = require.NoError
			}

			newModel, err := declcfg.ConvertToModel(s.newCfg)
			require.NoError(t, err)

			_, err = s.g.Run(model.Model{}, newModel)
			require.Error(t, err)
		})
	}
}

func TestSetDefaultChannel(t *testing.T) {
	type spec struct {
		name                   string
		pkg                    *model.Package
		expectedError          bool
		expectedDefaultChannel *model.Channel
	}

	specs := []spec{
		{
			name: "WithPriorities/TwoChannels",
			pkg: &model.Package{
				Name:           "abc",
				DefaultChannel: nil,
				Channels: map[string]*model.Channel{
					"stable": {
						Name: "stable",
						Properties: []property.Property{
							property.MustBuildChannelPriority("stable", 2),
						}},
					"fast": {
						Name: "fast",
						Properties: []property.Property{
							property.MustBuildChannelPriority("fast", 1),
						}},
				},
			},
			expectedError: false,
			expectedDefaultChannel: &model.Channel{
				Name: "stable",
				Properties: []property.Property{
					property.MustBuildChannelPriority("stable", 2),
				}},
		},
		{
			name: "WithPriorities/OnlyOneChannel",
			pkg: &model.Package{
				Name:           "abc",
				DefaultChannel: nil,
				Channels: map[string]*model.Channel{
					"stable": {
						Name: "stable",
						Properties: []property.Property{
							property.MustBuildChannelPriority("stable", 2),
						}},
				},
			},
			expectedError: false,
			expectedDefaultChannel: &model.Channel{
				Name: "stable",
				Properties: []property.Property{
					property.MustBuildChannelPriority("stable", 2),
				}},
		},
		{
			name: "NoPriorities",
			pkg: &model.Package{
				Name:           "abc",
				DefaultChannel: nil,
				Channels: map[string]*model.Channel{
					"stable": {
						Name: "stable",
					},
					"fast": {
						Name: "fast",
					},
				},
			},
			expectedError:          true,
			expectedDefaultChannel: nil,
		},
		{
			name: "NoChannels",
			pkg: &model.Package{
				Name:           "abc",
				DefaultChannel: nil,
			},
			expectedError:          true,
			expectedDefaultChannel: nil,
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			pkg := s.pkg
			err := setDefaultChannel(pkg, "dummyChannelName")
			if s.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.EqualValues(t, pkg.DefaultChannel.Name, s.expectedDefaultChannel.Name)
				require.EqualValues(t, pkg.DefaultChannel.Properties, s.expectedDefaultChannel.Properties)
			}
		})
	}
}
