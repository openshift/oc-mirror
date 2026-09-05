package operator

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/vbauerster/mpb/v8"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/folder"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/spinners"
)

type FilterCollector struct {
	OperatorCollector
}

// OperatorImageCollector - this looks into the operator index image
// taking into account the mode we are in (mirrorToDisk, diskToMirror)
// the image is downloaded (oci format) and the index.json is inspected
// once unmarshalled, the links to manifests are inspected
func (o *FilterCollector) OperatorImageCollector(ctx context.Context) (v2alpha1.CollectorSchema, error) {
	o.Log.Debug(collectorPrefix+"setting copy option o.Opts.MultiArch=%s when collecting operator images", o.Opts.MultiArch)

	relatedImages := make(map[string][]v2alpha1.RelatedImage)
	collectorSchema := v2alpha1.CollectorSchema{
		CatalogToFBCMap: make(map[string]v2alpha1.CatalogFilterResult),
		PlatformFilters: make(map[string][]v2alpha1.InstancePlatformFilter),
	}
	copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{
		OperatorsByImage: make(map[string]map[string]struct{}),
		BundlesByImage:   make(map[string]map[string]string),
	}

	// We are going to try to collect all operators before returning.
	// This slice holds the errors found for each operator plus any error
	// found during the preparation of the images.
	allErrs := []error{}

	p := mpb.New(mpb.PopCompletedMode(), mpb.ContainerOptional(mpb.WithOutput(io.Discard), !o.Opts.Global.IsTerminal))
	for _, op := range o.Config.Mirror.Operators {
		// download the operator index image
		o.Log.Debug(collectorPrefix+"copying operator image %s", op.Catalog)

		if !o.Opts.Global.IsTerminal {
			o.Log.Info("Collecting catalog %s", op.Catalog)
		}
		// prepare spinner
		spinner := spinners.AddSpinner(p, "Collecting catalog "+op.Catalog)

		// CLID-27 ensure we pick up oci:// (on disk) catalogs
		imgSpec, err := image.ParseRef(op.Catalog)
		if err != nil {
			spinner.Abort(true)
			spinner.Wait()
			allErrs = append(allErrs, fmt.Errorf("parse catalog %q: %w", op.Catalog, err))
			continue
		}

		// Use a fresh local map per catalog so we can read exactly which images this
		// catalog contributed — needed to populate platform filters correctly when the
		// same bundle appears in multiple catalogs with different platform requirements.
		localRelatedImages := make(map[string][]v2alpha1.RelatedImage)
		result, err := o.collectOperator(ctx, op, localRelatedImages, copyImageSchemaMap)
		if err != nil {
			spinner.Abort(true)
			spinner.Wait()
			allErrs = append(allErrs, fmt.Errorf("collect catalog %q: %w", op.Catalog, err))
			continue
		}

		// Append this catalog's platform filters into PlatformFilters using localRelatedImages.
		// The same bundle appearing in two catalogs with different platforms gets both appended.
		// Duplicates for shared images are intentional here; mergePlatformFilters in executor.go deduplicates them.
		if err := v2alpha1.ValidatePlatforms(op.Platforms); err != nil {
			allErrs = append(allErrs, fmt.Errorf("invalid platform for operator catalog %q: %w", op.Catalog, err))
		} else if len(op.Platforms) > 0 {
			for _, imgs := range localRelatedImages {
				for _, img := range imgs {
					if ref, parseErr := image.ParseRef(img.Image); parseErr == nil {
						origin := ref.ReferenceWithTransport
						collectorSchema.PlatformFilters[origin] = append(collectorSchema.PlatformFilters[origin], op.Platforms...)
					} else {
						o.Log.Debug(collectorPrefix+"skipping platform filter for image %q: failed to parse ref: %v", img.Image, parseErr)
					}
				}
			}
		}

		// Merge this catalog's images into the global map.
		maps.Copy(relatedImages, localRelatedImages)

		// OCPBUGS-81712: In M2D/M2M modes, op.Catalog is already pinned to digest by executor.go
		// CLID-513: For OCI paths with digest, use a consistent key format (without digest)
		// This matches how catalogImage is constructed in collectOperator
		mapKey := imgSpec.ReferenceWithTransport
		if imgSpec.Transport == consts.OciProtocol && imgSpec.IsImageByDigestOnly() {
			sourceOCIDir, absErr := filepath.Abs(imgSpec.Name)
			if absErr == nil {
				mapKey = consts.OciProtocol + sourceOCIDir
			}
		}
		collectorSchema.CatalogToFBCMap[mapKey] = result

		spinner.Increment()
		if !o.Opts.Global.IsTerminal {
			o.Log.Info("Collected catalog %s", op.Catalog)
		}
	}
	p.Wait()

	o.Log.Debug(collectorPrefix+"related images length %d ", len(relatedImages))
	count := 0
	for _, v := range relatedImages {
		count += len(v)
	}
	o.Log.Debug(collectorPrefix+"images to copy (before duplicates) %d ", count)

	var err error
	var allImages []v2alpha1.CopyImageSchema
	// check the mode
	switch {
	case o.Opts.IsMirrorToDisk():
		allImages, err = o.prepareM2DCopyBatch(relatedImages)
	case o.Opts.IsMirrorToMirror():
		allImages, err = o.dispatchImagesForM2M(relatedImages)
	case o.Opts.IsDiskToMirror() || o.Opts.Mode == string(mirror.DeleteMode):
		allImages, err = o.prepareD2MCopyBatch(relatedImages)
	}
	if err != nil {
		allErrs = append(allErrs, err)
	}

	collectorSchema.AllImages = allImages
	collectorSchema.CopyImageSchemaMap = *copyImageSchemaMap

	return collectorSchema, errors.Join(allErrs...)
}

