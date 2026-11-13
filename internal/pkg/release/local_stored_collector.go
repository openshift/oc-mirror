package release

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/vbauerster/mpb/v8"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	"github.com/openshift/oc-mirror/v2/internal/pkg/imagebuilder"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/parser"
	"github.com/openshift/oc-mirror/v2/internal/pkg/spinners"
)

type LocalStorageCollector struct {
	CollectorInterface
	Log              clog.PluggableLoggerInterface
	LogsDir          string
	Mirror           mirror.MirrorInterface
	Manifest         manifest.ManifestInterface
	Config           v2alpha1.ImageSetConfiguration
	Opts             mirror.CopyOptions
	Cincinnati       CincinnatiInterface
	LocalStorageFQDN string
	ImageBuilder     imagebuilder.ImageBuilderInterface
	Releases         []string
	GraphDataImage   string
	destReg          string
}

func (o LocalStorageCollector) destinationRegistry() string {
	if o.destReg == "" {
		if o.Opts.Mode == mirror.DiskToMirror || o.Opts.Mode == mirror.MirrorToMirror {
			o.destReg = strings.TrimPrefix(o.Opts.Destination, consts.DockerProtocol)
		} else {
			o.destReg = o.LocalStorageFQDN
		}
	}
	return o.destReg
}

// getPlatformFilters returns the platform filters for the new Platforms field.
// Returns nil when Platforms is not set — the Architectures path uses Cincinnati directly.
func (o LocalStorageCollector) getPlatformFilters() ([]v2alpha1.InstancePlatformFilter, error) {
	if err := v2alpha1.ValidatePlatforms(o.Config.Mirror.Platform.Platforms); err != nil {
		return nil, err
	}
	return o.Config.Mirror.Platform.Platforms, nil
}

func (o *LocalStorageCollector) ReleaseImageCollector(ctx context.Context) (v2alpha1.CollectorSchema, error) {
	// we just care for 1 platform release, in order to read release images
	o.Opts.MultiArch = "system"
	if len(o.Config.Mirror.Platform.Platforms) > 0 {
		// New Platforms field: download all arch payloads from the multi-arch release so that
		// ensureReleaseInOCIFormat can extract image-references with manifest-list component digests.
		o.Opts.MultiArch = "all"
	}
	o.Log.Debug(collectorPrefix+"setting copy option o.Opts.MultiArch=%s when collecting releases image", o.Opts.MultiArch)

	var err error
	var allImages []v2alpha1.CopyImageSchema
	switch {
	case o.Opts.IsMirrorToDisk():
		fallthrough
	case o.Opts.IsMirrorToMirror():
		allImages, err = o.collectImageFromMirror(ctx)
	case o.Opts.IsDiskToMirror():
		allImages, err = o.collectImageFromDisk(ctx)
	default:
		err = fmt.Errorf("release collector: invalid mirror mode %s", o.Opts.Mode)
	}
	if err != nil {
		return v2alpha1.CollectorSchema{}, err
	}

	// OCPBUGS-43275: deduplicating
	slices.SortFunc(allImages, compareByOriginSourceDest)
	allImages = slices.Compact(allImages)

	// TODO probably does not need to loop all the images, it would be possible to check the image type, if it is from a release, apply always the same os/arch saved somewhere else.
	// TBD with team - Keeping in this way the advantages is that we create a standard and don't treat releases in a different way
	// The disadvantage is performance
	cs := v2alpha1.CollectorSchema{AllImages: allImages}
	platforms, err := o.getPlatformFilters()
	if err != nil {
		return v2alpha1.CollectorSchema{}, fmt.Errorf("invalid platform configuration: %w", err)
	}
	if len(platforms) > 0 {
		cs.PlatformFilters = buildReleasePlatformFilters(allImages, platforms)
	}
	return cs, nil
}

// buildReleasePlatformFilters builds a map from image origin to platform filters,
// excluding Cincinnati graph images which must always be copied whole.
func buildReleasePlatformFilters(allImages []v2alpha1.CopyImageSchema, platforms []v2alpha1.InstancePlatformFilter) map[string][]v2alpha1.InstancePlatformFilter {
	filters := make(map[string][]v2alpha1.InstancePlatformFilter, len(allImages))
	for _, img := range allImages {
		if img.Type != v2alpha1.TypeCincinnatiGraph {
			filters[img.Origin] = platforms
		}
	}
	return filters
}

