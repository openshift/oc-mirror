package v1alpha2

import (
	"encoding/json"
	"errors"
)

// DefaultPlatformArchitecture defines the default
// architecture used by mirroring platform
// release payloads.
const DefaultPlatformArchitecture = "amd64"

// PlatformType defines the content type for platforms
type PlatformType int

// TypeOCP is default
const (
	TypeOCP PlatformType = iota
	TypeOKD
)

var platformTypeStrings = map[PlatformType]string{
	TypeOCP: "ocp",
	TypeOKD: "okd",
}

var platformStringsType = map[string]PlatformType{
	"ocp": TypeOCP,
	"okd": TypeOKD,
}

// String returns the string representation
// of an PlatformType
func (pt PlatformType) String() string {
	return platformTypeStrings[pt]
}

// MarshalJSON marshals the PlatformType as a quoted json string
func (pt PlatformType) MarshalJSON() ([]byte, error) {
	if err := pt.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(pt.String())
}

// UnmarshalJSON unmarshals a quoted json string to the PlatformType
func (pt *PlatformType) UnmarshalJSON(b []byte) error {
	var j string
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}

	*pt = platformStringsType[j]
	return nil
}

func (pt PlatformType) validate() error {
	if _, ok := platformTypeStrings[pt]; !ok {
		return errors.New("unknown platform type")
	}
	return nil
}
