package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	digest "github.com/opencontainers/go-digest"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	"github.com/openshift/oc-mirror/v2/internal/pkg/imagebuilder"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"gopkg.in/yaml.v2"
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
	var allImages []v2alpha1.CopyImageSchema
	var imageIndexDir string
	if o.Opts.IsMirrorToDisk() || o.Opts.IsMirrorToMirror() {
		releases, err := o.Cincinnati.GetReleaseReferenceImages(ctx)
		if err != nil {
			return allImages, err
		}

		// all errors will be probogated to the caller
		// no redundant logging to console
		for _, value := range releases {
			hld := strings.Split(value.Source, "/")
			releaseRepoAndTag := hld[len(hld)-1]
			imageIndexDir = strings.Replace(releaseRepoAndTag, ":", "/", -1)
			releaseTag := releaseRepoAndTag[strings.Index(releaseRepoAndTag, ":")+1:]
			cacheDir := filepath.Join(o.Opts.Global.WorkingDir, releaseImageExtractDir, imageIndexDir)
			dir := filepath.Join(o.Opts.Global.WorkingDir, releaseImageDir, imageIndexDir)

			src := dockerProtocol + value.Source
			dest := ociProtocolTrimmed + dir

			if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
				o.Log.Debug(collectorPrefix+"copying  release image %s ", value.Source)
				err := os.MkdirAll(dir, 0755)
				if err != nil {
					//o.Log.Error(errMsg, err.Error())
					return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
				}

				optsCopy := o.Opts
				optsCopy.Stdout = io.Discard
				optsCopy.RemoveSignatures = true

				err = o.Mirror.Run(ctx, src, dest, "copy", &optsCopy)
				if err != nil {
					//o.Log.Error(errMsg, err.Error())
					return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
				}
				o.Log.Debug(collectorPrefix+"copied release index image %s ", value.Source)
			} else {
				o.Log.Debug(collectorPrefix+"release-images index directory alredy exists %s", dir)
			}

			oci, err := o.Manifest.GetImageIndex(dir)
			if err != nil {
				//o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
			}

			//read the link to the manifest
			if len(oci.Manifests) == 0 {
				//o.Log.Error(errMsg, "image index not found ")
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, "image index not found ")
			}
			validDigest, err := digest.Parse(oci.Manifests[0].Digest)
			if err != nil {
				//o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(collectorPrefix+"invalid digest for image index %s: %s", oci.Manifests[0].Digest, err.Error())
			}

			manifest := validDigest.Encoded()
			o.Log.Debug(collectorPrefix+"image manifest digest %s", manifest)

			manifestDir := filepath.Join(dir, blobsDir, manifest)
			mfst, err := o.Manifest.GetImageManifest(manifestDir)
			if err != nil {
				//o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
			}
			o.Log.Debug(collectorPrefix+"config digest %s ", oci.Config.Digest)

			fromDir := strings.Join([]string{dir, blobsDir}, "/")
			err = o.Manifest.ExtractLayersOCI(fromDir, cacheDir, releaseManifests, mfst)
			if err != nil {
				//o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
			}
			o.Log.Debug("extracted layer %s ", cacheDir)

			// overkill but its used for consistency
			releaseDir := strings.Join([]string{cacheDir, releaseImageExtractFullPath}, "/")
			allRelatedImages, err := o.Manifest.GetReleaseSchema(releaseDir)
			if err != nil {
				//o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
			}

			if o.Config.Mirror.Platform.KubeVirtContainer {
				ki, err := o.getKubeVirtImage(cacheDir)
				if err != nil {
					// log to console as warning
					o.Log.Warn("%v", err)
				} else {
					allRelatedImages = append(allRelatedImages, ki)
				}
			}

			//add the release image itself
			allRelatedImages = append(allRelatedImages, v2alpha1.RelatedImage{Image: value.Source, Name: value.Source, Type: v2alpha1.TypeOCPRelease})
			tmpAllImages, err := o.prepareM2DCopyBatch(allRelatedImages, releaseTag)
			if err != nil {
				return []v2alpha1.CopyImageSchema{}, err
			}
			allImages = append(allImages, tmpAllImages...)
		}

		if o.Config.Mirror.Platform.Graph {
			graphImage, err := o.handleGraphImage(ctx)
			if err != nil {
				o.Log.Warn("error during graph image processing - SKIPPING: %v", err)
			} else if graphImage.Source != "" {
				allImages = append(allImages, graphImage)
			}

		}

	} else if o.Opts.IsDiskToMirror() {
		releaseImages, releaseFolders, err := o.identifyReleases(ctx)
		if err != nil {
			//o.Log.Error(errMsg, err.Error())
			return allImages, err
		}

		o.Releases = []string{}
		for _, img := range releaseImages {
			o.Releases = append(o.Releases, img.Image)
		}

		for _, releaseImg := range releaseImages {
			releaseRef, err := image.ParseRef(releaseImg.Image)
			if err != nil {
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
			}
			if releaseRef.Tag == "" && len(releaseRef.Digest) == 0 {
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, "release image "+releaseImg.Image+" doesn't have a tag or digest")
			}
			tag := releaseRef.Tag
			if releaseRef.Tag == "" && len(releaseRef.Digest) > 0 {
				tag = releaseRef.Digest
			}
			monoReleaseSlice, err := o.prepareD2MCopyBatch([]v2alpha1.RelatedImage{releaseImg}, tag)
			if err != nil {
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
			}
			allImages = append(allImages, monoReleaseSlice...)
		}

		for _, releaseDir := range releaseFolders {

			releaseTag := filepath.Base(releaseDir)

			// get all release images from manifest (json)
			imageReferencesFile := filepath.Join(releaseDir, releaseManifests, imageReferences)
			releaseRelatedImages, err := o.Manifest.GetReleaseSchema(imageReferencesFile)
			if err != nil {
				//o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
			}

			if o.Config.Mirror.Platform.KubeVirtContainer {
				cacheDir := filepath.Join(releaseDir)
				ki, err := o.getKubeVirtImage(cacheDir)
				if err != nil {
					// log to console as warning
					o.Log.Warn("%v", err)
				} else {
					releaseRelatedImages = append(releaseRelatedImages, ki)
				}
			}

			releaseCopyImages, err := o.prepareD2MCopyBatch(releaseRelatedImages, releaseTag)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, err
			}
			allImages = append(allImages, releaseCopyImages...)
		}

		if o.Config.Mirror.Platform.Graph {
			o.Log.Debug("adding graph data image")
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
			// OCPBUGS-43825: The check graphInCache is relevant for DiskToMirror workflow only, not for delete workflow
			// In delete workflow, the graph image might have been mirrored with M2M, and the graph image might have
			// therefore been pushed directly to the destination registry. It will not exist in the cache, and that should be ok.
			// Nevertheless, in DiskToMirror, and as explained in OCPBUGS-38037, the graphInCache check is important
			// because in enclave environment, the Cincinnati API may not have been called, so we rely on the existance of the
			// graph image in the cache as a paliative.
			shouldProceed := graphInCache || o.Opts.IsDeleteMode()
			if err != nil && !o.Opts.IsDeleteMode() {
				o.Log.Warn("unable to find graph image in local cache: %v. SKIPPING", err)
			}
			if shouldProceed {
				// OCPBUGS-26513: In order to get the destination for the graphDataImage
				// into `o.GraphDataImage`, we call `prepareD2MCopyBatch` on an array
				// containing only the graph image. This way we can easily identify the destination
				// of the graph image.
				graphImageSlice := []v2alpha1.RelatedImage{graphRelatedImage}
				graphCopySlice, err := o.prepareD2MCopyBatch(graphImageSlice, "")
				if err != nil {
					o.Log.Error(errMsg, err.Error())
					return []v2alpha1.CopyImageSchema{}, err
				}
				// if there is no error, we are certain that the slice only contains 1 element
				// but double checking...
				if len(graphCopySlice) != 1 {
					//o.Log.Error(errMsg, "error while calculating the destination reference for the graph image")
					return []v2alpha1.CopyImageSchema{}, fmt.Errorf(collectorPrefix + "error while calculating the destination reference for the graph image")
				}
				o.GraphDataImage = graphCopySlice[0].Destination
				allImages = append(allImages, graphCopySlice...)
			}
		}
	}

	//OCPBUGS-43275: deduplicating
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

