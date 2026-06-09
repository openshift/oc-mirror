package config

import (
	"context"
	"fmt"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

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
//
//nolint:cyclop // Keeping function as-is to avoid diverging from release-4.22 and main branches
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
