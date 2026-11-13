package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/docker/go-units"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/archive"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/emoji"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	"github.com/openshift/oc-mirror/v2/internal/pkg/imagebuilder"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/signature"
)

// Failed image inspections discard partial blobs; signature errors retain image content.
type imageSizeResult struct {
	Blobs  []types.BlobInfo
	Errors []error
}

func (o *ExecutorSchema) DryRun(ctx context.Context, cs v2alpha1.CollectorSchema) error {
	allImages := cs.AllImages
	outDir := filepath.Join(o.Opts.Global.WorkingDir, dryRunOutDir)
	os.RemoveAll(outDir)

	err := o.MakeDir.makeDirAll(outDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	sizeSchema, aliases := o.prepareDryRunCatalogs(ctx, cs)
	manifestListDigests, sizeResults := o.inspectDryRunImages(ctx, sizeSchema)
	for source, rebuilt := range aliases {
		if digests := manifestListDigests[rebuilt]; len(digests) > 0 {
			manifestListDigests[source] = digests
		}
	}

	if err := o.writeDryRunMapping(ctx, allImages, manifestListDigests, outDir); err != nil {
		return err
	}

	// Generate cluster resources in dry-run mode (skip for mirror-to-disk since target registry is unknown)
	if !o.Opts.IsMirrorToDisk() {
		if err := o.generateClusterResources(ctx, allImages); err != nil {
			o.Log.Warn("Cluster resources generation failed (dry-run mode): %v", err)
		}
	}

	// Size estimation (appended AFTER existing output for backward compatibility)
	if o.Manifest != nil && !o.Opts.IsDiskToMirror() {
		report := o.estimateDryRunSize(ctx, sizeSchema, sizeResults)
		report.ImageCount = len(dedupeImagesBySource(allImages))
		o.logSizeReport(report)
	}

	return nil
}

// Prepare catalog OCI layouts for sizing, using the normal builder without pushes.
func (o *ExecutorSchema) prepareDryRunCatalogs(ctx context.Context, cs v2alpha1.CollectorSchema) (v2alpha1.CollectorSchema, map[string]string) {
	aliases := make(map[string]string)
	if o.Opts.IsDiskToMirror() {
		return cs, aliases
	}
	cs.AllImages = append([]v2alpha1.CopyImageSchema(nil), cs.AllImages...)
	cs.CatalogToFBCMap = maps.Clone(cs.CatalogToFBCMap)
	seen := make(map[string]bool)
	for _, img := range cs.AllImages {
		filtered, ok := cs.CatalogToFBCMap[img.Origin]
		if !ok || img.RebuiltTag == "" || !isLocalCacheSource(img.Destination, o.Opts.LocalStorageFQDN) || seen[img.Origin] {
			continue
		}
		seen[img.Origin] = true
		rebuilt, err := o.dryRunCatalog(ctx, img, filtered)
		if err != nil {
			o.Log.Debug("unable to prepare catalog %s: %v", img.Origin, err)
			continue
		}
		aliases[rebuilt.Destination] = rebuilt.Source
		cs.AllImages = append(cs.AllImages, rebuilt)
		filtered.ToRebuild = false
		cs.CatalogToFBCMap[img.Origin] = filtered
	}
	for i, img := range cs.AllImages {
		if source, ok := aliases[img.Source]; ok {
			cs.AllImages[i].Source = source
		}
	}
	return cs, aliases
}

func (o *ExecutorSchema) dryRunCatalog(ctx context.Context, img v2alpha1.CopyImageSchema, filtered v2alpha1.CatalogFilterResult) (v2alpha1.CopyImageSchema, error) {
	if filtered.ToRebuild {
		builder, ok := o.CatalogBuilder.(*imagebuilder.GCRCatalogBuilder)
		if !ok || !builder.CopyOpts.IsDryRun {
			return img, errors.New("catalog builder is not configured for dry-run")
		}
		if err := builder.RebuildCatalog(ctx, img, filtered.FilteredConfigPath); err != nil {
			return img, err
		}
	}
	rebuilt, err := rebuiltCatalog(img)
	if err != nil {
		return img, err
	}
	layoutDir := filepath.Join(filepath.Dir(filtered.FilteredConfigPath), imagebuilder.FilteredCatalogImageDir)
	if _, err := os.Stat(layoutDir); err == nil {
		rebuilt.Source = consts.OciProtocolTrimmed + layoutDir
	}
	return rebuilt, nil
}

func (o *ExecutorSchema) inspectDryRunImages(ctx context.Context, cs v2alpha1.CollectorSchema) (map[string][]string, map[string]imageSizeResult) {
	includeSizes := !o.Opts.IsDiskToMirror()
	var images []v2alpha1.CopyImageSchema
	if includeSizes || o.Opts.IsDryRunManifestLists {
		images = dedupeImagesBySource(cs.AllImages)
	}
	if o.Opts.IsDryRunManifestLists {
		o.Log.Info(emoji.LeftPointingMagnifyingGlass+" inspecting %d images for manifest lists...", len(images))
	}
	if o.Manifest != nil && includeSizes {
		o.Log.Info(emoji.LeftPointingMagnifyingGlass + " estimating image sizes...")
	}
	digests, results := o.inspectImages(ctx, images, cs.PlatformFilters, includeSizes)
	if !o.Opts.IsDryRunManifestLists {
		digests = nil
	}
	return digests, results
}

// writeDryRunMapping generates the mapping file for all images and, for the
// mirror-to-disk workflow, the missing-images file. It reports cache
// availability counts via the logger.
func (o *ExecutorSchema) writeDryRunMapping(ctx context.Context, allImages []v2alpha1.CopyImageSchema, manifestListDigests map[string][]string, outDir string) error {
	mappingTxtFilePath := filepath.Join(outDir, mappingFile)
	mappingTxtFile, err := os.Create(mappingTxtFilePath)
	if err != nil {
		return fmt.Errorf("failed to create mapping file: %w", err)
	}
	defer mappingTxtFile.Close()

	nbMissingImgs := 0
	nbAvailableImgs := 0
	var buff bytes.Buffer
	var missingImgsBuff bytes.Buffer
	for _, img := range allImages {
		missing := o.processImageForDryRun(ctx, img, manifestListDigests, &buff, &missingImgsBuff)
		if missing {
			nbMissingImgs++
		} else if o.Opts.IsMirrorToDisk() {
			nbAvailableImgs++
		}
	}

	if _, err := mappingTxtFile.Write(buff.Bytes()); err != nil {
		return err
	}
	if nbMissingImgs > 0 {
		if err := o.writeMissingImagesFile(outDir, missingImgsBuff.Bytes(), nbMissingImgs, len(allImages)); err != nil {
			return err
		}
	}

	if nbAvailableImgs > 0 && nbMissingImgs == 0 {
		o.Log.Info("all %d images required for mirroring are available in local cache. You may proceed with mirroring from disk to disconnected registry", nbAvailableImgs)
	}
	o.Log.Info(emoji.PageFacingUp+" list of all images for mirroring in : %s", mappingTxtFilePath)
	return nil
}

func (o *ExecutorSchema) writeMissingImagesFile(outDir string, data []byte, nbMissing, total int) error {
	missingImgsFilePath := filepath.Join(outDir, missingImgsFile)
	if err := os.WriteFile(missingImgsFilePath, data, 0644); err != nil { //nolint: gosec // no sensitive data in file and it must be readable by others
		return fmt.Errorf("error writing missing images file: %w", err)
	}
	o.Log.Warn(emoji.Warning+"  %d/%d images necessary for mirroring are not available in the cache.", nbMissing, total)
	o.Log.Warn("List of missing images in : %s.\nplease re-run the mirror to disk process", missingImgsFilePath)
	return nil
}

type subDigestEntry struct{ source, dest string }

func (o *ExecutorSchema) processImageForDryRun(ctx context.Context, img v2alpha1.CopyImageSchema, manifestListDigests map[string][]string, buff, missingImgsBuff *bytes.Buffer) bool {
	buff.WriteString(img.Source + "=" + img.Destination + "\n")

	subDigestEntries := o.writeSubDigestEntries(img, manifestListDigests, buff)

	if !o.Opts.IsMirrorToDisk() {
		return false
	}
	exists, err := o.Mirror.Check(ctx, img.Destination, o.Opts, false)
	if err != nil {
		o.Log.Debug("unable to check existence of %s in local cache: %v", img.Destination, err)
	}
	if err != nil || !exists {
		missingImgsBuff.WriteString(img.Source + "=" + img.Destination + "\n")
		for _, sub := range subDigestEntries {
			missingImgsBuff.WriteString(sub.source + "=" + sub.dest + "\n")
		}
		return true
	}
	return false
}

func (o *ExecutorSchema) writeSubDigestEntries(img v2alpha1.CopyImageSchema, manifestListDigests map[string][]string, buff *bytes.Buffer) []subDigestEntry {
	manifestDigests := manifestListDigests[img.Origin]
	if len(manifestDigests) == 0 {
		manifestDigests = manifestListDigests[img.Source]
	}
	if len(manifestDigests) == 0 {
		return nil
	}
	sourceBase, _, _ := strings.Cut(img.Source, "@")
	entries := make([]subDigestEntry, 0, len(manifestDigests))
	for _, digest := range manifestDigests {
		subSource := sourceBase + "@" + digest
		subDest := subDigestDestination(img.Destination, digest)
		buff.WriteString(subSource + "=" + subDest + "\n")
		entries = append(entries, subDigestEntry{source: subSource, dest: subDest})
	}
	return entries
}

// subDigestDestination returns a digest-pinned destination for a sub-digest entry.
// For docker:// destinations, it strips the tag and appends the sub-digest to avoid
// destination overwrites when multiple architectures map to the same tag.
// For non-docker destinations (oci:, dir:, etc.), the destination is returned as-is.
func subDigestDestination(dest string, digest string) string {
	if !strings.HasPrefix(dest, consts.DockerProtocol) {
		return dest
	}
	destSpec, err := image.ParseRef(dest)
	if err != nil {
		return dest
	}
	return destSpec.Transport + destSpec.Name + "@" + digest
}

func (o *ExecutorSchema) newSystemContextForSource(source string) (*types.SystemContext, error) {
	sysCtx, err := o.Opts.SrcImage.NewSystemContext()
	if err != nil {
		return nil, fmt.Errorf("error creating system context: %w", err)
	}
	if isLocalCacheSource(source, o.Opts.LocalStorageFQDN) {
		sysCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
	}
	return sysCtx, nil
}

// inspectImages fetches each source once for mapping digests and optional blob sizes.
func (o *ExecutorSchema) inspectImages(ctx context.Context, images []v2alpha1.CopyImageSchema, platformFilters map[string][]v2alpha1.InstancePlatformFilter, includeSizes bool) (map[string][]string, map[string]imageSizeResult) {
	results := make(map[string]imageSizeResult, len(images))
	digests := make(map[string][]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	parallelism := o.Opts.ParallelImages
	if parallelism == 0 {
		parallelism = maxParallelImageDownloads
	}
	semaphore := make(chan struct{}, parallelism)
	for _, img := range images {
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return digests, results
		}
		wg.Add(1)
		go func(img v2alpha1.CopyImageSchema) {
			defer wg.Done()
			defer func() { <-semaphore }()
			instances, result := o.inspectImage(ctx, img, platformFilters[img.Origin], includeSizes)
			mu.Lock()
			defer mu.Unlock()
			if len(instances) > 0 {
				digests[img.Source] = instances
			}
			if includeSizes && o.Manifest != nil {
				results[img.Source] = result
			}
		}(img)
	}
	wg.Wait()
	return digests, results
}

func (o *ExecutorSchema) inspectImage(ctx context.Context, img v2alpha1.CopyImageSchema, filters []v2alpha1.InstancePlatformFilter, includeSizes bool) ([]string, imageSizeResult) {
	var result imageSizeResult
	if o.Manifest == nil {
		return nil, result
	}
	sysCtx, err := o.newSystemContextForSource(img.Source)
	var digests []string
	if err == nil {
		digests, result.Blobs, err = o.Manifest.InspectImage(ctx, sysCtx, img.Source, filters, includeSizes)
	}
	if err != nil {
		if includeSizes {
			result.Blobs = nil
			result.Errors = append(result.Errors, fmt.Errorf("error fetching blob sizes: %w", err))
		}
		if !includeSizes || o.Opts.IsDryRunManifestLists {
			o.Log.Warn("unable to inspect manifest for %s: %v", img.Source, err)
		}
	} else if includeSizes && !o.Opts.RemoveSignatures && img.RebuiltTag == "" {
		o.inspectSignatureSizes(ctx, img, sysCtx, &result)
	}
	return digests, result
}

// isLocalCacheSource reports whether source's registry authority equals localStorageFQDN.
// Substring checks on the full reference are unsafe: a remote host like
// "notmirror.local:55000" contains "mirror.local:55000".
func isLocalCacheSource(source, localStorageFQDN string) bool {
	spec, err := image.ParseRef(source)
	return err == nil && localStorageFQDN != "" && spec.Domain == localStorageFQDN
}

// dedupeImagesBySource keeps one sizing row per Source so m2m dual catalog copies
// (cache dest + registry dest) are not fetched/counted twice.
func dedupeImagesBySource(images []v2alpha1.CopyImageSchema) []v2alpha1.CopyImageSchema {
	seen := make(map[string]int, len(images))
	out := make([]v2alpha1.CopyImageSchema, 0, len(images))
	for _, img := range images {
		if idx, ok := seen[img.Source]; ok {
			if out[idx].Origin == "" && img.Origin != "" {
				out[idx].Origin = img.Origin
			}
			if img.Type.IsOperatorCatalog() {
				out[idx].Type = img.Type
			}
			continue
		}
		seen[img.Source] = len(out)
		out = append(out, img)
	}
	return out
}

type sizeEstimate struct {
	Bytes       int64
	Errors      []error
	Unavailable bool
}

type dryRunSizeReport struct {
	TotalDownloadSize sizeEstimate
	LocalStorageSize  sizeEstimate
	TarballSize       sizeEstimate
	ImageCount        int
}

func (s *sizeEstimate) addError(image string, issues ...error) {
	for _, err := range issues {
		s.Errors = append(s.Errors, fmt.Errorf("%s: %w", image, err))
	}
}

// estimateDryRunSize keeps cache availability and archive history independent.
func (o *ExecutorSchema) estimateDryRunSize(ctx context.Context, cs v2alpha1.CollectorSchema, results map[string]imageSizeResult) dryRunSizeReport {
	report := dryRunSizeReport{ImageCount: len(dedupeImagesBySource(cs.AllImages))}
	archiveBlobs := make(map[string][]types.BlobInfo, len(results))
	for source, result := range results {
		archiveBlobs[source] = result.Blobs
		report.TarballSize.addError(source, result.Errors...)
	}
	if len(results) < report.ImageCount {
		report.TarballSize.addError("images", errors.New("image inspection did not finish"))
	}
	checker := o.blobExists
	if checker == nil {
		var close func()
		checker, close = mirror.NewBlobChecker(o.Opts)
		defer close()
	}
	var downloads, cache []v2alpha1.CopyImageSchema
	for _, img := range cs.AllImages {
		if o.Opts.IsMirrorToDisk() || isLocalCacheSource(img.Destination, o.Opts.LocalStorageFQDN) {
			cache = append(cache, img)
		}
		if strings.HasPrefix(img.Source, "docker://") && !isLocalCacheSource(img.Source, o.Opts.LocalStorageFQDN) {
			downloads = append(downloads, img)
		}
	}
	report.TotalDownloadSize = o.computeSizeReport(ctx, downloads, results, checker)
	cacheResults := o.cacheSizeResults(ctx, cache, results)
	report.LocalStorageSize = o.computeSizeReport(ctx, cache, cacheResults, checker)
	if !o.Opts.IsMirrorToDisk() {
		report.TarballSize = sizeEstimate{}
	}
	o.completeSizeReport(ctx, cs, cache, archiveBlobs, cacheResults, &report)
	return report
}

// cacheSizeResults sizes every rebuilt platform without changing the selected-platform inventory.
func (o *ExecutorSchema) cacheSizeResults(ctx context.Context, images []v2alpha1.CopyImageSchema, results map[string]imageSizeResult) map[string]imageSizeResult {
	cacheResults := make(map[string]imageSizeResult, len(results))
	maps.Copy(cacheResults, results)
	for _, img := range images {
		if img.RebuiltTag != "" && strings.HasPrefix(img.Source, consts.OciProtocolTrimmed) {
			_, cacheResults[img.Source] = o.inspectImage(ctx, img, nil, true)
		}
	}
	return cacheResults
}

// computeSizeReport is shared by download and cache estimates. Availability and
// deduplication follow the copy destination; all local cache blobs share a scope.
func (o *ExecutorSchema) computeSizeReport(ctx context.Context, images []v2alpha1.CopyImageSchema, results map[string]imageSizeResult, checker func(context.Context, string, types.BlobInfo) (bool, error)) sizeEstimate {
	var estimate sizeEstimate
	seen := make(map[string]bool)
	for _, img := range images {
		result, ok := results[img.Source]
		if !ok {
			estimate.addError(img.Source, errors.New("source could not be sized"))
			continue
		}
		scope := "cache"
		exists := o.cachedBlobExists
		if !o.Opts.IsMirrorToDisk() && !isLocalCacheSource(img.Destination, o.Opts.LocalStorageFQDN) {
			ref, err := image.ParseRef(img.Destination)
			if err != nil {
				estimate.addError(img.Source, err)
				continue
			}
			scope = ref.Name
			exists = func(blob types.BlobInfo) (bool, error) { return checker(ctx, img.Destination, blob) }
		}
		estimate.addMissing(img.Source, result, scope, seen, exists)
	}
	return estimate
}

func (s *sizeEstimate) addMissing(source string, result imageSizeResult, scope string, seen map[string]bool, exists func(types.BlobInfo) (bool, error)) {
	for _, blob := range result.Blobs {
		key := scope + "@" + blob.Digest.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		present, err := exists(blob)
		if err != nil {
			s.addError(source, err)
			continue
		}
		if !present {
			s.Bytes = archive.AddSize(s.Bytes, blob.Size)
		}
	}
	s.addError(source, result.Errors...)
}

func (o *ExecutorSchema) completeSizeReport(ctx context.Context, cs v2alpha1.CollectorSchema, cacheImages []v2alpha1.CopyImageSchema, blobs map[string][]types.BlobInfo, results map[string]imageSizeResult, report *dryRunSizeReport) {
	metadata := o.estimateCacheMetadata(cacheImages, results, &report.LocalStorageSize)
	if o.Opts.IsMirrorToDisk() {
		var err error
		report.TarballSize.Bytes, err = archive.EstimateSize(ctx, o.Opts, o.LocalStorageDisk, cs, blobs, metadata, o.Config.ArchiveSize, o.Log)
		if err != nil {
			report.TarballSize.Unavailable = true
			report.TarballSize.addError("archive", err)
		}
	}
	for catalog, filtered := range cs.CatalogToFBCMap {
		if filtered.ToRebuild {
			err := errors.New("rebuilt catalog content could not be prepared")
			report.LocalStorageSize.addError(catalog, err)
			if o.Opts.IsMirrorToDisk() {
				report.TarballSize.addError(catalog, err)
			}
		}
	}
}

// inspectSignatureSizes reuses the existing inspector and sigstore tag naming.
// Missing optional signatures contribute zero; inspection failures stay visible.
func (o *ExecutorSchema) inspectSignatureSizes(ctx context.Context, img v2alpha1.CopyImageSchema, sys *types.SystemContext, result *imageSizeResult) {
	ref, err := image.ParseRef(img.Source)
	if err != nil || ref.Transport != consts.DockerProtocol {
		return
	}
	var signatures []types.BlobInfo
	for _, blob := range result.Blobs {
		if !mirror.IsManifest(blob.MediaType) {
			continue
		}
		tag, err := signature.SigstoreAttachmentTag(blob.Digest)
		var blobs []types.BlobInfo
		if err == nil {
			_, blobs, err = o.Manifest.InspectImage(ctx, sys, ref.Transport+ref.Name+":"+tag, nil, true)
		}
		if mirror.IsManifestUnknown(err) {
			continue
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("signature %s could not be sized: %w", tag, err))
			continue
		}
		if len(blobs) > 0 {
			blobs = append([]types.BlobInfo(nil), blobs...)
			last := &blobs[len(blobs)-1]
			annotations := make(map[string]string, len(last.Annotations)+1)
			maps.Copy(annotations, last.Annotations)
			annotations[consts.SignatureFor] = blob.Digest.String()
			last.Annotations = annotations
			signatures = append(signatures, blobs...)
		}
	}
	// Preserve the original top-level manifest at the end for tag metadata.
	result.Blobs = append(signatures, result.Blobs...)
}