// collects release images from a mirror.
// all errors will be propagated to the caller; no redundant error logging to console
func (o *LocalStorageCollector) collectImageFromMirror(ctx context.Context) ([]v2alpha1.CopyImageSchema, error) {
	releases, err := o.Cincinnati.GetReleaseReferenceImages(ctx)
	if err != nil {
		return []v2alpha1.CopyImageSchema{}, err
	}

	var allImages []v2alpha1.CopyImageSchema

	// prepare progress bar
	p := mpb.New(mpb.PopCompletedMode(), mpb.ContainerOptional(mpb.WithOutput(io.Discard), !o.Opts.Global.IsTerminal))

	for _, value := range releases {
		spinner := spinners.AddSpinner(p, "Collecting release "+value.Source)

		hld := strings.Split(value.Source, "/")
		releaseRepoAndTag := hld[len(hld)-1]
		releaseTag := releaseRepoAndTag[strings.Index(releaseRepoAndTag, ":")+1:]

		allRelatedImages, err := o.collectReleaseImages(ctx, value)
		if err != nil {
			logCollectionError(o.Log, spinner, o.Opts.Global.IsTerminal, value.Source, err)
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf("%s%w", collectorPrefix, err)
		}

		// add the release image itself
		allRelatedImages = append(allRelatedImages, v2alpha1.RelatedImage{
			Image: value.Source,
			Name:  value.Source,
			Type:  v2alpha1.TypeOCPRelease,
		})
		tmpAllImages, err := o.prepareM2DCopyBatch(allRelatedImages, releaseTag)
		if err != nil {
			logCollectionError(o.Log, spinner, o.Opts.Global.IsTerminal, value.Source, err)
			return []v2alpha1.CopyImageSchema{}, err
		}
		allImages = append(allImages, tmpAllImages...)
		spinner.Increment()
		if !o.Opts.Global.IsTerminal {
			o.Log.Info("Success collecting release %s", value.Source)
		}
	}
	p.Wait()
	if o.Config.Mirror.Platform.Graph {
		graphImage, err := o.handleGraphImage(ctx)
		if err != nil {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, fmt.Sprintf("error processing graph image: %v", err))
		} else if graphImage.Source != "" {
			allImages = append(allImages, graphImage)
		}

	}

	return allImages, nil
}

// collects related images from a release
func (o *LocalStorageCollector) collectReleaseImages(ctx context.Context, release v2alpha1.CopyImageSchema) ([]v2alpha1.RelatedImage, error) {
	hld := strings.Split(release.Source, "/")
	releaseRepoAndTag := hld[len(hld)-1]
	releaseTag := releaseRepoAndTag[strings.Index(releaseRepoAndTag, ":")+1:]
	imageIndexDir := strings.ReplaceAll(releaseRepoAndTag, ":", "/")
	cacheDir := filepath.Join(o.Opts.Global.WorkingDir, releaseImageExtractDir, imageIndexDir)
	dir := filepath.Join(o.Opts.Global.WorkingDir, releaseImageDir, imageIndexDir)

	if err := o.ensureReleaseInOCIFormat(ctx, release, dir); err != nil {
		return []v2alpha1.RelatedImage{}, err
	}

	img, err := o.Manifest.GetOCIImageFromIndex(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to find release image in index: %w", err)
	}

	dgest, err := img.Digest()
	if err != nil {
		return nil, fmt.Errorf("failed to get release image digest: %w", err)
	}
	o.Log.Debug(collectorPrefix+"image manifest digest %s", dgest.String())

	if err := o.Manifest.ExtractOCILayers(img, cacheDir, releaseManifests); err != nil {
		return []v2alpha1.RelatedImage{}, fmt.Errorf("extract release image %q manifests: %w", dgest.String(), err)
	}
	o.Log.Debug("extracted layer %s ", cacheDir)

	// overkill but its used for consistency
	releaseDir := filepath.Join(cacheDir, releaseImageExtractFullPath)
	allRelatedImages, err := o.Manifest.GetReleaseSchema(releaseDir)
	if err != nil {
		return []v2alpha1.RelatedImage{}, err
	}

	if o.Config.Mirror.Platform.KubeVirtContainer {
		kvImages, err := o.collectKubeVirtImages(cacheDir)
		if err != nil {
			return []v2alpha1.RelatedImage{}, err
		}
		if err := checkKubeVirtStreamTags(kvImages, releaseTag); err != nil {
			return []v2alpha1.RelatedImage{}, err
		}
		for _, kvImage := range kvImages {
			allRelatedImages = append(allRelatedImages, kvImage.RelatedImage)
		}
	}

	return allRelatedImages, nil
}

func (o *LocalStorageCollector) ensureReleaseInOCIFormat(ctx context.Context, release v2alpha1.CopyImageSchema, dir string) error {
	_, err := os.Stat(dir)
	if err == nil {
		o.Log.Debug(collectorPrefix+"release-images index directory alredy exists %s", dir)
		return nil
	}
	o.Log.Debug(collectorPrefix+"copying  release image %s ", release.Source)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create oci dir: %w", err)
	}

	optsCopy := o.Opts
	optsCopy.Stdout = io.Discard
	// Converting docker→OCI changes the digest, which invalidates existing signatures.
	// This copy is for metadata extraction only, so signatures are not needed.
	optsCopy.RemoveSignatures = true

	src := consts.DockerProtocol + release.Source
	dest := consts.OciProtocolTrimmed + dir

	if err := o.Mirror.Run(ctx, src, dest, "copy", &optsCopy); err != nil {
		return fmt.Errorf("copy release index image: %w", err)
	}
	o.Log.Debug(collectorPrefix+"copied release index image %s ", release.Source)

	return nil
}

// collects release images from the disk
// all errors will be propagated to the caller; no redundant error logging to console
func (o *LocalStorageCollector) collectImageFromDisk(ctx context.Context) ([]v2alpha1.CopyImageSchema, error) {
	var allImages []v2alpha1.CopyImageSchema

	releaseImages, releaseFolders, err := o.identifyReleases(ctx)
	if err != nil {
		return allImages, err
	}

	o.Releases = make([]string, 0, len(releaseImages))
	for _, img := range releaseImages {
		o.Releases = append(o.Releases, img.Image)
	}

	for _, releaseImg := range releaseImages {
		monoReleaseSlice, err := o.prepareMonoReleaseBatch(releaseImg)
		if err != nil {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
		}
		allImages = append(allImages, monoReleaseSlice...)
	}

	for _, releaseDir := range releaseFolders {
		releaseCopyImages, err := o.prepareReleaseBatchFromDir(ctx, releaseDir)
		if err != nil {
			return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
		}
		allImages = append(allImages, releaseCopyImages...)
	}

	if !o.Config.Mirror.Platform.Graph {
		return allImages, nil
	}

	o.Log.Debug("adding graph data image")
	graphCopy, err := o.prepareGraphImage(ctx)
	if err != nil {
		return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
	}
	o.GraphDataImage = graphCopy.Destination
	allImages = append(allImages, graphCopy)

	return allImages, nil
}

