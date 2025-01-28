package v2alpha1

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
	Channels       []IncludeChannel `json:"channels,omitempty" yaml:"channels,omitempty"`
	DefaultChannel string           `json:"defaultChannel,omitempty"`

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
	// MinBundle string `json:"minBundle,omitempty" yaml:"minBundle,omitempty"`
}

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
