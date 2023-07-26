package v1alpha2

import (
	"encoding/gob"
	"fmt"
	"io"
)

// IncludeConfig defines a list of packages for
// operator version selection.
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

// IncludeBundle contains a name (required) and versions (optional) to
// include in the diff. The full package or channel is only included if no
// versions are specified.
type IncludeBundle struct {
	// MinVersion to include, plus all versions in the upgrade graph to the MaxVersion.
	MinVersion string `json:"minVersion,omitempty" yaml:"minVersion,omitempty"`
	// MaxVersion to include as the channel head version.
	MaxVersion string `json:"maxVersion,omitempty" yaml:"maxVersion,omitempty"`
	// MinBundle to include, plus all bundles in the upgrade graph to the channel head.
	// Set this field only if the named bundle has no semantic version metadata.
	MinBundle string `json:"minBundle,omitempty" yaml:"minBundle,omitempty"`
}

/*
// ConvertToDiffIncludeConfig converts an IncludeConfig to a DiffIncludeConfig type to
// interact with `operator-registry` libraries.
func (ic *IncludeConfig) ConvertToDiffIncludeConfig() (dic diff.DiffIncludeConfig, err error) {
	if ic == nil || len(ic.Packages) == 0 {
		return dic, nil
	}

	for pkgIdx, pkg := range ic.Packages {
		if pkg.Name == "" {
			return dic, fmt.Errorf("package %d requires a name", pkgIdx)
		}
		if err := pkg.IncludeBundle.validate(); err != nil {
			return dic, fmt.Errorf("package %s: %v", pkg.Name, err)
		}

		dpkg := diff.DiffIncludePackage{Name: pkg.Name}
		switch {
		case pkg.MinVersion != "" && pkg.MaxVersion != "":
			dpkg.Range = fmt.Sprintf(">=%s <=%s", pkg.MinVersion, pkg.MaxVersion)
		case pkg.MinVersion != "":
			minVer, err := semver.Parse(pkg.MinVersion)
			if err != nil {
				return dic, fmt.Errorf("package %s: %v", pkg.Name, err)
			}
			dpkg.Versions = []semver.Version{minVer}
		case pkg.MaxVersion != "":
			dpkg.Range = fmt.Sprintf("<=%s", pkg.MaxVersion)
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

			dch := diff.DiffIncludeChannel{Name: ch.Name}
			switch {
			case ch.MinVersion != "" && ch.MaxVersion != "":
				dch.Range = fmt.Sprintf(">=%s <=%s", ch.MinVersion, ch.MaxVersion)
			case ch.MinVersion != "":
				ver, err := semver.Parse(ch.MinVersion)
				if err != nil {
					return dic, fmt.Errorf("channel %s: %v", ch.Name, err)
				}
				dch.Versions = []semver.Version{ver}
			case ch.MaxVersion != "":
				dch.Range = fmt.Sprintf("<=%s", ch.MaxVersion)
			case ch.MinBundle != "":
				dch.Bundles = []string{ch.MinBundle}
			}
			dpkg.Channels = append(dpkg.Channels, dch)
		}
		dic.Packages = append(dic.Packages, dpkg)
	}

	return dic, nil
}
*/

// Encode IncludeConfig in an efficient, opaque format.
func (ic *IncludeConfig) Encode(w io.Writer) error {
	enc := gob.NewEncoder(w)
	if err := enc.Encode(ic); err != nil {
		return fmt.Errorf("error encoding include config: %v", err)
	}
	return nil
}

// Decode IncludeConfig from an opaque format. Only usable if Include Config
// was encoded with Encode().
func (ic *IncludeConfig) Decode(r io.Reader) error {
	dec := gob.NewDecoder(r)
	if err := dec.Decode(ic); err != nil {
		return fmt.Errorf("error decoding include config: %v", err)
	}
	return nil
}

/*
func (b IncludeBundle) validate() error {
	if b.MinVersion != "" && b.MinBundle != "" {
		return fmt.Errorf("minimum version and bundle are mutually exclusive")
	}
	return nil
}
*/
