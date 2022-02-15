package v1alpha2

import (
	"fmt"

	"github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/action"
)

type IncludeConfig struct {
	// Packages to include.
	Packages []IncludePackage `json:"packages" yaml:"packages"`
}

// IncludePackage contains a name (required) and channels and/or versions
// (optional) to include in the diff. The full package is only included if no channels
// or versions are specified.
type IncludePackage struct {
	// Name of package.
	Name string `json:"name" yaml:"name"`
	// Channels to include.
	Channels []IncludeChannel `json:"channels,omitempty" yaml:"channels,omitempty"`

	// All channels containing these bundles are parsed for an upgrade graph.
	IncludeBundle `json:",inline"`
}

// IncludeChannel contains a name (required) and versions (optional)
// to include in the diff. The full channel is only included if no versions are specified.
type IncludeChannel struct {
	// Name of channel.
	Name string `json:"name" yaml:"name"`

	IncludeBundle `json:",inline"`
}

type IncludeBundle struct {
	// StartingVersion to include, plus all versions in the upgrade graph to the channel head.
	StartingVersion semver.Version `json:"startingVersion,omitempty" yaml:"startingVersion,omitempty"`
	// StartingBundle to include, plus all bundles in the upgrade graph to the channel head.
	// Set this field only if the named bundle has no semantic version metadata.
	StartingBundle string `json:"startingBundle,omitempty" yaml:"startingBundle,omitempty"`
}

func (ic *IncludeConfig) ConvertToDiffIncludeConfig() (dic action.DiffIncludeConfig, err error) {
	if ic == nil {
		return dic, nil
	}

	for pkgIdx, pkg := range ic.Packages {
		if pkg.Name == "" {
			return dic, fmt.Errorf("package %d requires a name", pkgIdx)
		}
		if err := pkg.IncludeBundle.validate(); err != nil {
			return dic, fmt.Errorf("package %s: %v", pkg.Name, err)
		}

		dpkg := action.DiffIncludePackage{Name: pkg.Name}
		switch {
		case !pkg.StartingVersion.EQ(semver.Version{}):
			dpkg.Versions = []semver.Version{pkg.StartingVersion}
		case pkg.StartingBundle != "":
			dpkg.Bundles = []string{pkg.StartingBundle}
		}
		dic.Packages = append(dic.Packages, dpkg)

		for chIdx, ch := range pkg.Channels {
			if ch.Name == "" {
				return dic, fmt.Errorf("package %s: channel %d requires a name", pkg.Name, chIdx)
			}
			if err := ch.IncludeBundle.validate(); err != nil {
				return dic, fmt.Errorf("channel %s: %v", ch.Name, err)
			}

			dch := action.DiffIncludeChannel{Name: ch.Name}
			switch {
			case !ch.StartingVersion.EQ(semver.Version{}):
				dch.Versions = []semver.Version{ch.StartingVersion}
			case ch.StartingBundle != "":
				dch.Bundles = []string{ch.StartingBundle}
			}
			dpkg.Channels = append(dpkg.Channels, dch)
		}
	}

	return dic, nil
}

func (b IncludeBundle) validate() error {
	if !b.StartingVersion.EQ(semver.Version{}) && b.StartingBundle != "" {
		return fmt.Errorf("starting version and bundle are mutually exclusive")
	}
	return nil
}