func (o *LocalStorageCollector) prepareMonoReleaseBatch(releaseImg v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	releaseRef, err := image.ParseRef(releaseImg.Image)
	if err != nil {
		return []v2alpha1.CopyImageSchema{}, err
	}
	if releaseRef.Tag == "" && len(releaseRef.Digest) == 0 {
		return []v2alpha1.CopyImageSchema{}, fmt.Errorf("release image %s doesn't have a tag or digest", releaseImg.Image)
	}
	tag := releaseRef.Tag
	if releaseRef.Tag == "" && len(releaseRef.Digest) > 0 {
		tag = releaseRef.Digest
	}
	return o.prepareD2MCopyBatch([]v2alpha1.RelatedImage{releaseImg}, tag)
}

func (o *LocalStorageCollector) prepareReleaseBatchFromDir(ctx context.Context, releaseDir string) ([]v2alpha1.CopyImageSchema, error) {
	releaseTag := filepath.Base(releaseDir)

	// get all release images from manifest (json)
	imageReferencesFile := filepath.Join(releaseDir, releaseManifests, imageReferences)
	releaseRelatedImages, err := o.Manifest.GetReleaseSchema(imageReferencesFile)
	if err != nil {
		return []v2alpha1.CopyImageSchema{}, err
	}

	releaseCopies, err := o.prepareD2MCopyBatch(releaseRelatedImages, releaseTag)
	if err != nil || !o.Config.Mirror.Platform.KubeVirtContainer {
		return releaseCopies, err
	}

	kvImages, err := o.collectKubeVirtImages(releaseDir)
	if err != nil {
		return []v2alpha1.CopyImageSchema{}, err
	}
	if err := checkKubeVirtStreamTags(kvImages, releaseTag); err != nil {
		return []v2alpha1.CopyImageSchema{}, err
	}
	kvCopies, err := o.prepareKubeVirtD2MCopyBatch(ctx, kvImages, releaseTag)
	if err != nil {
		return []v2alpha1.CopyImageSchema{}, err
	}
	return append(releaseCopies, kvCopies...), nil
}

// prepareKubeVirtD2MCopyBatch prepares the disk to mirror copies of the kubevirt images.
// Archives created before stream collections were supported only hold the images of the
// legacy single stream, under their legacy names. When the cache does not hold the image
// of a stream under the name of the stream, the image held under its legacy name is copied
// instead, but only when its digest is the digest of the image of the stream: the
// destination keeps the name of the stream, so that it always matches its content.
// Otherwise the copy is kept unchanged and fails, like for any image missing from the archive.
// The cache is not read in delete mode.
func (o LocalStorageCollector) prepareKubeVirtD2MCopyBatch(ctx context.Context, images []kubeVirtImage, releaseTag string) ([]v2alpha1.CopyImageSchema, error) {
	relatedImages := make([]v2alpha1.RelatedImage, 0, len(images))
	for _, img := range images {
		relatedImages = append(relatedImages, img.RelatedImage)
	}
	copies, err := o.prepareD2MCopyBatch(relatedImages, releaseTag)
	if err != nil {
		return nil, err
	}
	// The copies are matched with their images by position.
	if len(copies) != len(images) {
		return nil, fmt.Errorf("kubevirt: %d disk to mirror copies prepared for %d images", len(copies), len(images))
	}
	if o.Opts.IsDeleteMode() {
		return copies, nil
	}

	// legacySources maps the legacy names to the sources of the legacy single stream images.
	legacySources := map[string]string{}
	for i, img := range images {
		if img.stream == "" {
			legacySources[img.Name] = copies[i].Source
		}
	}
	for i, img := range images {
		if img.stream != "" {
			copies[i].Source = o.kubeVirtD2MSource(ctx, img, copies[i].Source, legacySources[img.legacyName])
		}
	}
	return copies, nil
}

// kubeVirtD2MSource returns the source, in the local cache, of the disk to mirror copy of
// the image of a stream collection (see prepareKubeVirtD2MCopyBatch). legacySource is the
// source of the legacy single stream image with the same legacy name, if any.
func (o LocalStorageCollector) kubeVirtD2MSource(ctx context.Context, img kubeVirtImage, streamSource, legacySource string) string {
	if exists, err := o.imageExists(ctx, streamSource); err == nil && exists {
		return streamSource
	}
	outcome := kubeVirtLegacyImageUnavailable
	if legacySource != "" {
		matches, err := o.cachedImageMatchesDigestRef(ctx, legacySource, img.Image)
		switch {
		case err == nil && matches:
			o.Log.Info("kubevirt: copying %s, archived under the legacy name, for stream %q (arch: %v, %s)",
				legacySource, img.stream, img.arch, kubeVirtLegacyDigestMatched)
			return legacySource
		case err == nil:
			outcome = kubeVirtLegacyDigestNotMatched
		}
	}
	o.Log.Warn("kubevirt: the archive has no image %s for stream %q (arch: %v, %s): "+
		"the archive was probably created by an older version of oc-mirror", streamSource, img.stream, img.arch, outcome)
	return streamSource
}

