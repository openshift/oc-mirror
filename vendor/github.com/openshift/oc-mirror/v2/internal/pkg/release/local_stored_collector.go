package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
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
		releases := o.Cincinnati.GetReleaseReferenceImages(ctx)

		for _, value := range releases {
			hld := strings.Split(value.Source, "/")
			imageIndexDir = strings.Replace(hld[len(hld)-1], ":", "/", -1)
			cacheDir := filepath.Join(o.Opts.Global.WorkingDir, releaseImageExtractDir, imageIndexDir)
			dir := filepath.Join(o.Opts.Global.WorkingDir, releaseImageDir, imageIndexDir)

			src := dockerProtocol + value.Source
			dest := ociProtocolTrimmed + dir

			if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
				o.Log.Debug(collectorPrefix+"copying  release image %s ", value.Source)
				err := os.MkdirAll(dir, 0755)
				if err != nil {
					o.Log.Error(errMsg, err.Error())
					return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
				}

				optsCopy := o.Opts
				optsCopy.Stdout = io.Discard

				err = o.Mirror.Run(ctx, src, dest, "copy", &optsCopy)
				if err != nil {
					o.Log.Error(errMsg, err.Error())
					return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
				}
				o.Log.Debug(collectorPrefix+"copied release index image %s ", value.Source)
			} else {
				o.Log.Debug(collectorPrefix+"release-images index directory alredy exists %s", dir)
			}

			oci, err := o.Manifest.GetImageIndex(dir)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
			}

			//read the link to the manifest
			if len(oci.Manifests) == 0 {
				o.Log.Error(errMsg, "image index not found ")
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, "image index not found ")
			}
			validDigest, err := digest.Parse(oci.Manifests[0].Digest)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(collectorPrefix+"invalid digest for image index %s: %s", oci.Manifests[0].Digest, err.Error())
			}

			manifest := validDigest.Encoded()
			o.Log.Debug(collectorPrefix+"image manifest digest %s", manifest)

			manifestDir := filepath.Join(dir, blobsDir, manifest)
			mfst, err := o.Manifest.GetImageManifest(manifestDir)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
			}
			o.Log.Debug(collectorPrefix+"config digest %s ", oci.Config.Digest)

			fromDir := strings.Join([]string{dir, blobsDir}, "/")
			err = o.Manifest.ExtractLayersOCI(fromDir, cacheDir, releaseManifests, mfst)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
			}
			o.Log.Debug("extracted layer %s ", cacheDir)

			// overkill but its used for consistency
			releaseDir := strings.Join([]string{cacheDir, releaseImageExtractFullPath}, "/")
			allRelatedImages, err := o.Manifest.GetReleaseSchema(releaseDir)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
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
			tmpAllImages, err := o.prepareM2DCopyBatch(allRelatedImages)
			if err != nil {
				return []v2alpha1.CopyImageSchema{}, err
			}
			allImages = append(allImages, tmpAllImages...)
		}

		if o.Config.Mirror.Platform.Graph {
			o.Log.Debug(collectorPrefix + "creating graph data image")
			finalGraphURL := graphURL
			if updateURLOverride := os.Getenv("UPDATE_URL_OVERRIDE"); len(updateURLOverride) != 0 {
				url, err := graphURLFromUpdateURL(updateURLOverride)
				if err != nil {
					o.Log.Error(errMsg, "graph image build: unable to construct graph URL from UPDATE_URL_OVERRIDE: "+err.Error())
					return []v2alpha1.CopyImageSchema{}, err
				}
				finalGraphURL = url
			}
			graphImgRef, err := o.CreateGraphImage(ctx, finalGraphURL)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, err
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
			allImages = append(allImages, graphCopy)
		}

	} else if o.Opts.IsDiskToMirror() {
		releaseImages, releaseFolders, err := o.identifyReleases(ctx)
		if err != nil {
			o.Log.Error(errMsg, err.Error())
			return allImages, err
		}

		o.Releases = []string{}
		for _, img := range releaseImages {
			o.Releases = append(o.Releases, img.Image)
		}

		allRelatedImages := []v2alpha1.RelatedImage{}

		// add the releaseImages so that they are added to the list of images to copy
		allRelatedImages = append(allRelatedImages, releaseImages...)

		for _, releaseDir := range releaseFolders {
			// get all release images from manifest (json)
			imageReferencesFile := filepath.Join(releaseDir, releaseManifests, imageReferences)
			releaseRelatedImages, err := o.Manifest.GetReleaseSchema(imageReferencesFile)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(errMsg, err.Error())
			}

			if o.Config.Mirror.Platform.KubeVirtContainer {
				cacheDir := filepath.Join(releaseDir)
				ki, err := o.getKubeVirtImage(cacheDir)
				if err != nil {
					// log to console as warning
					o.Log.Warn("%v", err)
				} else {
					allRelatedImages = append(allRelatedImages, ki)
				}
			}

			allRelatedImages = append(allRelatedImages, releaseRelatedImages...)
		}

		if o.Config.Mirror.Platform.Graph {
			o.Log.Debug("adding graph data image")
			graphRelatedImage := v2alpha1.RelatedImage{
				Name: graphImageName,
				// Supposing that the mirror to disk saved the image with the latest tag
				// If this supposition is false, then we need to implement a mechanism to save
				// the digest of the graph image and use it here
				Image: filepath.Join(o.LocalStorageFQDN, graphImageName) + ":latest",
				Type:  v2alpha1.TypeCincinnatiGraph,
			}
			// OCPBUGS-26513: In order to get the destination for the graphDataImage
			// into `o.GraphDataImage`, we call `prepareD2MCopyBatch` on an array
			// containing only the graph image. This way we can easily identify the destination
			// of the graph image.
			graphImageSlice := []v2alpha1.RelatedImage{graphRelatedImage}
			graphCopySlice, err := o.prepareD2MCopyBatch(graphImageSlice)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
				return []v2alpha1.CopyImageSchema{}, err
			}
			// if there is no error, we are certain that the slice only contains 1 element
			// but double checking...
			if len(graphCopySlice) != 1 {
				o.Log.Error(errMsg, "error while calculating the destination reference for the graph image")
				return []v2alpha1.CopyImageSchema{}, fmt.Errorf(collectorPrefix + "error while calculating the destination reference for the graph image")
			}
			o.GraphDataImage = graphCopySlice[0].Destination
			allImages = append(allImages, graphCopySlice...)
		}
		releaseCopyImages, err := o.prepareD2MCopyBatch(allRelatedImages)
		if err != nil {
			o.Log.Error(errMsg, err.Error())
			return []v2alpha1.CopyImageSchema{}, err
		}
		allImages = append(allImages, releaseCopyImages...)
	}

	return allImages, nil
}

