package operatorcatalog

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
)

// satisfy the stringer interface
var _ fmt.Stringer = &OperatorCatalogPlatform{}

// OperatorCatalogPlatform represents the "platform" that a docker image runs on, which consists of
// a combination of os / architecture / variant.
type OperatorCatalogPlatform struct {
	Os           string // this is likely only ever linux, but this allows for future expansion
	Architecture string // this should be amd64 | ppc64le | s390x
	Variant      string // this is likely to always be empty string, but architectures do have variants (e.g. arm32 has v6, v7 and v8 variants)
	IsIndex      bool   // True if the image was associated with a "manifest list" (i.e. a "index" image), false otherwise for "single" architecture images
}

// String returns combination of os / architecture / variant for images that are associated with a "manifest list"
// and empty string otherwise.
func (p *OperatorCatalogPlatform) String() string {
	if p.IsIndex {
		parts := []string{}
		if p.Os != "" {
			parts = append(parts, p.Os)
		}
		if p.Architecture != "" {
			parts = append(parts, p.Architecture)
		}
		if p.Variant != "" {
			parts = append(parts, p.Variant)
		}
		return strings.Join(parts, "-")
	}
	return ""
}

// ErrorMessagePrefix creates an error message prefix that handles both single and multi architecture images.
// In the case of a single architecture catalog image, we don't need a prefix since that would make things more confusing.
func (p *OperatorCatalogPlatform) ErrorMessagePrefix() string {
	// default to empty string for single architecture images
	platformErrorMessage := ""
	if p.IsIndex {
		platformErrorMessage = fmt.Sprintf("platform %s: ", p.String())
	}
	return platformErrorMessage
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
				result.Os = part
				continue
			}
			if _, found := getKnownArchitectureValues()[part]; found {
				result.Architecture = part
				continue
			}
			if _, found := getKnownVariantValues()[part]; found {
				result.Variant = part
				continue
			}
		}
		result.IsIndex = true
	} else {
		// best we can do here is set the flag correctly and return
		result.IsIndex = false
		return result
	}

	return result
}

/*
CatalogMetadata represents the combination of a DeclarativeConfig, and its associated
IncludeConfig, which is either provided by the user directly or generated from the contents
of DeclarativeConfig
*/
type CatalogMetadata struct {
	Dc               *declcfg.DeclarativeConfig // a DeclarativeConfig instance
	Ic               v1alpha2.IncludeConfig     // a IncludeConfig instance
	CatalogRef       name.Reference             // the reference used to obtain DeclarativeConfig and IncludeConfig
	FullArtifactPath string                     // used of OCI temp location <current working directory>/olm_artifacts/<repo>/<optional platform>/<config folder>
}
