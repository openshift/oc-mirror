package release

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vbauerster/mpb/v8"

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
			o.destReg = strings.TrimPrefix(o.Opts.Destination, dockerProtocol)
		} else {
			o.destReg = o.LocalStorageFQDN
		}
	}
	return o.destReg
}

func (o *LocalStorageCollector) ReleaseImageCollector(ctx context.Context) ([]v2alpha1.CopyImageSchema, error) {
	// we just care for 1 platform release, in order to read release images
	o.Opts.MultiArch = "system"
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
		return []v2alpha1.CopyImageSchema{}, err
	}

	// OCPBUGS-43275: deduplicating
	slices.SortFunc(allImages, func(a, b v2alpha1.CopyImageSchema) int {
		cmp := strings.Compare(a.Origin, b.Origin)
		if cmp == 0 {
			cmp = strings.Compare(a.Source, b.Source)
		}
		if cmp == 0 { // this comparison is important because the same digest can be used
			// several times in image-references for different components
			cmp = strings.Compare(a.Destination, b.Destination)
		}
		return cmp
	})
	allImages = slices.Compact(allImages)

	return allImages, nil
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
		allRelatedImages = append(allRelatedImages, v2alpha1.RelatedImage{Image: value.Source, Name: value.Source, Type: v2alpha1.TypeOCPRelease})
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
		ki, err := o.getKubeVirtImage(cacheDir)
		if err != nil {
			return []v2alpha1.RelatedImage{}, err
		}
		allRelatedImages = append(allRelatedImages, ki)
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create oci dir: %w", err)
	}

	optsCopy := o.Opts
	optsCopy.Stdout = io.Discard

	src := dockerProtocol + release.Source
	dest := ociProtocolTrimmed + dir

	optsCopy.RemoveSignatures = true

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
		releaseCopyImages, err := o.prepareReleaseBatchFromDir(releaseDir)
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

func (o *LocalStorageCollector) prepareReleaseBatchFromDir(releaseDir string) ([]v2alpha1.CopyImageSchema, error) {
	releaseTag := filepath.Base(releaseDir)

	// get all release images from manifest (json)
	imageReferencesFile := filepath.Join(releaseDir, releaseManifests, imageReferences)
	releaseRelatedImages, err := o.Manifest.GetReleaseSchema(imageReferencesFile)
	if err != nil {
		return []v2alpha1.CopyImageSchema{}, err
	}

	if o.Config.Mirror.Platform.KubeVirtContainer {
		ki, err := o.getKubeVirtImage(releaseDir)
		if err != nil {
			return []v2alpha1.CopyImageSchema{}, err
		}
		releaseRelatedImages = append(releaseRelatedImages, ki)
	}

	return o.prepareD2MCopyBatch(releaseRelatedImages, releaseTag)
}