func isFullCatalog(catalog v2alpha1.Operator) bool {
	return len(catalog.IncludeConfig.Packages) == 0 && catalog.Full
}

// digestOfFilter computes a hash of the operator filter configuration.
//
// The catalogDigest parameter controls normalization behavior:
//   - When non-empty: normalizes the catalog reference to "name@sha256:digest" form (CLID-513).
//     This ensures consistent hashes whether catalog is specified by tag or digest in the ISC.
//   - When empty: uses the catalog reference as-is.
func digestOfFilter(catalog v2alpha1.Operator, catalogDigest string) (string, error) {
	c := catalog
	c.TargetCatalog = ""
	c.TargetTag = ""
	c.TargetCatalogSourceTemplate = ""
	if c.Catalog != "" && catalogDigest != "" {
		imgSpec, err := image.ParseRef(c.Catalog)
		if err == nil {
			c.Catalog = image.WithDigest(imgSpec.Name, catalogDigest)
		}
	}
	pkgs, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", md5.Sum(pkgs))[0:32], nil
}

// findFilterDigest returns the filter digest to use, checking for an existing
// filtered catalog on disk and falling back to the legacy digest if needed.
func findFilterDigest(op v2alpha1.Operator, catalogDigest, filteredCatalogsDir string) (string, error) {
	filterDigest, err := digestOfFilter(op, catalogDigest)
	if err != nil {
		return "", err
	}

	digestFile := filepath.Join(filteredCatalogsDir, filterDigest, "digest")
	if _, err := os.Stat(digestFile); err == nil {
		return filterDigest, nil
	}

	// Try legacy digest (without normalization) for backwards compatibility
	// Error is intentionally ignored - if legacy digest fails, we use the normalized one
	legacyDigest, _ := digestOfFilter(op, "")
	if legacyDigest == filterDigest {
		return filterDigest, nil
	}

	legacyDigestFile := filepath.Join(filteredCatalogsDir, legacyDigest, "digest")
	if _, err := os.Stat(legacyDigestFile); err == nil {
		return legacyDigest, nil
	}

	return filterDigest, nil
}