// Outcome codes of the lookup of a stream collection image archived under its legacy name.
const (
	kubeVirtLegacyDigestMatched    = "legacy-digest-matched"
	kubeVirtLegacyDigestNotMatched = "legacy-digest-not-matched"
	kubeVirtLegacyImageUnavailable = "legacy-image-unavailable"
)

// cachedImageMatchesDigestRef reports whether the image of the local cache has the
// sha256 manifest digest of the digest reference.
func (o LocalStorageCollector) cachedImageMatchesDigestRef(ctx context.Context, cachedRef, digestRef string) (bool, error) {
	spec, err := image.ParseRef(digestRef)
	if err != nil {
		return false, err
	}
	if spec.Algorithm != "sha256" || spec.Digest == "" {
		return false, nil
	}
	cachedDigest, err := o.cachedImageDigest(ctx, cachedRef)
	if err != nil {
		return false, err
	}
	return cachedDigest == spec.Digest, nil
}

func (o *LocalStorageCollector) prepareGraphImage(ctx context.Context) (v2alpha1.CopyImageSchema, error) {
	graphRelatedImage := v2alpha1.RelatedImage{
		Name: graphImageName,
		// Supposing that the mirror to disk saved the image with the latest tag
		// If this supposition is false, then we need to implement a mechanism to save
		// the digest of the graph image and use it here
		Image: consts.DockerProtocol + filepath.Join(o.LocalStorageFQDN, graphImageName) + ":latest",
		Type:  v2alpha1.TypeCincinnatiGraph,
	}
	// OCPBUGS-38037: Check the graph image is in the cache before adding it
	graphInCache, err := o.imageExists(ctx, graphRelatedImage.Image)
	if err != nil && !o.Opts.IsDeleteMode() {
		return v2alpha1.CopyImageSchema{}, fmt.Errorf("%s error processing graph image in local cache: %w", collectorPrefix, err)
	}
	// OCPBUGS-43825: The check graphInCache is relevant for DiskToMirror workflow only, not for delete workflow
	// In delete workflow, the graph image might have been mirrored with M2M, and the graph image might have
	// therefore been pushed directly to the destination registry. It will not exist in the cache, and that should be ok.
	// Nevertheless, in DiskToMirror, and as explained in OCPBUGS-38037, the graphInCache check is important
	// because in enclave environment, the Cincinnati API may not have been called, so we rely on the existence of the
	// graph image in the cache as a paliative.
	if !graphInCache && !o.Opts.IsDeleteMode() {
		return v2alpha1.CopyImageSchema{}, fmt.Errorf("%s unable to find graph image in local cache", collectorPrefix)
	}
	// OCPBUGS-26513: In order to get the destination for the graphDataImage
	// into `o.GraphDataImage`, we call `prepareD2MCopyBatch` on an array
	// containing only the graph image. This way we can easily identify the destination
	// of the graph image.
	graphImageSlice := []v2alpha1.RelatedImage{graphRelatedImage}
	graphCopySlice, err := o.prepareD2MCopyBatch(graphImageSlice, "")
	if err != nil {
		return v2alpha1.CopyImageSchema{}, err
	}
	// if there is no error, we are certain that the slice only contains 1 element
	// but double checking...
	if len(graphCopySlice) != 1 {
		return v2alpha1.CopyImageSchema{}, fmt.Errorf(collectorPrefix + "error while calculating the destination reference for the graph image")
	}
	return graphCopySlice[0], nil
}

func (o LocalStorageCollector) prepareM2DCopyBatch(images []v2alpha1.RelatedImage, releaseTag string) ([]v2alpha1.CopyImageSchema, error) {
	result := make([]v2alpha1.CopyImageSchema, 0, len(images))
	for _, img := range images {
		var src string
		var dest string

		imgSpec, err := image.ParseRef(img.Image)
		if err != nil {
			return nil, err
		}
		src = imgSpec.ReferenceWithTransport

		pathComponents := preparePathComponents(imgSpec, img.Type, img.Name)
		tag := prepareTag(imgSpec, img.Type, releaseTag, img.Name)

		dest = consts.DockerProtocol + strings.Join([]string{o.destinationRegistry(), pathComponents + ":" + tag}, "/")

		o.Log.Debug("source %s", src)
		o.Log.Debug("destination %s", dest)
		result = append(result, v2alpha1.CopyImageSchema{
			Origin:      img.Image,
			Source:      src,
			Destination: dest,
			Type:        img.Type,
		})
	}
	return result, nil
}

func (o LocalStorageCollector) prepareD2MCopyBatch(images []v2alpha1.RelatedImage, releaseTag string) ([]v2alpha1.CopyImageSchema, error) {
	result := make([]v2alpha1.CopyImageSchema, 0, len(images))
	for _, img := range images {
		var src string
		var dest string

		imgSpec, err := image.ParseRef(img.Image)
		if err != nil {
			return nil, err
		}

		pathComponents := preparePathComponents(imgSpec, img.Type, img.Name)
		tag := prepareTag(imgSpec, img.Type, releaseTag, img.Name)

		src = consts.DockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathComponents + ":" + tag}, "/")
		dest = strings.Join([]string{o.Opts.Destination, pathComponents + ":" + tag}, "/")

		if src == "" || dest == "" {
			return result, fmt.Errorf("unable to determine src %s or dst %s for %s", src, dest, img.Name)
		}

		o.Log.Debug("source %s", src)
		o.Log.Debug("destination %s", dest)
		result = append(result, v2alpha1.CopyImageSchema{
			Origin:      img.Image,
			Source:      src,
			Destination: dest,
			Type:        img.Type,
		})

	}
	return result, nil
}