func (o *LocalStorageCollector) prepareGraphImage(ctx context.Context) (v2alpha1.CopyImageSchema, error) {
	graphRelatedImage := v2alpha1.RelatedImage{
		Name: graphImageName,
		// Supposing that the mirror to disk saved the image with the latest tag
		// If this supposition is false, then we need to implement a mechanism to save
		// the digest of the graph image and use it here
		Image: dockerProtocol + filepath.Join(o.LocalStorageFQDN, graphImageName) + ":latest",
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

		dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), pathComponents + ":" + tag}, "/")

		o.Log.Debug("source %s", src)
		o.Log.Debug("destination %s", dest)
		result = append(result, v2alpha1.CopyImageSchema{Origin: img.Image, Source: src, Destination: dest, Type: img.Type})
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

		src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathComponents + ":" + tag}, "/")
		dest = strings.Join([]string{o.Opts.Destination, pathComponents + ":" + tag}, "/")

		if src == "" || dest == "" {
			return result, fmt.Errorf("unable to determine src %s or dst %s for %s", src, dest, img.Name)
		}

		o.Log.Debug("source %s", src)
		o.Log.Debug("destination %s", dest)
		result = append(result, v2alpha1.CopyImageSchema{Origin: img.Image, Source: src, Destination: dest, Type: img.Type})

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

		src := dockerProtocol + value.Source
		dest := ociProtocolTrimmed + dir
		r := v2alpha1.CopyImageSchema{
			Source:      src,
			Destination: dest,
		}
		releaseImageCopies = append(releaseImageCopies, r)
	}

	releaseFolders := []string{}
	releaseImages := []v2alpha1.RelatedImage{}
	for _, copy := range releaseImageCopies {
		releasePath := strings.TrimPrefix(copy.Destination, ociProtocol)
		releasePath = strings.TrimPrefix(releasePath, ociProtocolTrimmed)
		releaseHoldPath := strings.Replace(releasePath, releaseImageDir, releaseImageExtractDir, 1)
		releaseFolders = append(releaseFolders, releaseHoldPath)
		releaseImages = append(releaseImages, v2alpha1.RelatedImage{Name: copy.Source, Image: copy.Source, Type: v2alpha1.TypeOCPRelease})
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

// getKubeVirtImage - CLID-179 : include coreos-bootable container image
// if set it will be across the board for all releases
func (o LocalStorageCollector) getKubeVirtImage(releaseArtifactsDir string) (v2alpha1.RelatedImage, error) {
	// parse the main yaml file
	biFile := strings.Join([]string{releaseArtifactsDir, releaseBootableImagesFullPath}, "/")
	icm, err := parser.ParseYamlFile[v2alpha1.InstallerConfigMap](biFile)
	if err != nil {
		// this should not break the release process
		// we just report the error and continue
		return v2alpha1.RelatedImage{}, fmt.Errorf("marshalling kubevirt yaml file %w", err)
	}

	o.Log.Trace(fmt.Sprintf("data %v", icm.Data.Stream))
	// now parse the json section
	ibi, err := parser.ParseJsonReader[v2alpha1.InstallerBootableImages](strings.NewReader(icm.Data.Stream))
	if err != nil {
		// this should not break the release process
		// we just report the error and continue
		return v2alpha1.RelatedImage{}, fmt.Errorf("parsing json from kubevirt configmap data %w", err)
	}

	image := ibi.Architectures.X86_64.Images.Kubevirt.DigestRef
	if image == "" {
		return v2alpha1.RelatedImage{}, fmt.Errorf("could not find kubevirt image in this release")
	}
	o.Log.Info(fmt.Sprintf("kubeVirtContainer set to true [ including : %v ]", image))
	kubeVirtImage := v2alpha1.RelatedImage{
		Image: image,
		Name:  "kube-virt-container",
		Type:  v2alpha1.TypeOCPReleaseContent,
	}
	return kubeVirtImage, nil
}

func (o LocalStorageCollector) handleGraphImage(ctx context.Context) (v2alpha1.CopyImageSchema, error) {
	o.Log.Debug(collectorPrefix + "processing graph data image")
	if updateURLOverride := os.Getenv("UPDATE_URL_OVERRIDE"); len(updateURLOverride) != 0 {
		// OCPBUGS-38037: this indicates that the official cincinnati API is not reacheable
		// and that graph image cannot be rebuilt on top the complete graph in tar.gz format

		graphImgRef := dockerProtocol + filepath.Join(o.destinationRegistry(), graphImageName) + ":latest"

		// 1. check if graph image is already in cache
		cachedImageRef := dockerProtocol + filepath.Join(o.LocalStorageFQDN, graphImageName) + ":latest"
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
		tag = fmt.Sprintf("%s-%s", imgSpec.Algorithm, imgSpec.Digest)
		if len(tag) > 128 {
			tag = tag[:127]
		}
	}

	return tag
}

func logCollectionError(log clog.PluggableLoggerInterface, spinner *mpb.Bar, isTerminal bool, releaseImage string, err error) {
	spinner.Abort(true)
	spinner.Wait()
	if !isTerminal {
		log.Error("Failed to collect release image %s: %w", releaseImage, err)
	}
}