func (o FilterCollector) isAlreadyFiltered(ctx context.Context, srcImage, filteredImageDigest string) bool {
	imgSpec, err := image.ParseRef(srcImage)
	if err != nil {
		o.Log.Debug(errMsg, err.Error())
		return false
	}

	sourceCtx, err := o.Opts.SrcImage.NewSystemContext()
	if err != nil {
		return false
	}
	// OCPBUGS-37948 : No TLS verification when getting manifests from the cache registry
	if strings.Contains(srcImage, o.Opts.LocalStorageFQDN) { // when copying from cache, use HTTP
		sourceCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
	}

	catalogDigest, err := o.Manifest.ImageDigest(ctx, sourceCtx, imgSpec.ReferenceWithTransport)
	if err != nil {
		o.Log.Debug(errMsg, err.Error())
		return false
	}
	return filteredImageDigest == catalogDigest
}

// isDeleteOfV1CatalogFromDisk returns true when trying to delete an operator catalog mirrored by oc-mirror v1 and the catalog was on disk (using oci:// on the ImageSetConfiguration)
// TODO remove me when the migration from oc-mirror v1 to v2 ends
func (o *FilterCollector) isDeleteOfV1CatalogFromDisk() bool {
	return o.Opts.IsDiskToMirror() && o.Opts.IsDelete() && o.generateV1DestTags
}

// TODO remove me when the migration from oc-mirror v1 to v2 ends
func addOriginFromOperatorCatalogOnDisk(relatedImages *map[string][]v2alpha1.RelatedImage) {
	for key, images := range *relatedImages {
		for i := range images {
			// Modify the RelatedImage object as needed
			images[i].OriginFromOperatorCatalogOnDisk = true
		}
		(*relatedImages)[key] = images
	}
}

func (o FilterCollector) collectOperator( //nolint:cyclop // TODO: this needs further refactoring
	ctx context.Context,
	op v2alpha1.Operator,
	relatedImages map[string][]v2alpha1.RelatedImage,
	copyImageSchemaMap *v2alpha1.CopyImageSchemaMap,
) (v2alpha1.CatalogFilterResult, error) {
	// CLID-47 double check that targetCatalog is valid
	if op.TargetCatalog != "" && !v2alpha1.IsValidPathComponent(op.TargetCatalog) {
		return v2alpha1.CatalogFilterResult{}, fmt.Errorf("invalid targetCatalog %s", op.TargetCatalog)
	}

	// CLID-27 ensure we pick up oci:// (on disk) catalogs
	imgSpec, err := image.ParseRef(op.Catalog)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	catalogDigest, err := o.getCatalogDigest(ctx, op)
	if err != nil {
		// OCPBUGS-36548 (manifest unknown)
		return v2alpha1.CatalogFilterResult{}, err
	}

	result, err := o.filterOperator(ctx, op, imgSpec, catalogDigest)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	ri, err := o.ctlgHandler.getRelatedImagesFromCatalog(result.DeclConfig, copyImageSchemaMap)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}
	o.Log.Debug("Found %d related images for catalog %q", len(ri), op.Catalog)

	// OCPBUGS-45059
	// TODO: remove me when the migration from oc-mirror v1 to v2 ends
	if imgSpec.Transport == consts.OciProtocol && o.isDeleteOfV1CatalogFromDisk() {
		addOriginFromOperatorCatalogOnDisk(&ri)
	}

	maps.Copy(relatedImages, ri)

	targetTag := op.TargetTag
	if len(targetTag) == 0 && imgSpec.Transport == consts.OciProtocol {
		// for this case only, img.ParseRef(in its current state)
		// will not be able to determine the digest.
		// this leaves the oci imgSpec with no tag nor digest as it
		// goes to prepareM2DCopyBatch/prepareD2MCopyBath. This is
		// why we set the digest read from manifest in targetTag
		targetTag = "latest"
	}

	catalogName := op.TargetCatalog
	if len(catalogName) == 0 {
		catalogName = path.Base(imgSpec.Name)
	}

	// OCPBUGS-81712: In M2D/M2M modes, op.Catalog is already pinned to digest by PinCatalogDigests()
	// so catalogImage will use the digest-based reference directly
	catalogImage := op.Catalog
	if imgSpec.Transport == consts.OciProtocol {
		// ensure correct oci format and directory lookup
		sourceOCIDir, err := filepath.Abs(imgSpec.Name)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, fmt.Errorf("failed to get OCI image path: %w", err)
		}
		catalogImage = consts.OciProtocol + sourceOCIDir
	}

	rebuiltTag := ""
	if !isFullCatalog(op) {
		imageIndexDir := filepath.Join(o.Opts.Global.WorkingDir, operatorCatalogsDir, imgSpec.ComponentName(), catalogDigest)
		filteredCatalogsDir := filepath.Join(imageIndexDir, operatorCatalogFilteredDir)

		tag, err := findFilterDigest(op, catalogDigest, filteredCatalogsDir)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, err
		}
		rebuiltTag = tag
	}

	componentName := imgSpec.ComponentName() + "." + result.Digest
	relatedImages[componentName] = []v2alpha1.RelatedImage{
		{
			Name:          catalogName,
			Image:         catalogImage,
			Type:          v2alpha1.TypeOperatorCatalog,
			TargetTag:     targetTag,
			TargetCatalog: op.TargetCatalog,
			RebuiltTag:    rebuiltTag,
			FullCatalog:   isFullCatalog(op),
		},
	}
	return result, nil
}

