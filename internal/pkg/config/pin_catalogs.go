package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

// copyISC creates a shallow copy of ImageSetConfiguration with a deep copy of the Operators slice.
// This ensures we don't mutate the original configuration when pinning catalog digests.
func copyISC(cfg v2alpha1.ImageSetConfiguration) v2alpha1.ImageSetConfiguration {
	copied := cfg

	// Deep copy operators slice (only part we modify)
	if len(cfg.Mirror.Operators) > 0 {
		copied.Mirror.Operators = make([]v2alpha1.Operator, len(cfg.Mirror.Operators))
		copy(copied.Mirror.Operators, cfg.Mirror.Operators)
	}

	return copied
}

// PinCatalogDigests creates a copy of ImageSetConfiguration with catalog references
// pinned to SHA256 digests instead of tags.
//
// For each operator catalog in cfg.Mirror.Operators:
// - Parses the catalog reference
// - Looks up the digest in catalogToFBCMap
// - Replaces the catalog field with digest format: {registry}/{path}@sha256:{digest}
//
// Returns a new ImageSetConfiguration with pinned catalogs and any errors encountered.
func PinCatalogDigests(
	cfg v2alpha1.ImageSetConfiguration,
	catalogToFBCMap map[string]v2alpha1.CatalogFilterResult,
	log clog.PluggableLoggerInterface,
) (v2alpha1.ImageSetConfiguration, error) {
	// Create copy to avoid mutating original
	pinnedCfg := copyISC(cfg)

	// Iterate through operator catalogs
	for i := range pinnedCfg.Mirror.Operators {
		operator := &pinnedCfg.Mirror.Operators[i]

		// Parse catalog reference
		imgSpec, err := image.ParseRef(operator.Catalog)
		if err != nil {
			return v2alpha1.ImageSetConfiguration{},
				fmt.Errorf("failed to parse catalog %s: %w", operator.Catalog, err)
		}

		// Skip if already digest-pinned
		if imgSpec.IsImageByDigest() {
			log.Debug("Catalog %s is already digest-pinned, skipping", operator.Catalog)
			continue
		}

		// Look up digest in map
		filterResult, ok := catalogToFBCMap[imgSpec.ReferenceWithTransport]
		if !ok {
			// Log warning but continue (non-fatal)
			log.Warn("Catalog %s not found in CatalogToFBCMap, skipping pin", operator.Catalog)
			continue
		}

		// Check for empty digest
		if filterResult.Digest == "" {
			log.Warn("Empty digest for catalog %s, skipping pin", operator.Catalog)
			continue
		}

		// Build pinned reference: {registry}/{path}@sha256:{digest}
		pinnedRef := fmt.Sprintf("%s@sha256:%s", imgSpec.Name, filterResult.Digest)

		// Add transport prefix if non-docker (docker:// is default and can be omitted)
		if imgSpec.Transport != "" && imgSpec.Transport != "docker://" {
			pinnedRef = imgSpec.Transport + pinnedRef
		}

		log.Debug("Pinning catalog %s to %s", operator.Catalog, pinnedRef)
		operator.Catalog = pinnedRef
	}

	return pinnedCfg, nil
}

// WritePinnedISC writes an ImageSetConfiguration to a YAML file with timestamp naming.
//
// The file is written to: {workingDir}/isc_pinned_{timestamp}.yaml
// e.g., isc_pinned_2025-12-31T11:37:17Z.yaml
//
// Returns the absolute path to the written file.
func WritePinnedISC(
	cfg v2alpha1.ImageSetConfiguration,
	workingDir string,
) (string, error) {
	cfg.SetGroupVersionKind(v2alpha1.GroupVersion.WithKind(v2alpha1.ImageSetConfigurationKind))

	// Generate filename with timestamp
	filename := fmt.Sprintf("isc_pinned_%s.yaml", time.Now().UTC().Format(time.RFC3339))
	filePath := filepath.Join(workingDir, filename)

	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ISC to YAML: %w", err)
	}

	if err := os.WriteFile(filePath, yamlData, 0o600); err != nil {
		return "", fmt.Errorf("failed to write pinned ISC to %s: %w", filePath, err)
	}

	return filePath, nil
}

// CreateDISCFromISC creates a DeleteImageSetConfiguration from an already-pinned ImageSetConfiguration.
// It simply converts the Mirror section to a Delete section.
//
// The file is written to: {workingDir}/disc_pinned_{timestamp}.yaml
// e.g., disc_pinned_2025-12-31T11:37:17Z.yaml
//
// Returns the absolute path to the written pinned DISC file.
func CreateDISCFromISC(
	pinnedISC v2alpha1.ImageSetConfiguration,
	workingDir string,
) (string, error) {
	disc := v2alpha1.DeleteImageSetConfiguration{
		DeleteImageSetConfigurationSpec: v2alpha1.DeleteImageSetConfigurationSpec{
			Delete: v2alpha1.Delete{
				Platform:         pinnedISC.Mirror.Platform,
				Operators:        pinnedISC.Mirror.Operators,
				AdditionalImages: pinnedISC.Mirror.AdditionalImages,
				Helm:             pinnedISC.Mirror.Helm,
			},
		},
	}

	// Set TypeMeta for DeleteImageSetConfiguration
	disc.SetGroupVersionKind(v2alpha1.GroupVersion.WithKind(v2alpha1.DeleteImageSetConfigurationKind))

	filename := fmt.Sprintf("disc_pinned_%s.yaml", time.Now().UTC().Format(time.RFC3339))
	filePath := filepath.Join(workingDir, filename)

	yamlData, err := yaml.Marshal(disc)
	if err != nil {
		return "", fmt.Errorf("failed to marshal DISC to YAML: %w", err)
	}

	if err := os.WriteFile(filePath, yamlData, 0o600); err != nil {
		return "", fmt.Errorf("failed to write pinned DISC to %s: %w", filePath, err)
	}

	return filePath, nil
}

// PinAndWriteConfigs pins catalogs and writes both ISC and DISC files.
// Returns paths to both written files (ISC path, DISC path).
func PinAndWriteConfigs(
	cfg v2alpha1.ImageSetConfiguration,
	catalogToFBCMap map[string]v2alpha1.CatalogFilterResult,
	workingDir string,
	log clog.PluggableLoggerInterface,
) (string, string, error) {
	// Pin catalog digests
	pinnedISC, err := PinCatalogDigests(cfg, catalogToFBCMap, log)
	if err != nil {
		return "", "", fmt.Errorf("failed to pin catalog digests: %w", err)
	}

	// Write pinned ISC
	iscPath, err := WritePinnedISC(pinnedISC, workingDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to write pinned ISC: %w", err)
	}

	// Write pinned DISC
	discPath, err := CreateDISCFromISC(pinnedISC, workingDir)
	if err != nil {
		return iscPath, "", fmt.Errorf("failed to create pinned DISC: %w", err)
	}

	return iscPath, discPath, nil
}