func (o LocalStorageCollector) identifyReleases(ctx context.Context) ([]v2alpha1.RelatedImage, []string, error) {
	releaseImageCopies := []v2alpha1.CopyImageSchema{}

	values, err := o.Cincinnati.GetReleaseReferenceImages(ctx)
	if err != nil {
		return []v2alpha1.RelatedImage{}, nil, err
	}
	for _, value := range values {
		hld := strings.Split(value.Source, "/")
		imageIndexDir := strings.Replace(hld[len(hld)-1], ":", "/", -1)
		dir := filepath.Join(o.Opts.Global.WorkingDir, releaseImageDir, imageIndexDir)

		src := consts.DockerProtocol + value.Source
		dest := consts.OciProtocolTrimmed + dir
		r := v2alpha1.CopyImageSchema{
			Source:      src,
			Destination: dest,
		}
		releaseImageCopies = append(releaseImageCopies, r)
	}

	releaseFolders := []string{}
	releaseImages := []v2alpha1.RelatedImage{}
	for _, copy := range releaseImageCopies {
		releasePath := strings.TrimPrefix(copy.Destination, consts.OciProtocol)
		releasePath = strings.TrimPrefix(releasePath, consts.OciProtocolTrimmed)
		releaseHoldPath := strings.Replace(releasePath, releaseImageDir, releaseImageExtractDir, 1)
		releaseFolders = append(releaseFolders, releaseHoldPath)
		releaseImages = append(releaseImages, v2alpha1.RelatedImage{
			Name:  copy.Source,
			Image: copy.Source,
			Type:  v2alpha1.TypeOCPRelease,
		})
	}
	return releaseImages, releaseFolders, nil
}

// assumes this is called during DiskToMirror workflow.
// this method doesn't verify if the graphImage has been generated
// by the collector.
func (o *LocalStorageCollector) GraphImage() (string, error) {
	if o.GraphDataImage == "" {
		sourceGraphDataImage := filepath.Join(o.LocalStorageFQDN, graphImageName) + ":latest"
		graphRelatedImage := []v2alpha1.RelatedImage{
			{
				Name:  "release",
				Image: sourceGraphDataImage,
				Type:  v2alpha1.TypeCincinnatiGraph,
			},
		}
		graphCopyImage, err := o.prepareD2MCopyBatch(graphRelatedImage, "")
		if err != nil {
			return "", fmt.Errorf("[release collector] could not establish the destination for the graph image: %v", err)
		}
		o.GraphDataImage = graphCopyImage[0].Destination
	}
	return o.GraphDataImage, nil
}

// assumes that this is called during DiskToMirror workflow.
// it relies on the previously saved cincinnati-graph-data in order
// to get the list of releases to mirror (saved during mirrorToDisk
// after the call to cincinnati API)
func (o *LocalStorageCollector) ReleaseImage(ctx context.Context) (string, error) {
	if len(o.Releases) == 0 {
		releaseImages, _, err := o.identifyReleases(ctx)
		if err != nil {
			return "", fmt.Errorf("[release collector] could not find release images (from disk cache): %v", err)
		}
		o.Releases = []string{}
		for _, img := range releaseImages {
			o.Releases = append(o.Releases, img.Image)
		}
	}
	if len(o.Releases) > 0 {
		releaseRelatedImage := []v2alpha1.RelatedImage{
			{
				Name:  "release",
				Image: o.Releases[0],
				Type:  v2alpha1.TypeOCPRelease,
			},
		}
		// OCPBUGS-50503
		// This could be a digest also
		imgSpec, err := image.ParseRef(o.Releases[0])
		if err != nil {
			return "", fmt.Errorf("[release collector] could not parse release image %s", o.Releases[0])
		}
		releaseTag := ""
		if imgSpec.IsImageByDigestOnly() {
			releaseTag = imgSpec.Digest
		} else {
			releaseTag = imgSpec.Tag
		}

		releaseCopyImage, err := o.prepareD2MCopyBatch(releaseRelatedImage, releaseTag)
		if err != nil {
			return "", fmt.Errorf("[release collector] could not establish the destination for the release image: %v", err)
		}
		return releaseCopyImage[0].Destination, nil

	} else {
		return "", fmt.Errorf("[release collector] could not establish the destination for the release image")
	}
}

// kubeVirtArchName maps the Go/OCP architecture names used in the
// ImageSetConfiguration to the keys used in the
// 0000_50_installer_coreos-bootimages ConfigMap JSON (architectures.*).
// Only architectures that actually ship a kubevirt container image are listed;
// as of RHCOS 4.15+ those are x86_64 and s390x.
// aarch64 and ppc64le are present in the bootimages JSON but carry no
// kubevirt image entry, so they are intentionally omitted here.
var kubeVirtArchName = map[string]string{
	"amd64": "x86_64",
	"s390x": "s390x",
}

// allKubeVirtArchKeys lists the bootimages JSON architecture keys that carry a
// kubevirt image. Used when the user selects the "multi" sparse-manifest-list
// workflow (platform.platforms) so that every supported arch is mirrored.
var allKubeVirtArchKeys = []string{"x86_64", "s390x"}