func (o FilterCollector) getCatalogDigest(ctx context.Context, op v2alpha1.Operator) (string, error) {
	// OCPBUGS-36214: For diskToMirror (and delete), access to the source registry is not guaranteed
	if o.Opts.IsDiskToMirror() || o.Opts.IsDeleteMode() {
		return o.catalogDigest(ctx, op)
	}

	imgSpec, err := image.ParseRef(op.Catalog)
	if err != nil {
		return "", err
	}

	// If the catalog is specified by digest, return it directly.
	if imgSpec.IsImageByDigestOnly() {
		return imgSpec.Digest, nil
	}

	srcCtx, err := o.Opts.SrcImage.NewSystemContext()
	if err != nil {
		return "", err
	}

	return o.Manifest.ImageDigest(ctx, srcCtx, imgSpec.ReferenceWithTransport)
}

func (o FilterCollector) filterOperator(ctx context.Context, op v2alpha1.Operator, imgSpec image.ImageSpec, catalogDigest string) (v2alpha1.CatalogFilterResult, error) { //nolint:cyclop // TODO: this needs further refactoring
	o.Log.Debug("Filtering catalog %q", op.Catalog)
	imageIndexDir := filepath.Join(o.Opts.Global.WorkingDir, operatorCatalogsDir, imgSpec.ComponentName(), catalogDigest)
	filteredCatalogsDir := filepath.Join(imageIndexDir, operatorCatalogFilteredDir)

	filterDigest, err := findFilterDigest(op, catalogDigest, filteredCatalogsDir)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	var isAlreadyFiltered bool
	filteredImageDigest, err := os.ReadFile(filepath.Join(filteredCatalogsDir, filterDigest, "digest"))
	if err != nil {
		isAlreadyFiltered = false
	} else {
		srcFilteredCatalog, err := o.cachedCatalog(op, filterDigest)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, err
		}
		isAlreadyFiltered = o.isAlreadyFiltered(ctx, srcFilteredCatalog, string(filteredImageDigest))
	}

	if isAlreadyFiltered {
		filterConfigDir := filepath.Join(filteredCatalogsDir, filterDigest, operatorCatalogConfigDir)
		filteredDC, err := o.ctlgHandler.GetDeclarativeConfig(ctx, filterConfigDir)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, fmt.Errorf("retrieve filtered catalog config from %s: %w", filterConfigDir, err)
		}
		return v2alpha1.CatalogFilterResult{
			OperatorFilter:     op,
			FilteredConfigPath: filterConfigDir,
			ToRebuild:          false,
			DeclConfig:         filteredDC,
			Digest:             catalogDigest,
		}, nil
	}
	o.Log.Debug("Catalog has not been filtered previously")

	if err := o.ctlgHandler.EnsureCatalogInOCIFormat(ctx, imgSpec, op.Catalog, imageIndexDir, o.Opts); err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	// It's now in oci format so we can go directly to the index.json file
	dcPath, err := o.ctlgHandler.ExtractOCIConfigLayers(imgSpec, imageIndexDir)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	originalDC, err := o.ctlgHandler.GetDeclarativeConfig(ctx, dcPath)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	// No filtering needs to be done if we're copying the whole catalog
	if isFullCatalog(op) {
		return v2alpha1.CatalogFilterResult{
			OperatorFilter:     op,
			FilteredConfigPath: "", // this value is not relevant: no rebuilding
			ToRebuild:          false,
			DeclConfig:         originalDC,
			Digest:             catalogDigest,
		}, nil
	}

	filteredDC, err := filterCatalog(ctx, *originalDC, op)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	filteredDC = eliminatingIntermediaryVersions(filteredDC, op, o.Log)

	filteredDigestPath := filepath.Join(filteredCatalogsDir, filterDigest, operatorCatalogConfigDir)

	if err := folder.CreateFolders(filteredDigestPath); err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	if err := saveDeclarativeConfig(*filteredDC, filteredDigestPath); err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	return v2alpha1.CatalogFilterResult{
		OperatorFilter:     op,
		FilteredConfigPath: filteredDigestPath,
		ToRebuild:          true,
		DeclConfig:         filteredDC,
		Digest:             catalogDigest,
	}, nil
}

