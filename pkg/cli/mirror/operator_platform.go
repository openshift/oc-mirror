package mirror

import (
	"fmt"
	"strings"
)

// satisfy the stringer interface
var _ fmt.Stringer = &OperatorCatalogPlatform{}

// OperatorCatalogPlatform represents the "platform" that a docker image runs on, which consists of
// a combination of os / architecture / variant.
type OperatorCatalogPlatform struct {
	os           string // this is likely only ever linux, but this allows for future expansion
	architecture string // this should be amd64 | ppc64le | s390x
	variant      string // this is likely to always be empty string, but architectures do have variants (e.g. arm32 has v6, v7 and v8 variants)
	isIndex      bool   // True if the image was associated with a "manifest list" (i.e. a "index" image), false otherwise for "single" architecture images
}

// String returns combination of os / architecture / variant for images that are associated with a "manifest list"
// and empty string otherwise.
func (p *OperatorCatalogPlatform) String() string {
	if p.isIndex {
		parts := []string{}
		if p.os != "" {
			parts = append(parts, p.os)
		}
		if p.architecture != "" {
			parts = append(parts, p.architecture)
		}
		if p.variant != "" {
			parts = append(parts, p.variant)
		}
		return strings.Join(parts, "-")
	}
	return ""
}

// getKnownOSValues returns known set of os values (see https://github.com/opencontainers/image-spec/blob/v1.0.0/image-index.md)
func getKnownOSValues() map[string]struct{} {
	return map[string]struct{}{
		"aix":       {},
		"android":   {},
		"darwin":    {},
		"dragonfly": {},
		"freebsd":   {},
		"illumos":   {},
		"ios":       {},
		"js":        {},
		"linux":     {},
		"netbsd":    {},
		"openbsd":   {},
		"plan9":     {},
		"solaris":   {},
		"windows":   {},
	}
}

// getKnownArchitectureValues returns known set of arch values (see https://github.com/opencontainers/image-spec/blob/v1.0.0/image-index.md)
func getKnownArchitectureValues() map[string]struct{} {
	return map[string]struct{}{
		"ppc64":    {},
		"386":      {},
		"amd64":    {},
		"arm":      {},
		"arm64":    {},
		"wasm":     {},
		"loong64":  {},
		"mips":     {},
		"mipsle":   {},
		"mips64":   {},
		"mips64le": {},
		"ppc64le":  {},
		"riscv64":  {},
		"s390x":    {},
	}
}

// getKnownVariantValues returns known set of variant values (see https://github.com/opencontainers/image-spec/blob/v1.0.0/image-index.md)
func getKnownVariantValues() map[string]struct{} {
	return map[string]struct{}{
		"v5": {},
		"v6": {},
		"v7": {},
		"v8": {},
	}
}

/*
This function parses a string created by OperatorCatalogPlatform.String() and creates a
OperatorCatalogPlatform as a result.
*/
func NewOperatorCatalogPlatform(platformAsString string) *OperatorCatalogPlatform {
	result := &OperatorCatalogPlatform{}
	if platformAsString != "" {
		// this is multi arch
		for _, part := range strings.Split(platformAsString, "-") {
			if _, found := getKnownOSValues()[part]; found {
				result.os = part
				continue
			}
			if _, found := getKnownArchitectureValues()[part]; found {
				result.architecture = part
				continue
			}
			if _, found := getKnownVariantValues()[part]; found {
				result.variant = part
				continue
			}
		}
		result.isIndex = true
	} else {
		// best we can do here is set the flag correctly and return
		result.isIndex = false
		return result
	}

	return result
}