// kubeVirtStream is a CoreOS stream document of the bootimages ConfigMap.
type kubeVirtStream struct {
	// name is the stream name in the stream collection (data.streams).
	// It is empty for the legacy single stream (data.stream).
	name   string
	images v2alpha1.InstallerBootableImages
}

// kubeVirtStreamNameRegexp matches the stream names that only contain characters
// allowed in an image tag, so that they can be appended to the destination tag.
var kubeVirtStreamNameRegexp = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// kubeVirtTagRegexp matches valid image tags: the tag grammar of
// github.com/distribution/reference ([\w][\w.-]{0,127}), anchored.
var kubeVirtTagRegexp = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

// kubeVirtImage is a kubevirt image to mirror.
type kubeVirtImage struct {
	v2alpha1.RelatedImage
	// arch is the bootimages architecture key of the image.
	arch string
	// stream is the name of the stream of the image in the stream collection.
	// It is empty for the images of the legacy single stream.
	stream string
	// legacyName is the name that the image has in archives created before stream
	// collections were supported. It is only set for the images of a stream collection.
	legacyName string
}

// getKubeVirtImages - CLID-179 : include coreos-bootable container images
// for all requested architectures. Returns one RelatedImage per architecture
// whose kubevirt digest-ref is non-empty in the bootimages ConfigMap, for every
// CoreOS stream: the images of the legacy single stream (data.stream) keep their
// legacy names, even when a stream of the stream collection (data.streams) has the
// same image, and the images of the stream collection get the name of their stream.
func (o LocalStorageCollector) getKubeVirtImages(releaseArtifactsDir string) ([]v2alpha1.RelatedImage, error) {
	kvImages, err := o.collectKubeVirtImages(releaseArtifactsDir)
	if err != nil {
		return nil, err
	}
	images := make([]v2alpha1.RelatedImage, 0, len(kvImages))
	for _, img := range kvImages {
		images = append(images, img.RelatedImage)
	}
	return images, nil
}

// collectKubeVirtImages returns the kubevirt images described by getKubeVirtImages,
// together with their architectures, stream names and legacy names.
func (o LocalStorageCollector) collectKubeVirtImages(releaseArtifactsDir string) ([]kubeVirtImage, error) {
	// parse the main yaml file
	biFile := strings.Join([]string{releaseArtifactsDir, releaseBootableImagesFullPath}, "/")
	icm, err := parser.ParseYamlFile[v2alpha1.InstallerConfigMap](biFile)
	if err != nil {
		// this should not break the release process
		// we just report the error and continue
		return nil, fmt.Errorf("marshalling kubevirt yaml file %w", err)
	}

	// now parse the json section(s)
	streams, err := parseKubeVirtStreams(icm)
	if err != nil {
		return nil, err
	}
	o.Log.Debug("kubevirt: %d CoreOS stream document(s) found in the bootimages ConfigMap", len(streams))

	// Build the set of bootimages JSON architecture keys to collect.
	// When the user chose platform.platforms (the "multi" payload path) we
	// collect every architecture present in the ConfigMap. When the user
	// chose the older platform.architectures field we map each entry to its
	// JSON key. Defaults to x86_64 for backward compatibility.
	archKeys := o.requestedKubeVirtArchKeys()

	var images []kubeVirtImage
	for _, stream := range streams {
		images = append(images, o.kubeVirtStreamImages(stream, archKeys)...)
	}
	images, err = dedupeKubeVirtImages(images)
	if err != nil {
		return nil, err
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("could not find kubevirt image in this release")
	}
	return images, nil
}

// dedupeKubeVirtImages removes the images already present with the same digest-ref and
// the same name, so the same destination. The images with the same digest-ref and
// different names are all kept, as each name is a destination that must exist: for
// example the legacy name of an image of the legacy single stream and the name of its
// stream in the stream collection. Two different images with the same name, that would
// be copied to the same destination, are an error.
func dedupeKubeVirtImages(images []kubeVirtImage) ([]kubeVirtImage, error) {
	digestRefByName := make(map[string]string, len(images))
	result := make([]kubeVirtImage, 0, len(images))
	for _, img := range images {
		if digestRef, ok := digestRefByName[img.Name]; ok {
			if digestRef == img.Image {
				continue
			}
			return nil, fmt.Errorf("kubevirt: images %s and %s have the same name %q", digestRef, img.Image, img.Name)
		}
		digestRefByName[img.Name] = img.Image
		result = append(result, img)
	}
	return result, nil
}

// kubeVirtStreamImages returns the kubevirt images of a CoreOS stream for the given
// bootimages architecture keys.
func (o LocalStorageCollector) kubeVirtStreamImages(stream kubeVirtStream, archKeys []string) []kubeVirtImage {
	// archImages maps a bootimages JSON key to its ArchImages struct.
	archImages := map[string]v2alpha1.ArchImages{
		"x86_64":  stream.images.Architectures.X86_64,
		"aarch64": stream.images.Architectures.Aarch64,
		"ppc64le": stream.images.Architectures.Ppc64le,
		"s390x":   stream.images.Architectures.S390x,
	}

	var images []kubeVirtImage
	for _, archKey := range archKeys {
		ai, ok := archImages[archKey]
		if !ok {
			o.Log.Warn("kubevirt: unknown architecture key %q — skipping", archKey)
			continue
		}
		digestRef := ai.Images.Kubevirt.DigestRef
		if digestRef == "" {
			if stream.name != "" {
				o.Log.Debug("kubevirt: no digest-ref for architecture %q in stream %q — skipping", archKey, stream.name)
			} else {
				o.Log.Debug("kubevirt: no digest-ref for architecture %q in this release — skipping", archKey)
			}
			continue
		}
		if stream.name != "" {
			o.Log.Info("kubeVirtContainer set to true [ including : %v (arch: %v, stream: %v) ]", digestRef, archKey, stream.name)
		} else {
			o.Log.Info("kubeVirtContainer set to true [ including : %v (arch: %v) ]", digestRef, archKey)
		}

		img := kubeVirtImage{arch: archKey, stream: stream.name, RelatedImage: v2alpha1.RelatedImage{
			Image: digestRef,
			Name:  kubeVirtImageName(archKey, len(archKeys) > 1, stream.name),
			Type:  v2alpha1.TypeOCPReleaseContent,
		}}
		if stream.name != "" {
			img.legacyName = kubeVirtImageName(archKey, len(archKeys) > 1, "")
		}
		images = append(images, img)
	}
	return images
}