func eliminatingIntermediaryVersions(dc *declcfg.DeclarativeConfig, iscCatalogFilter v2alpha1.Operator, log clog.PluggableLoggerInterface) *declcfg.DeclarativeConfig {
	maxVersions := map[string]string{}
	for _, pkg := range iscCatalogFilter.Packages {
		maxVersions[pkg.Name] = pkg.MaxVersion
	}
	for _, pkg := range dc.Packages {
		maxVersion, ok := maxVersions[pkg.Name]
		if !ok {
			log.Debug("No max version found for package %q and thus no elimination of versions", pkg.Name)
			continue
		}
		var channels []declcfg.Channel
		for _, chanel := range dc.Channels {
			chanel.Entries = eliminatingIntermediaryVersionsWithMaxVersion(chanel, maxVersion, log)
			channels = append(channels, chanel)
		}
		dc.Channels = channels
	}
	return dc
}

// eliminatingIntermediaryVersions eliminates intermediary versions between maxVersion to the head if
// the replaces chain holds from maxVersion to the head of the channel
// and each between them skips all older versions
func eliminatingIntermediaryVersionsWithMaxVersion(channel declcfg.Channel, maxVersion string, log clog.PluggableLoggerInterface) []declcfg.ChannelEntry {
	maxV, err := semver.Parse(maxVersion)
	if err != nil {
		return channel.Entries
	}
	eliminationIndex := -1
	for i, entry := range channel.Entries {
		if i+1 >= len(channel.Entries) {
			break
		}
		if greater(entry.Name, maxV) && entry.Replaces == channel.Entries[i+1].Name {
			eliminationIndex = i
			continue
		}
		break
	}
	if eliminationIndex == -1 || !skipLookGood(channel.Entries, eliminationIndex) {
		return channel.Entries
	}
	for i := 0; i <= eliminationIndex; i++ {
		log.Info("eliminating intermediary version %q for channel %q of package %q", channel.Entries[i].Name, channel.Name, channel.Package)
	}
	entryHead := channel.Entries[0]
	entryHead.Replaces = channel.Entries[eliminationIndex+1].Name
	return append([]declcfg.ChannelEntry{entryHead}, channel.Entries[eliminationIndex+1:]...)
}

