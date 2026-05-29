package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// WriteISCAndDSC writes both ISC and DISC files from an already-pinned configuration.
// For M2D/M2M modes, catalogs are already pinned during Complete() before collection starts.
// Returns paths to both written files (ISC path, DISC path).
func WriteISCAndDSC(
	cfg v2alpha1.ImageSetConfiguration,
	opts *mirror.CopyOptions,
	log clog.PluggableLoggerInterface,
) (string, string, error) {
	// Write pinned ISC
	iscPath, err := writePinnedISC(cfg, opts.Global.WorkingDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to write pinned ISC: %w", err)
	}

	// Write pinned DISC
	discPath, err := createDISCFromISC(cfg, opts.Global.WorkingDir)
	if err != nil {
		return iscPath, "", fmt.Errorf("failed to create pinned DISC: %w", err)
	}

	return iscPath, discPath, nil
}

// PinCatalogDigests creates a copy of ImageSetConfiguration with catalog references
// pinned to SHA256 digests instead of tags.
//
// OCPBUGS-81712: This function resolves all catalog tags to digests upfront (during Complete())
// to prevent race conditions where catalog tags change between collection and mirroring.
// Without this, a catalog tag could resolve to different digests at collection vs mirroring time,
// causing the working-dir to be structured for one digest while the tarball contains another.
//
// For each operator catalog in cfg.Mirror.Operators:
// - Parses the catalog reference
// - Resolves the tag to a digest via network call (manifestAPI.ImageDigest)
// - Replaces the catalog field with digest format: {registry}/{path}@sha256:{digest}
// - Preserves original tag as TargetTag if user didn't provide one
//
// Best-effort approach: If catalog pinning fails (e.g., invalid registry, DNS errors, auth failures),
// logs a warning and continues. This allows invalid catalogs to be caught by the operator collector
// during collection phase, where proper exit codes (2, 4, 20) can be returned for integration tests.
//
// Returns a new ImageSetConfiguration with pinned catalogs. The original config is never mutated.
// Never returns an error due to best-effort approach (errors are logged as warnings).
func PinCatalogDigests(
	ctx context.Context,
	cfg v2alpha1.ImageSetConfiguration,
	manifestAPI manifest.ManifestInterface,
	opts *mirror.CopyOptions,
	log clog.PluggableLoggerInterface,
) v2alpha1.ImageSetConfiguration {
	if len(cfg.Mirror.Operators) == 0 {
		return cfg
	}

	// Create copy to avoid mutating original
	pinnedCfg := copyISC(cfg)

	// Iterate through operator catalogs by index to modify in place
	for i := range pinnedCfg.Mirror.Operators {
		op := &pinnedCfg.Mirror.Operators[i]

		if err := pinSingleCatalogDigest(ctx, op, manifestAPI, opts, log); err != nil {
			// Best-effort: log warning and continue instead of failing
			// This allows the operator collector to handle the error with proper exit codes
			log.Warn("failed to pin catalog at index %d (%s): %v - will be resolved during collection", i, op.Catalog, err)
			continue
		}
	}

	return pinnedCfg
}

// pinSingleCatalogDigest pins a single catalog reference to its SHA256 digest.
//
// The function modifies the op.Catalog field in-place and handles the following cases:
// - OCI transport catalogs: skipped (already local, no network resolution needed)
// - Already digest-pinned: skipped (no action needed)
// - Tag-based reference: resolves digest via network call and updates op.Catalog
// - Empty digest returned: skipped (catalog won't be pinned)
//
// When pinning a tag-based catalog:
// 1. Preserves the original tag as TargetTag (if user didn't provide one)
// 2. Resolves the tag to a digest via manifestAPI.ImageDigest (network call)
// 3. Replaces op.Catalog with the digest-pinned reference
// 4. Preserves transport prefix for non-docker transports
//
// Returns error only for fatal failures (parse errors, network errors).
// Empty or missing digests result in skipping (no error) to maintain best-effort behavior.
func pinSingleCatalogDigest(
	ctx context.Context,
	op *v2alpha1.Operator,
	manifestAPI manifest.ManifestInterface,
	opts *mirror.CopyOptions,
	log clog.PluggableLoggerInterface,
) error {
	// Parse catalog reference
	imgSpec, err := image.ParseRef(op.Catalog)
	if err != nil {
		return fmt.Errorf("failed to parse catalog %s: %w", op.Catalog, err)
	}

	// Skip OCI catalogs - they're already local
	if imgSpec.Transport == consts.OciProtocol {
		log.Debug("Skipping OCI catalog %s (already local)", op.Catalog)
		return nil
	}

	// Skip if the image is already pinned by digest
	if imgSpec.IsImageByDigest() {
		log.Debug("Catalog %s is already pinned by digest, skipping", op.Catalog)
		return nil
	}

	// Preserve the original tag for the destination if user didn't provide targetTag
	if op.TargetTag == "" && imgSpec.Tag != "" {
		op.TargetTag = imgSpec.Tag
		log.Debug("Preserving original tag %s as TargetTag for catalog %s", imgSpec.Tag, op.Catalog)
	}

	// Resolve tag to digest via network
	srcCtx, err := opts.SrcImage.NewSystemContext()
	if err != nil {
		return fmt.Errorf("failed to create system context: %w", err)
	}

	catalogDigest, err := manifestAPI.ImageDigest(ctx, srcCtx, imgSpec.ReferenceWithTransport)
	if err != nil {
		return fmt.Errorf("failed to resolve digest for catalog %s: %w", op.Catalog, err)
	}

	// Skip pinning if digest is empty
	if catalogDigest == "" {
		log.Debug("Skipping catalog %s (empty digest)", op.Catalog)
		return nil
	}

	// Build pinned reference: {registry}/{path}@sha256:{digest}
	pinnedRef := image.WithDigest(imgSpec.Name, catalogDigest)

	// Preserve transport prefix if non-docker
	if imgSpec.Transport != "" && imgSpec.Transport != consts.DockerProtocol {
		pinnedRef = imgSpec.Transport + pinnedRef
	}

	log.Debug("Resolved catalog %s to digest %s", op.Catalog, pinnedRef)
	op.Catalog = pinnedRef
	return nil
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