// checkKubeVirtStreamTags returns an error when the destination tag of an image of a
// stream collection, made of the release tag, the image name and the stream name, is
// not a valid image tag. The destination tags of the legacy single stream are unchanged.
func checkKubeVirtStreamTags(images []kubeVirtImage, releaseTag string) error {
	for _, img := range images {
		if img.stream == "" {
			continue
		}
		if tag := prepareTag(image.ImageSpec{}, img.Type, releaseTag, img.Name); !kubeVirtTagRegexp.MatchString(tag) {
			return fmt.Errorf("kubevirt: the destination tag %q of the image of stream %q (arch: %v) is not a valid image tag: "+
				"tags have at most 128 letters, digits, '_', '.' and '-'", tag, img.stream, img.arch)
		}
	}
	return nil
}

// parseKubeVirtStreams returns the CoreOS stream documents of the bootimages ConfigMap:
// every stream of the stream collection (data.streams), sorted by name so that the
// result is deterministic, followed by the legacy single stream (data.stream), which
// has no name. The legacy single stream is required when the stream collection is
// absent or empty, and optional otherwise.
func parseKubeVirtStreams(icm v2alpha1.InstallerConfigMap) ([]kubeVirtStream, error) {
	var streams []kubeVirtStream
	if strings.TrimSpace(icm.Data.Streams) != "" {
		collection, err := parser.ParseJsonReader[map[string]v2alpha1.InstallerBootableImages](strings.NewReader(icm.Data.Streams))
		if err != nil {
			return nil, fmt.Errorf("parsing json from kubevirt configmap streams data: %w", err)
		}
		for _, name := range slices.Sorted(maps.Keys(collection)) {
			if !kubeVirtStreamNameRegexp.MatchString(name) {
				return nil, errors.New("invalid stream name in kubevirt configmap streams data: names may only contain letters, digits, '_', '.' and '-'")
			}
			streams = append(streams, kubeVirtStream{name: name, images: collection[name]})
		}
	}

	if len(streams) > 0 && strings.TrimSpace(icm.Data.Stream) == "" {
		return streams, nil
	}
	ibi, err := parser.ParseJsonReader[v2alpha1.InstallerBootableImages](strings.NewReader(icm.Data.Stream))
	if err != nil {
		return nil, fmt.Errorf("parsing json from kubevirt configmap data %w", err)
	}
	return append(streams, kubeVirtStream{images: ibi}), nil
}

// kubeVirtImageName returns the name used to build the destination tag of a kubevirt image.
// For backward compatibility, the image of the legacy single stream keeps the legacy name
// "kube-virt-container" in single-arch mirrors. The architecture is appended when mirroring
// multiple architectures, and the stream name is appended for every stream of a stream
// collection, so that each image gets its own destination.
func kubeVirtImageName(archKey string, multiArch bool, streamName string) string {
	imageName := "kube-virt-container"
	if multiArch {
		imageName += "-" + archKey
	}
	if streamName != "" {
		imageName += "-" + streamName
	}
	return imageName
}

// requestedKubeVirtArchKeys returns the bootimages JSON architecture keys
// that correspond to the architectures requested in the ImageSetConfiguration.
// When platform.platforms is set (the "multi" sparse-manifest-list workflow)
// all known architecture keys are returned. When platform.architectures is
// set each entry is mapped from its Go name (e.g. "amd64") to its JSON key
// (e.g. "x86_64"). Defaults to ["x86_64"] for backward compatibility.
func (o LocalStorageCollector) requestedKubeVirtArchKeys() []string {
	cfg := o.Config.Mirror.Platform

	// New path: platform.platforms was set — the release payload is the "multi"
	// manifest list and we want kubevirt images for every architecture.
	if len(cfg.Platforms) > 0 {
		var keys []string
		for _, pf := range cfg.Platforms {
			if jsonKey, ok := kubeVirtArchName[pf.Architecture]; ok {
				keys = append(keys, jsonKey)
			} else {
				o.Log.Warn("kubevirt: no bootimages key for platform architecture %q — skipping", pf.Architecture)
			}
		}
		if len(keys) > 0 {
			return keys
		}
		// Fall through: none of the requested architectures had a known mapping;
		// return all keys so we still attempt to collect something.
		return allKubeVirtArchKeys
	}

	// Deprecated path: platform.architectures was set.
	//nolint:staticcheck // SA1019: Architectures is deprecated but we maintain backward compatibility
	if len(cfg.Architectures) > 0 {
		var keys []string
		for _, arch := range cfg.Architectures {
			if arch == "multi" {
				return allKubeVirtArchKeys
			}
			if jsonKey, ok := kubeVirtArchName[arch]; ok {
				keys = append(keys, jsonKey)
			} else {
				o.Log.Warn("kubevirt: no bootimages key for architecture %q — skipping", arch)
			}
		}
		if len(keys) > 0 {
			return keys
		}
	}

	// Default: backward-compatible single-arch (x86_64).
	return []string{"x86_64"}
}