// skipLookGood reports whether the intermediary versions in
// entries[1 .. eliminationIndex] can be safely eliminated. After elimination
// only the head (entries[0]) survives above entries[eliminationIndex+1], so it
// is safe only when the head skips every version it would then jump over: each
// entry from index 1 through eliminationIndex+1. A version is skipped when its
// name is listed in the head's Skips or when it falls within the head's SkipRange.
func skipLookGood(entries []declcfg.ChannelEntry, eliminationIndex int) bool {
	if eliminationIndex < 0 || eliminationIndex+1 >= len(entries) {
		return false
	}
	head := entries[0]
	skips := make(map[string]struct{}, len(head.Skips))
	for _, s := range head.Skips {
		skips[s] = struct{}{}
	}
	// A missing or malformed SkipRange simply provides no coverage.
	var skipRange semver.Range
	if head.SkipRange != "" {
		skipRange, _ = semver.ParseRange(head.SkipRange)
	}
	for _, entry := range entries[1 : eliminationIndex+2] {
		if _, listed := skips[entry.Name]; listed {
			continue
		}
		if v, ok := versionFromEntryName(entry.Name); ok && skipRange != nil && skipRange(v) {
			continue
		}
		return false
	}
	return true
}

// versionFromEntryName extracts the semver from a channel entry name of the
// form "<package>.v<semver>" (e.g. "foo.v1.3.0").
func versionFromEntryName(entryName string) (semver.Version, bool) {
	idx := strings.LastIndex(entryName, ".v")
	if idx == -1 {
		return semver.Version{}, false
	}
	v, err := semver.Parse(entryName[idx+2:])
	if err != nil {
		return semver.Version{}, false
	}
	return v, true
}

func greater(entryName string, version semver.Version) bool {
	entryVersion, ok := versionFromEntryName(entryName)
	if !ok {
		return false
	}
	return entryVersion.GT(version)
}

func TagRebuiltCatalogByDigestOnly(collectorSchema *v2alpha1.CollectorSchema, localStorageFQDN, workingDir string) {
	for k, img := range collectorSchema.AllImages {
		if img.RebuiltTag == "" || !img.Type.IsOperatorCatalog() || strings.Contains(img.Destination, localStorageFQDN) {
			continue
		}

		imgSpec, err := image.ParseRef(img.Origin)
		if err != nil {
			continue
		}
		if !imgSpec.IsImageByDigestOnly() {
			continue
		}
		dest := strings.Split(img.Destination, imgSpec.Algorithm)
		if len(dest) <= 1 {
			continue
		}

		filteredImageDigest, err := FilteredCatalogDigest(workingDir, imgSpec.ComponentName(), imgSpec.Digest, img.RebuiltTag)
		if err != nil {
			collectorSchema.AllImages[k].Destination = dest[0] + imgSpec.Algorithm + "-" + img.RebuiltTag
		} else {
			collectorSchema.AllImages[k].Destination = dest[0] + imgSpec.Algorithm + "-" + string(filteredImageDigest)
		}
	}
}

func FilteredCatalogDigest(workingDir, catalogName, originalDigest, iscFilterDigest string) (string, error) {
	imageIndexDir := filepath.Join(workingDir, operatorCatalogsDir, catalogName, originalDigest)
	filteredCatalogsDir := filepath.Join(imageIndexDir, operatorCatalogFilteredDir)

	filteredImageDigest, err := os.ReadFile(filepath.Join(filteredCatalogsDir, iscFilterDigest, "digest"))

	return string(filteredImageDigest), err
}