func (o LocalStorageCollector) prepareM2DCopyBatch(images []v2alpha1.RelatedImage, releaseTag string) ([]v2alpha1.CopyImageSchema, error) {
	var result []v2alpha1.CopyImageSchema
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
	var result []v2alpha1.CopyImageSchema
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
			return "", fmt.Errorf("[release collector] could not establish the destination for the release image: %v", err)
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
		releaseTag := o.Releases[0][:strings.LastIndex(o.Releases[0], ":")]

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
	var ibi v2alpha1.InstallerBootableImages
	var icm v2alpha1.InstallerConfigMap

	// parse the main yaml file
	biFile := strings.Join([]string{releaseArtifactsDir, releaseBootableImagesFullPath}, "/")
	file, err := os.ReadFile(biFile)
	if err != nil {
		return v2alpha1.RelatedImage{}, fmt.Errorf("reading kubevirt yaml file %v", err)
	}

	errs := yaml.Unmarshal(file, &icm)
	if errs != nil {
		// this should not break the release process
		// we just report the error and continue
		return v2alpha1.RelatedImage{}, fmt.Errorf("marshalling kubevirt yaml file %v", errs)
	}

	o.Log.Trace(fmt.Sprintf("data %v", icm.Data.Stream))
	// now parse the json section
	errs = json.Unmarshal([]byte(icm.Data.Stream), &ibi)
	if errs != nil {
		// this should not break the release process
		// we just report the error and continue
		return v2alpha1.RelatedImage{}, fmt.Errorf("parsing json from kubevirt configmap data %v", errs)
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