func (o *ExecutorSchema) cachedBlobExists(blob types.BlobInfo) (bool, error) {
	if o.LocalStorageDisk == "" {
		return false, errors.New("local cache path is unavailable")
	}
	path, err := archive.BlobPath(o.LocalStorageDisk, blob.Digest)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat cached blob %s: %w", blob.Digest, err)
	}
	if !info.Mode().IsRegular() || info.Size() != blob.Size {
		return false, fmt.Errorf("cached blob %s is not a regular file of the expected size", blob.Digest)
	}
	return true, nil
}

func (o *ExecutorSchema) estimateCacheMetadata(images []v2alpha1.CopyImageSchema, results map[string]imageSizeResult, estimate *sizeEstimate) map[string]int64 {
	if len(images) == 0 {
		return nil
	}
	if o.LocalStorageDisk == "" {
		estimate.addError("cache", errors.New("local cache path is unavailable"))
		return nil
	}
	blobs := make(map[string][]types.BlobInfo, len(images))
	for _, img := range images {
		blobs[img.Source] = results[img.Source].Blobs
	}
	metadata, err := archive.CacheMetadata(images, blobs)
	if err != nil {
		estimate.addError("cache metadata", err)
		return nil
	}
	for path, size := range metadata {
		info, err := os.Stat(filepath.Join(o.LocalStorageDisk, path))
		switch {
		case errors.Is(err, os.ErrNotExist):
			estimate.Bytes = archive.AddSize(estimate.Bytes, size)
		case err != nil:
			estimate.addError("cache metadata", err)
		case !info.Mode().IsRegular():
			estimate.addError("cache metadata", fmt.Errorf("%s is not a regular file", path))
		case size > info.Size():
			estimate.Bytes = archive.AddSize(estimate.Bytes, size-info.Size())
		}
	}
	return metadata
}

func (o *ExecutorSchema) logSizeReport(report dryRunSizeReport) {
	o.Log.Info("📊 Dry-run size estimate:")
	o.logSizeEstimate("Total Download Size", report.TotalDownloadSize)
	o.logSizeEstimate("Local Storage Size", report.LocalStorageSize)
	o.logSizeEstimate("Tarball Size", report.TarballSize)
	o.Log.Info("Images: %d", report.ImageCount)
}

func (o *ExecutorSchema) logSizeEstimate(label string, estimate sizeEstimate) {
	switch {
	case estimate.Unavailable:
		o.Log.Info("%s: unavailable", label)
	case len(estimate.Errors) > 0:
		o.Log.Info("%s: %s (%d bytes; incomplete)", label, units.HumanSize(float64(estimate.Bytes)), estimate.Bytes)
	default:
		o.Log.Info("%s: %s (%d bytes)", label, units.HumanSize(float64(estimate.Bytes)), estimate.Bytes)
	}
	if len(estimate.Errors) > 0 {
		o.Log.Warn("%s: %d issue(s); rerun with --log-level=debug for details", label, len(estimate.Errors))
	}
	for _, err := range estimate.Errors {
		o.Log.Debug("%s — %v", label, err)
	}
}