func (o LocalStorageCollector) prepareM2DCopyBatch(images []v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	var result []v2alpha1.CopyImageSchema
	for _, img := range images {
		var src string
		var dest string

		imgSpec, err := image.ParseRef(img.Image)
		if err != nil {
			o.Log.Error("%s", err.Error())
			return nil, err
		}
		src = imgSpec.ReferenceWithTransport
		if imgSpec.IsImageByDigest() {
			tag := fmt.Sprintf("%s-%s", imgSpec.Algorithm, imgSpec.Digest)
			if len(tag) > 128 {
				tag = tag[:127]
			}
			dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), imgSpec.PathComponent + ":" + tag}, "/")
		} else {
			dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), imgSpec.PathComponent + ":" + imgSpec.Tag}, "/")

		}
		o.Log.Debug("source %s", src)
		o.Log.Debug("destination %s", dest)
		result = append(result, v2alpha1.CopyImageSchema{Origin: img.Image, Source: src, Destination: dest, Type: img.Type})
	}
	return result, nil
}

func (o LocalStorageCollector) prepareD2MCopyBatch(images []v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	var result []v2alpha1.CopyImageSchema
	for _, img := range images {
		var src string
		var dest string

		imgSpec, err := image.ParseRef(img.Image)
		if err != nil {
			o.Log.Error("%s", err.Error())
			return nil, err
		}
		if imgSpec.IsImageByDigest() {
			tag := fmt.Sprintf("%s-%s", imgSpec.Algorithm, imgSpec.Digest)
			if len(tag) > 128 {
				tag = tag[:127]
			}
			src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent + ":" + tag}, "/")
			dest = strings.Join([]string{o.Opts.Destination, imgSpec.PathComponent + ":" + tag}, "/")
		} else {
			src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
			dest = strings.Join([]string{o.Opts.Destination, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
		}
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

	for _, value := range o.Cincinnati.GetReleaseReferenceImages(ctx) {
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
		graphCopyImage, err := o.prepareD2MCopyBatch(graphRelatedImage)
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
		releaseCopyImage, err := o.prepareD2MCopyBatch(releaseRelatedImage)
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
	o.Log.Info(fmt.Sprintf("kubeVirtContainer set to true [ including : %v ]", image))
	kubeVirtImage := v2alpha1.RelatedImage{
		Image: image,
		Name:  "KubeVirtContainer",
		Type:  v2alpha1.TypeOCPRelease,
	}
	return kubeVirtImage, nil
}

func graphURLFromUpdateURL(updateURL string) (string, error) {
	finalGraphURL := graphURL

	if updateURL != "" {
		originalURLStruct, err := url.Parse(graphURL)
		if err != nil {
			return "", err
		}
		updateURLStruct, err := url.Parse(updateURL)
		if err != nil {
			return "", err
		}
		finalGraphURL = strings.Replace(finalGraphURL, originalURLStruct.Host, updateURLStruct.Host, 1)
		finalGraphURL = strings.Replace(finalGraphURL, originalURLStruct.Scheme, updateURLStruct.Scheme, 1)
	}
	return finalGraphURL, nil
}