func (o LocalStorageCollector) handleGraphImage(ctx context.Context) (v2alpha1.CopyImageSchema, error) {
	o.Log.Debug(collectorPrefix + "processing graph data image")
	if updateURLOverride := os.Getenv("UPDATE_URL_OVERRIDE"); len(updateURLOverride) != 0 {
		// OCPBUGS-38037: this indicates that the official cincinnati API is not reacheable
		// and that graph image cannot be rebuilt on top the complete graph in tar.gz format

		graphImgRef := consts.DockerProtocol + filepath.Join(o.destinationRegistry(), graphImageName) + ":latest"

		// 1. check if graph image is already in cache
		cachedImageRef := consts.DockerProtocol + filepath.Join(o.LocalStorageFQDN, graphImageName) + ":latest"
		alreadyInCache, err := o.imageExists(ctx, cachedImageRef)
		if err != nil {
			o.Log.Warn("graph image not found in cache: %v", err)
		}
		if alreadyInCache { // use graph image from cache
			graphCopy := v2alpha1.CopyImageSchema{
				Source:      cachedImageRef,
				Destination: graphImgRef,
				Origin:      cachedImageRef,
				Type:        v2alpha1.TypeCincinnatiGraph,
			}
			return graphCopy, nil
		}
		// 2. check if graph image exist in oci format in working-dir
		workingDirGraphImageRef, err := o.graphImageInWorkingDir(ctx)
		if err != nil || workingDirGraphImageRef == "" {
			return v2alpha1.CopyImageSchema{}, fmt.Errorf("no graph image in cache (nor working-dir): %v", err)
		} else {
			//    => use OCI image in workingDir
			graphCopy := v2alpha1.CopyImageSchema{
				Source:      workingDirGraphImageRef,
				Destination: graphImgRef,
				Origin:      workingDirGraphImageRef,
				Type:        v2alpha1.TypeCincinnatiGraph,
			}
			return graphCopy, nil
		}

	} else {
		// TODO: pass o.getPlatformFilters() to CreateGraphImage so the graph image is built
		// only for the requested architectures. Tracked as a separate PR.
		graphImgRef, err := o.CreateGraphImage(ctx, graphURL)
		if err != nil {
			return v2alpha1.CopyImageSchema{}, err
		}
		o.Log.Debug(collectorPrefix + "graph image created and pushed to cache.")
		// still add the graph image to the `allImages` so that we later can add it in the tar.gz archive
		// or copied to the destination registry (case of mirror to mirror)
		graphCopy := v2alpha1.CopyImageSchema{
			Source:      graphImgRef,
			Destination: graphImgRef,
			Origin:      graphImgRef,
			Type:        v2alpha1.TypeCincinnatiGraph,
		}
		return graphCopy, nil
	}
}

// compareByOriginSourceDest sorts CopyImageSchema entries deterministically.
// The same digest can appear several times in image-references for different
// components, so all three fields are compared to produce a stable order.
func compareByOriginSourceDest(a, b v2alpha1.CopyImageSchema) int {
	if cmp := strings.Compare(a.Origin, b.Origin); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(a.Source, b.Source); cmp != 0 {
		return cmp
	}
	return strings.Compare(a.Destination, b.Destination)
}

func preparePathComponents(imgSpec image.ImageSpec, imgType v2alpha1.ImageType, imgName string) string {
	pathComponents := ""
	switch {
	case imgType == v2alpha1.TypeOCPRelease:
		pathComponents = releaseImagePathComponents
	case imgType == v2alpha1.TypeCincinnatiGraph:
		pathComponents = imgSpec.PathComponent
	case imgType == v2alpha1.TypeOCPReleaseContent && imgName != "":
		pathComponents = releaseComponentPathComponents
	case imgSpec.IsImageByDigestOnly():
		pathComponents = imgSpec.PathComponent
	}

	return pathComponents
}

func prepareTag(imgSpec image.ImageSpec, imgType v2alpha1.ImageType, releaseTag, imgName string) string {
	tag := imgSpec.Tag

	switch {
	case imgType == v2alpha1.TypeOCPRelease || imgType == v2alpha1.TypeCincinnatiGraph:
		// OCPBUGS-44033 mirroring release with no release tag
		// i.e by digest registry.ci.openshift.org/ocp/release@sha256:0fb444ec9bb1b01f06dd387519f0fe5b4168e2d09a015697a26534fc1565c5e7
		if len(imgSpec.Tag) == 0 {
			tag = releaseTag
		} else {
			tag = imgSpec.Tag
		}
	case imgType == v2alpha1.TypeOCPReleaseContent && imgName != "":
		tag = releaseTag + "-" + imgName
	case imgSpec.IsImageByDigestOnly():
		tag = imgSpec.AlgorithmAndDigest()
	}

	return tag
}

func logCollectionError(log clog.PluggableLoggerInterface, spinner *mpb.Bar, isTerminal bool, releaseImage string, err error) {
	spinner.Abort(true)
	spinner.Wait()
	if !isTerminal {
		log.Error("Failed to collect release image %s: %v", releaseImage, err)
	}
}
