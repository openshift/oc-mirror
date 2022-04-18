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
	// MinVersion to include, plus all versions in the upgrade graph to the MaxVersion.
	MinVersion string `json:"minVersion,omitempty" yaml:"minVersion,omitempty"`
	// MaxVersion to include as the channel head version.
	MaxVersion string `json:"maxVersion,omitempty" yaml:"maxVersion,omitempty"`
	// MinBundle to include, plus all bundles in the upgrade graph to the MaxBundle.
	// Set this field only if the named bundle has no semantic version metadata.
	MinBundle string `json:"minBundle,omitempty" yaml:"minBundle,omitempty"`
	// MaxBundle to include as the channel head bundle.
	// Set this field only if the named bundle has no semantic version metadata.
	MaxBundle string `json:"maxBundle,omitempty" yaml:"maxBundle,omitempty"`
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
		case pkg.MinVersion != "":
			minVer, err := semver.Parse(pkg.MinVersion)
			if err != nil {
				return dic, fmt.Errorf("package %s: %v", pkg.Name, err)
			}
			dpkg.Versions = []semver.Version{minVer}
		case pkg.MinBundle != "":
			dpkg.Bundles = []string{pkg.MinBundle}
		}

		for chIdx, ch := range pkg.Channels {
			if ch.Name == "" {
				return dic, fmt.Errorf("package %s: channel %d requires a name", pkg.Name, chIdx)
			}
			if err := ch.IncludeBundle.validate(); err != nil {
				return dic, fmt.Errorf("channel %s: %v", ch.Name, err)
			}

			dch := action.DiffIncludeChannel{Name: ch.Name}
			switch {
			case ch.MinVersion != "":
				minVer, err := semver.Parse(ch.MinVersion)
				if err != nil {
					return dic, fmt.Errorf("channel %s: %v", ch.Name, err)
				}
				dch.Versions = []semver.Version{minVer}
			case ch.MinBundle != "":
				dch.Bundles = []string{ch.MinBundle}
			}
			dpkg.Channels = append(dpkg.Channels, dch)
		}
		dic.Packages = append(dic.Packages, dpkg)
	}

	return dic, nil
}

func (b IncludeBundle) validate() error {
	if b.MinVersion != "" && b.MinBundle != "" {
		return fmt.Errorf("starting version and bundle are mutually exclusive")
	}
	return nil
}
