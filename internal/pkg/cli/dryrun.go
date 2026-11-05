package cli

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/docker/go-units"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/emoji"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type manifestFetchCacher interface {
	BeginManifestCache()
	EndManifestCache()
}

func (o *ExecutorSchema) DryRun(ctx context.Context, allImages []v2alpha1.CopyImageSchema) error {
	outDir := filepath.Join(o.Opts.Global.WorkingDir, dryRunOutDir)
	os.RemoveAll(outDir)

	err := o.MakeDir.makeDirAll(outDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	// Reuse top-level manifests between list inspection and size estimation.
	if cacher, ok := o.Manifest.(manifestFetchCacher); ok {
		cacher.BeginManifestCache()
		defer cacher.EndManifestCache()
	}

	var manifestListDigests map[string][]string
	if o.Opts.IsDryRunManifestLists {
		o.Log.Info(emoji.LeftPointingMagnifyingGlass+" inspecting %d images for manifest lists...", len(allImages))
		manifestListDigests = o.inspectManifestLists(ctx, allImages)
	}

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

	_, err = mappingTxtFile.Write(buff.Bytes())
	if err != nil {
		return err
	}
	if nbMissingImgs > 0 {
		if err := o.writeMissingImagesFile(outDir, missingImgsBuff.Bytes(), nbMissingImgs, len(allImages)); err != nil {
			return err
		}
	}

	if nbAvailableImgs > 0 {
		o.Log.Info("all %d images required for mirroring are available in local cache. You may proceed with mirroring from disk to disconnected registry", nbAvailableImgs)
	}
	o.Log.Info(emoji.PageFacingUp+" list of all images for mirroring in : %s", mappingTxtFilePath)

	// Generate cluster resources in dry-run mode (skip for mirror-to-disk since target registry is unknown)
	if !o.Opts.IsMirrorToDisk() {
		if err := o.generateClusterResources(ctx, allImages); err != nil {
			o.Log.Warn("Cluster resources generation failed (dry-run mode): %v", err)
		}
	}

	// Size estimation (appended AFTER existing output for backward compatibility)
	if o.Manifest != nil {
		o.logSizeReport(o.estimateDryRunSize(ctx, allImages))
	}

	return nil
}

// estimateDryRunSize builds the dry-run size report for the current workflow mode.
func (o *ExecutorSchema) estimateDryRunSize(ctx context.Context, allImages []v2alpha1.CopyImageSchema) v2alpha1.DryRunSizeReport {
	o.Log.Info(emoji.LeftPointingMagnifyingGlass + " estimating image sizes...")
	if o.Opts.IsDiskToMirror() {
		// d2m: all Sources are local-cache; size comes from archive tarballs on disk.
		report := v2alpha1.DryRunSizeReport{ImageCount: len(allImages)}
		tarballSize, tarballErr := computeTarballFilesSize(o.Opts.Global.From)
		if tarballErr != nil {
			o.Log.Warn("unable to read archive for size estimate; tarball size excluded")
			report.SizeUnavailable = true
			return report
		}
		if o.Opts.Global.From != "" && tarballSize == 0 {
			o.Log.Warn("no archive tar files found; size estimate does not include tarball")
		}
		report.TotalSize = tarballSize
		return report
	}

	// m2m/m2d: size from remote manifests (skip local-cache Sources).
	sizingImages := dedupeImagesBySource(filterLocalCacheSourceImages(allImages, o.Opts.LocalStorageFQDN))
	sizeResults := o.estimateImageSizes(ctx, sizingImages, o.DryRunPlatformFilters)
	report := computeSizeReport(sizeResults)
	report.TotalSize = adjustSizeForWorkflow(report.TotalSize, o.Opts.Mode, sizeResults, sizingImages, 0)
	return report
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

// inspectManifestLists concurrently inspects all images to identify manifest lists
// and returns a map of source references to their sub-manifest digests.
// Concurrency is bounded via a semaphore to avoid overwhelming registries.
func (o *ExecutorSchema) inspectManifestLists(ctx context.Context, images []v2alpha1.CopyImageSchema) map[string][]string {
	manifestListDigests := make(map[string][]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	parallelism := o.Opts.ParallelImages
	if parallelism == 0 {
		parallelism = maxParallelImageDownloads
	}
	semaphore := make(chan struct{}, parallelism)

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, img := range images {
		if cancelCtx.Err() != nil {
			break
		}

		semaphore <- struct{}{}

		wg.Add(1)
		go func(source string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			var digests []string
			sysCtx, err := o.newSystemContextForSource(source)
			if err == nil {
				digests, err = o.Manifest.GetManifestListDigests(cancelCtx, sysCtx, source)
			}
			if err != nil {
				o.Log.Warn("unable to inspect manifest for %s: %v", source, err)
				return
			}
			if len(digests) > 0 {
				mu.Lock()
				manifestListDigests[source] = digests
				mu.Unlock()
			}
		}(img.Source)
	}
	wg.Wait()
	return manifestListDigests
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

// estimateImageSizes concurrently fetches blob sizes for all images.
// Errors are captured per-image.
func (o *ExecutorSchema) estimateImageSizes(ctx context.Context, images []v2alpha1.CopyImageSchema, platformFilters map[string][]v2alpha1.InstancePlatformFilter) []v2alpha1.ImageSizeResult {
	results := make([]v2alpha1.ImageSizeResult, len(images))
	var wg sync.WaitGroup

	parallelism := o.Opts.ParallelImages
	if parallelism == 0 {
		parallelism = maxParallelImageDownloads
	}
	semaphore := make(chan struct{}, parallelism)

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i, img := range images {
		if cancelCtx.Err() != nil {
			break
		}

		semaphore <- struct{}{}

		wg.Add(1)
		go func(idx int, source, origin string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			result := v2alpha1.ImageSizeResult{Source: source}

			sysCtx, err := o.newSystemContextForSource(source)
			if err != nil {
				result.Error = fmt.Sprintf("error creating system context: %v", err)
				results[idx] = result
				return
			}

			// Platform filters are keyed by Origin, not Source (Source may be a local-cache ref).
			blobs, err := o.Manifest.ImageBlobSizes(cancelCtx, sysCtx, source, platformFilters[origin])
			if err != nil {
				result.Error = fmt.Sprintf("error fetching blob sizes: %v", err)
				results[idx] = result
				return
			}

			result.Blobs = blobs
			results[idx] = result
		}(i, img.Source, img.Origin)
	}
	wg.Wait()
	return results
}

// computeSizeReport deduplicates blobs globally and computes the total unique
// compressed content size. Errored images are recorded
func computeSizeReport(results []v2alpha1.ImageSizeResult) v2alpha1.DryRunSizeReport {
	var report v2alpha1.DryRunSizeReport
	blobSeen := make(map[string]int64)
	for _, r := range results {
		if r.Source == "" {
			continue // unprocessed slot (e.g., context cancelled before goroutine ran)
		}
		if r.Error != "" {
			report.Errors = append(report.Errors, v2alpha1.SizeReportError{
				Image: r.Source,
				Error: r.Error,
			})
			continue
		}
		report.ImageCount++
		for _, blob := range r.Blobs {
			blobSeen[blob.Digest] = blob.Size
		}
	}
	report.TotalSize = sumMapValues(blobSeen)
	return report
}

// logSizeReport prints the size estimation summary (aggregates only).
func (o *ExecutorSchema) logSizeReport(report v2alpha1.DryRunSizeReport) {
	o.Log.Info("📊 Dry-run size estimate:")
	switch {
	case report.SizeUnavailable:
		o.Log.Info("Total estimated size: unavailable")
	case len(report.Errors) > 0:
		// Partial total is a lower bound — do not present it as a complete estimate.
		totalHuman := units.HumanSize(float64(report.TotalSize))
		o.Log.Info("Total estimated size: %s (%d bytes; incomplete)", totalHuman, report.TotalSize)
	default:
		totalHuman := units.HumanSize(float64(report.TotalSize))
		o.Log.Info("Total estimated size: %s (%d bytes)", totalHuman, report.TotalSize)
	}
	o.Log.Info("Images: %d", report.ImageCount)
	if len(report.Errors) > 0 {
		o.Log.Warn("%d image(s) could not be sized", len(report.Errors))
	}
}

// isLocalCacheSource reports whether source's registry authority equals localStorageFQDN.
// Substring checks on the full reference are unsafe: a remote host like
// "notmirror.local:55000" contains "mirror.local:55000".
func isLocalCacheSource(source, localStorageFQDN string) bool {
	if localStorageFQDN == "" {
		return false
	}
	spec, err := image.ParseRef(source)
	if err != nil {
		return false
	}
	return spec.Domain == localStorageFQDN
}

// filterLocalCacheSourceImages drops images sourced from the local cache
// (e.g. rebuilt catalogs in m2m) so dry-run does not try to size them remotely.
func filterLocalCacheSourceImages(images []v2alpha1.CopyImageSchema, localStorageFQDN string) []v2alpha1.CopyImageSchema {
	if localStorageFQDN == "" {
		return images
	}
	filtered := make([]v2alpha1.CopyImageSchema, 0, len(images))
	for _, img := range images {
		if !isLocalCacheSource(img.Source, localStorageFQDN) {
			filtered = append(filtered, img)
		}
	}
	return filtered
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

// adjustSizeForWorkflow adjusts the raw unique blob size based on workflow:
//
//   - m2m: image content + local cache (operator catalogs only)
//   - m2d: image content × 2 (cache + tarball coexist on disk)
//   - d2m: exact tarball size (all Sources are local-cache-based and filtered
//     out, so blob-level estimation is not done; the tarball is measured from disk)
func adjustSizeForWorkflow(rawSize int64, mode string, results []v2alpha1.ImageSizeResult, images []v2alpha1.CopyImageSchema, tarballSize int64) int64 {
	switch mode {
	case mirror.MirrorToMirror:
		return saturatingAdd(rawSize, computeOperatorCatalogCacheSize(results, images))
	case mirror.MirrorToDisk:
		return saturatingAdd(rawSize, rawSize)
	case mirror.DiskToMirror:
		return tarballSize
	default:
		return rawSize
	}
}

// computeOperatorCatalogCacheSize returns the deduplicated blob size of operator catalog images only.
// In m2m, catalogs are cached locally for the rebuild/delete workflow in addition to being pushed to the target registry.
// Matching is done by Source key to avoid fragile index-based correlation.
func computeOperatorCatalogCacheSize(results []v2alpha1.ImageSizeResult, images []v2alpha1.CopyImageSchema) int64 {
	catalogSources := make(map[string]bool, len(images))
	for _, img := range images {
		if img.Type.IsOperatorCatalog() {
			catalogSources[img.Source] = true
		}
	}
	blobSeen := make(map[string]int64)
	for _, r := range results {
		if r.Source == "" || r.Error != "" {
			continue
		}
		if catalogSources[r.Source] {
			for _, blob := range r.Blobs {
				blobSeen[blob.Digest] = blob.Size
			}
		}
	}
	return sumMapValues(blobSeen)
}

func sumMapValues(m map[string]int64) int64 {
	var total int64
	for _, v := range m {
		total = saturatingAdd(total, v)
	}
	return total
}

// saturatingAdd adds sizes without wrapping past MaxInt64.
func saturatingAdd(a, b int64) int64 {
	if a < 0 {
		a = 0
	}
	if b < 0 {
		b = 0
	}
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

var mirrorTarPattern = regexp.MustCompile(`^mirror_[0-9]{6}\.tar$`)

// computeTarballFilesSize measures the total size of existing mirror_NNNNNN.tar files in the given directory.
// Used for d2m to get the exact tarball size.
func computeTarballFilesSize(fromDir string) (int64, error) {
	if fromDir == "" {
		return 0, nil
	}
	dir := strings.TrimPrefix(fromDir, consts.FileProtocol)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("unable to read archive directory %s: %w", dir, err)
	}
	var total int64
	for _, e := range entries {
		if e.IsDir() || !mirrorTarPattern.MatchString(e.Name()) {
			continue
		}
		// os.Stat follows symlinks so a mirror_NNNNNN.tar link contributes the
		// target tarball size (matches how the archive reader opens the path).
		info, err := os.Stat(filepath.Join(dir, e.Name()))
		if err != nil {
			return 0, fmt.Errorf("unable to stat archive file: %w", err)
		}
		if info.IsDir() {
			continue
		}
		total = saturatingAdd(total, info.Size())
	}
	return total, nil
}
