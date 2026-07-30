package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/emoji"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
)

func (o *ExecutorSchema) DryRun(ctx context.Context, allImages []v2alpha1.CopyImageSchema) error {
	outDir := filepath.Join(o.Opts.Global.WorkingDir, dryRunOutDir)
	os.RemoveAll(outDir)

	err := o.MakeDir.makeDirAll(outDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
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
	var buff bytes.Buffer
	var missingImgsBuff bytes.Buffer
	nbMissingImgs, nbAvailableImgs := o.processImages(ctx, allImages, manifestListDigests, &buff, &missingImgsBuff)

	_, err = mappingTxtFile.Write(buff.Bytes())
	if err != nil {
		return fmt.Errorf("failed to write mapping file: %w", err)
	}
	if err := o.writeMissingImagesFile(outDir, &missingImgsBuff, nbMissingImgs, len(allImages)); err != nil {
		return err
	}

	if nbAvailableImgs > 0 {
		o.Log.Info("all %d images required for mirroring are available in local cache. You may proceed with mirroring from disk to disconnected registry", nbAvailableImgs)
	}
	o.Log.Info(emoji.PageFacingUp+" list of all images for mirroring in : %s", mappingTxtFilePath)

	// Generate cluster resources in dry-run mode (skip for mirror-to-disk since target registry is unknown)
	if !o.Opts.IsMirrorToDisk() {
		if err := o.generateClusterResources(ctx, allImages); err != nil {
			// In dry-run mode, log cluster resource generation errors as warnings instead of failing
			o.Log.Warn("Cluster resources generation failed (dry-run mode): %v", err)
		}
	}

	return nil
}

// writeMissingImagesFile creates the missing.txt file if there are missing images
func (o *ExecutorSchema) writeMissingImagesFile(outDir string, missingImgsBuff *bytes.Buffer, nbMissingImgs, totalImgs int) error {
	if nbMissingImgs == 0 {
		return nil
	}

	missingImgsFilePath := filepath.Join(outDir, missingImgsFile)
	missingImgsTxtFile, err := os.Create(missingImgsFilePath)
	if err != nil {
		return fmt.Errorf("failed to create missing images file: %w", err)
	}
	defer missingImgsTxtFile.Close()

	_, err = missingImgsTxtFile.Write(missingImgsBuff.Bytes())
	if err != nil {
		return fmt.Errorf("failed to write missing images file: %w", err)
	}

	o.Log.Warn(emoji.Warning+"  %d/%d images necessary for mirroring are not available in the cache.", nbMissingImgs, totalImgs)
	o.Log.Warn("List of missing images in : %s.\nplease re-run the mirror to disk process", missingImgsFilePath)
	return nil
}

// processImages processes all images and returns the count of missing and available images
func (o *ExecutorSchema) processImages(ctx context.Context, allImages []v2alpha1.CopyImageSchema, manifestListDigests map[string][]string, buff, missingImgsBuff *bytes.Buffer) (int, int) {
	nbMissingImgs := 0
	nbAvailableImgs := 0
	for _, img := range allImages {
		missing := o.processImageForDryRun(ctx, img, manifestListDigests, buff, missingImgsBuff)
		if missing {
			nbMissingImgs++
		} else if o.Opts.IsMirrorToDisk() {
			nbAvailableImgs++
		}
	}
	return nbMissingImgs, nbAvailableImgs
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
	if o.Opts.LocalStorageFQDN != "" && strings.Contains(source, o.Opts.LocalStorageFQDN) {
		sysCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
	}
	return sysCtx, nil
}
