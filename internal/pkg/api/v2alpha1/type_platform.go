package v2alpha1

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

// InstancePlatformFilter defines OS and Architecture for filtering
// multi-architecture manifest lists. This allows selecting specific
// platforms (e.g., linux/amd64, linux/arm64) when mirroring images.
type InstancePlatformFilter struct {
	// OS is the operating system (e.g., "linux", "windows")
	OS string `json:"os"`
	// Architecture is the CPU architecture (e.g., "amd64", "arm64", "ppc64le", "s390x")
	Architecture string `json:"architecture"`
}

// String returns the platform in "os/architecture" format (e.g., "linux/amd64")
func (p InstancePlatformFilter) String() string {
	return p.OS + "/" + p.Architecture
}

// ParseInstancePlatformFilter parses an "os/architecture" string into an InstancePlatformFilter.
// Returns an error if the string is not in the expected format or if OS/Architecture is empty.
func ParseInstancePlatformFilter(s string) (InstancePlatformFilter, error) {
	parts := strings.SplitN(strings.TrimSpace(s), "/", 2)
	if len(parts) != 2 {
		return InstancePlatformFilter{}, fmt.Errorf("invalid platform %q: expected format is os/architecture (e.g. linux/amd64)", s)
	}
	p := InstancePlatformFilter{OS: parts[0], Architecture: parts[1]}
	if err := p.Validate(); err != nil {
		return InstancePlatformFilter{}, err
	}
	return p, nil
}

// Validate returns an error if OS or Architecture is empty.
func (p InstancePlatformFilter) Validate() error {
	if p.OS == "" {
		return fmt.Errorf("platform OS must not be empty (architecture: %q)", p.Architecture)
	}
	if p.Architecture == "" {
		return fmt.Errorf("platform architecture must not be empty (OS: %q)", p.OS)
	}
	return nil
}

// ValidatePlatforms returns an error if any entry in platforms has an empty OS or Architecture.
func ValidatePlatforms(platforms []InstancePlatformFilter) error {
	for _, p := range platforms {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ConvertPlatformsToStringSlice converts a slice of InstancePlatformFilter to a slice
// of platform strings in "os/architecture" format. Returns nil if the input is empty.
// Returns an error if any entry has an empty OS or Architecture.
func ConvertPlatformsToStringSlice(platforms []InstancePlatformFilter) ([]string, error) {
	if len(platforms) == 0 {
		return nil, nil
	}

	platformStrs := make([]string, 0, len(platforms))
	for _, p := range platforms {
		if err := p.Validate(); err != nil {
			return nil, err
		}
		platformStrs = append(platformStrs, p.String())
	}
	return platformStrs, nil
}
