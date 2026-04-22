package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	imgmanifest "go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/transports/alltransports"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/emoji"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
)

func (o *ExecutorSchema) DryRun(ctx context.Context, allImages []v2alpha1.CopyImageSchema, preCollectedManifestLists map[string][]string) error {
	// set up location of logs dir
	outDir := filepath.Join(o.Opts.Global.WorkingDir, dryRunOutDir)
	// clean up logs directory
	os.RemoveAll(outDir)

	// create logs directory
	err := o.MakeDir.makeDirAll(outDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	// Inspect only images not already classified during collection.
	var remaining []v2alpha1.CopyImageSchema
	for _, img := range allImages {
		if _, found := preCollectedManifestLists[img.Origin]; !found {
			remaining = append(remaining, img)
		}
	}

	o.Log.Info(emoji.LeftPointingMagnifyingGlass+" inspecting %d remaining images for manifest lists (%d already detected during collection)...",
		len(remaining), len(allImages)-len(remaining))
	runtimeDigests := o.inspectManifestLists(ctx, remaining)

	// Merge pre-collected and runtime manifest list results.
	manifestListDigests := make(map[string][]string, len(preCollectedManifestLists)+len(runtimeDigests))
	for k, v := range preCollectedManifestLists {
		manifestListDigests[k] = v
	}
	for k, v := range runtimeDigests {
		manifestListDigests[k] = v
	}

	// creating file for storing list of cached images
	mappingTxtFilePath := filepath.Join(outDir, mappingFile)
	mappingTxtFile, err := os.Create(mappingTxtFilePath)
	if err != nil {
		return err
	}
	defer mappingTxtFile.Close()
	imagesAvailable := map[string]bool{}
	nbMissingImgs := 0
	var buff bytes.Buffer
	var missingImgsBuff bytes.Buffer

	for _, img := range allImages {
		buff.WriteString(img.Source + "=" + img.Destination + "\n")

		// Collect sub-digest source=destination pairs
		type subDigestEntry struct{ source, dest string }
		var subDigestEntries []subDigestEntry

		// Look up manifest list sub-digests: check both by Origin (pre-collected
		// during operator collection) and by Source (detected at runtime).
		manifestDigests := manifestListDigests[img.Origin]
		if len(manifestDigests) == 0 {
			manifestDigests = manifestListDigests[img.Source]
		}
		if len(manifestDigests) > 0 {
			// This is a manifest list, write each sub-digest with digest-pinned destination
			sourceBase, _, _ := strings.Cut(img.Source, "@")
			for _, digest := range manifestDigests {
				subSource := sourceBase + "@" + digest
				subDest := subDigestDestination(img.Destination, digest)
				buff.WriteString(subSource + "=" + subDest + "\n")
				subDigestEntries = append(subDigestEntries, subDigestEntry{source: subSource, dest: subDest})
			}
		}

		if o.Opts.IsMirrorToDisk() {
			exists, err := o.Mirror.Check(ctx, img.Destination, o.Opts, false)
			if err != nil {
				o.Log.Debug("unable to check existence of %s in local cache: %v", img.Destination, err)
			}
			if err != nil || !exists {
				missingImgsBuff.WriteString(img.Source + "=" + img.Destination + "\n")
				nbMissingImgs++
				// Also include sub-digest entries in missing list
				for _, sub := range subDigestEntries {
					missingImgsBuff.WriteString(sub.source + "=" + sub.dest + "\n")
				}
			}
		}
	}

	_, err = mappingTxtFile.Write(buff.Bytes())
	if err != nil {
		return err
	}
	if nbMissingImgs > 0 {
		// creating file for storing list of cached images
		missingImgsFilePath := filepath.Join(outDir, missingImgsFile)
		missingImgsTxtFile, err := os.Create(missingImgsFilePath)
		if err != nil {
			return err
		}
		defer missingImgsTxtFile.Close()
		_, err = missingImgsTxtFile.Write(missingImgsBuff.Bytes())
		if err != nil {
			return err
		}
		o.Log.Warn(emoji.Warning+"  %d/%d images necessary for mirroring are not available in the cache.", nbMissingImgs, len(allImages))
		o.Log.Warn("List of missing images in : %s.\nplease re-run the mirror to disk process", missingImgsFilePath)
	}

	if len(imagesAvailable) > 0 {
		o.Log.Info("all %d images required for mirroring are available in local cache. You may proceed with mirroring from disk to disconnected registry", len(imagesAvailable))
	}
	o.Log.Info(emoji.PageFacingUp+" list of all images for mirroring in : %s", mappingTxtFilePath)
	return nil
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
		select {
		case <-cancelCtx.Done():
			break
		default:
		}

		semaphore <- struct{}{}

		wg.Add(1)
		go func(source string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			digests, err := o.getManifestListDigests(cancelCtx, source)
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

// getManifestListDigests inspects the source image to check if it's a manifest list
// and returns the sub-manifest digests. Works with any transport supported by
// containers/image (docker://, oci:, etc.) via alltransports.ParseImageName.
// Returns a slice of digest strings (e.g., ["sha256:abc...", "sha256:def..."]) or nil if not a manifest list.
func (o *ExecutorSchema) getManifestListDigests(ctx context.Context, source string) ([]string, error) {
	srcRef, err := alltransports.ParseImageName(source)
	if err != nil {
		// Retry with docker:// prefix for sources without transport (e.g., Cincinnati sources)
		srcRef, err = alltransports.ParseImageName(consts.DockerProtocol + source)
		if err != nil {
			return nil, fmt.Errorf("error parsing image name %s: %w", source, err)
		}
	}

	sysCtx, err := o.Opts.SrcImage.NewSystemContext()
	if err != nil {
		return nil, fmt.Errorf("error creating system context: %w", err)
	}
	// The local cache registry is HTTP-only; ensure we skip TLS verification for it.
	if o.Opts.LocalStorageFQDN != "" && strings.Contains(source, o.Opts.LocalStorageFQDN) {
		sysCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
	}

	imgSrc, err := srcRef.NewImageSource(ctx, sysCtx)
	if err != nil {
		return nil, fmt.Errorf("error creating image source for %s: %w", source, err)
	}
	defer imgSrc.Close()

	manifestBytes, manifestType, err := imgSrc.GetManifest(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error getting manifest for %s: %w", source, err)
	}

	if !imgmanifest.MIMETypeIsMultiImage(manifestType) {
		return nil, nil
	}

	list, err := imgmanifest.ListFromBlob(manifestBytes, manifestType)
	if err != nil {
		return nil, fmt.Errorf("error parsing manifest list for %s: %w", source, err)
	}

	instances := list.Instances()
	digests := make([]string, 0, len(instances))
	for _, instance := range instances {
		digests = append(digests, instance.String())
	}
	return digests, nil
}
