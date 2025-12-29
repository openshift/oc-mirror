package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// pinCatalogDigests creates a copy of ImageSetConfiguration with catalog references
// pinned to SHA256 digests instead of tags.
//
// For each operator catalog in cfg.Mirror.Operators:
// - Parses the catalog reference
// - Looks up the digest in catalogToFBCMap
// - Replaces the catalog field with digest format: {registry}/{path}@sha256:{digest}
//
// Returns a new ImageSetConfiguration with pinned catalogs and any errors encountered.
func pinCatalogDigests(
	cfg v2alpha1.ImageSetConfiguration,
	catalogToFBCMap map[string]v2alpha1.CatalogFilterResult,
	log clog.PluggableLoggerInterface,
) (v2alpha1.ImageSetConfiguration, error) {
	// Create copy to avoid mutating original
	pinnedCfg := copyISC(cfg)

	// Iterate through operator catalogs by index to modify in place
	for i := range pinnedCfg.Mirror.Operators {
		op := &pinnedCfg.Mirror.Operators[i]

		// Parse catalog reference
		imgSpec, err := image.ParseRef(op.Catalog)
		if err != nil {
			return v2alpha1.ImageSetConfiguration{},
				fmt.Errorf("failed to parse catalog %s: %w", op.Catalog, err)
		}

		// Skip if already digest-pinned
		if imgSpec.IsImageByDigest() {
			log.Debug("Catalog %s is already digest-pinned, skipping", op.Catalog)
			continue
		}

		// Look up digest in map
		filterResult, ok := catalogToFBCMap[imgSpec.ReferenceWithTransport]
		if !ok {
			// Log warning but continue (non-fatal)
			log.Warn("Catalog %s not found in CatalogToFBCMap, skipping pin", op.Catalog)
			continue
		}

		// Check for empty digest
		if filterResult.Digest == "" {
			log.Warn("Empty digest for catalog %s, skipping pin", op.Catalog)
			continue
		}

		// Build pinned reference: {registry}/{path}@sha256:{digest}
		pinnedRef := fmt.Sprintf("%s@sha256:%s", imgSpec.Name, filterResult.Digest)

		// Add transport prefix if non-docker (docker:// is default and can be omitted)
		if imgSpec.Transport != "" && imgSpec.Transport != "docker://" {
			pinnedRef = imgSpec.Transport + pinnedRef
		}

		log.Debug("Pinning catalog %s to %s", op.Catalog, pinnedRef)
		op.Catalog = pinnedRef
	}

	return pinnedCfg, nil
}

// writeConfigToFile writes a config object (ISC or DISC) to a YAML file with timestamp naming.
// Returns the absolute path to the written file.
func writeConfigToFile(obj interface{}, configType, workingDir string) (string, error) {
	filename := fmt.Sprintf("%s_pinned_%s.yaml", configType, time.Now().UTC().Format(time.RFC3339))
	filePath := filepath.Join(workingDir, filename)

	yamlData, err := yaml.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal %s to YAML: %w", strings.ToUpper(configType), err)
	}

	// #nosec G306 -- config files need to be readable by other users
	if err := os.WriteFile(filePath, yamlData, 0o644); err != nil {
		return "", fmt.Errorf("failed to write pinned %s to %s: %w", strings.ToUpper(configType), filePath, err)
	}

	return filePath, nil
}

// writePinnedISC writes an ImageSetConfiguration to a YAML file with timestamp naming.
//
// The file is written to: {workingDir}/isc_pinned_{timestamp}.yaml
// e.g., isc_pinned_2025-12-31T11:37:17Z.yaml
//
// Returns the absolute path to the written file.
func writePinnedISC(
	cfg v2alpha1.ImageSetConfiguration,
	workingDir string,
) (string, error) {
	cfg.SetGroupVersionKind(v2alpha1.GroupVersion.WithKind(v2alpha1.ImageSetConfigurationKind))
	return writeConfigToFile(cfg, "isc", workingDir)
}

// createDISCFromISC creates a DeleteImageSetConfiguration from an already-pinned ImageSetConfiguration.
// It simply converts the Mirror section to a Delete section.
//
// The file is written to: {workingDir}/disc_pinned_{timestamp}.yaml
// e.g., disc_pinned_2025-12-31T11:37:17Z.yaml
//
// Returns the absolute path to the written pinned DISC file.
func createDISCFromISC(
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

	return writeConfigToFile(disc, "disc", workingDir)
}

// PinAndWriteISCAndDSC pins catalogs and writes both ISC and DISC files.
// Returns paths to both written files (ISC path, DISC path).
func PinAndWriteISCAndDSC(
	cfg v2alpha1.ImageSetConfiguration,
	catalogToFBCMap map[string]v2alpha1.CatalogFilterResult,
	workingDir string,
	log clog.PluggableLoggerInterface,
) (string, string, error) {
	// Pin catalog digests
	pinnedISC, err := pinCatalogDigests(cfg, catalogToFBCMap, log)
	if err != nil {
		return "", "", fmt.Errorf("failed to pin catalog digests: %w", err)
	}

	// Write pinned ISC
	iscPath, err := writePinnedISC(pinnedISC, workingDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to write pinned ISC: %w", err)
	}

	// Write pinned DISC
	discPath, err := createDISCFromISC(pinnedISC, workingDir)
	if err != nil {
		return iscPath, "", fmt.Errorf("failed to create pinned DISC: %w", err)
	}

	return iscPath, discPath, nil
}
