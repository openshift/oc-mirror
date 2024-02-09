package release

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	digest "github.com/opencontainers/go-digest"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/image"
	"github.com/openshift/oc-mirror/v2/pkg/imagebuilder"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

type releasesForFilter struct {
	Filter   v1alpha2.Platform          `json:"filter"`
	Releases []v1alpha3.CopyImageSchema `json:"releases"`
}

type LocalStorageCollector struct {
	CollectorInterface
	Log              clog.PluggableLoggerInterface
	LogsDir          string
	Mirror           mirror.MirrorInterface
	Manifest         manifest.ManifestInterface
	Config           v1alpha2.ImageSetConfiguration
	Opts             mirror.CopyOptions
	Cincinnati       CincinnatiInterface
	LocalStorageFQDN string
	ImageBuilder     imagebuilder.ImageBuilderInterface
	Releases         []string
	GraphDataImage   string
}

func (o *LocalStorageCollector) ReleaseImageCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error) {
	o.Opts.MultiArch = "system" // we just care for 1 platform release, in order to read release images
	o.Log.Debug("setting copy option multiArch=system for retrieving release image")
	var allImages []v1alpha3.CopyImageSchema
	var imageIndexDir string
	filterCopy := o.Config.Mirror.Platform.DeepCopy()
	if o.Opts.IsMirrorToDisk() || o.Opts.IsPrepare() {
		releases := o.Cincinnati.GetReleaseReferenceImages(ctx)

		releasesForFilter := releasesForFilter{
			Filter: filterCopy,
			//cannot directly use the array releases here as the Destinations are still empty
			Releases: []v1alpha3.CopyImageSchema{},
		}

		for _, value := range releases {
			hld := strings.Split(value.Source, "/")
			imageIndexDir = strings.Replace(hld[len(hld)-1], ":", "/", -1)
			cacheDir := filepath.Join(o.Opts.Global.WorkingDir, releaseImageExtractDir, imageIndexDir)
			dir := filepath.Join(o.Opts.Global.WorkingDir, releaseImageDir, imageIndexDir)

			//Save to releasesForFilter so that we can reuse it during Disk To Mirror flow
			src := dockerProtocol + value.Source
			dest := ociProtocolTrimmed + dir
			r := v1alpha3.CopyImageSchema{
				Source:      src,
				Destination: dest,
			}
			releasesForFilter.Releases = append(releasesForFilter.Releases, r)

			if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
				o.Log.Info("copying  %s ", value.Source)
				err := os.MkdirAll(dir, 0755)
				if err != nil {
					return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, err)
				}
				err = o.Mirror.Run(ctx, src, dest, "copy", &o.Opts)
				if err != nil {
					return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, err)
				}
				o.Log.Debug("copied release index image %s ", value.Source)

				f, _ := os.ReadFile(logFile)
				lines := strings.Split(string(f), "\n")
				for _, s := range lines {
					if len(s) > 0 {
						o.Log.Debug(" %s ", strings.ToLower(s))
					}
				}
			} else {
				o.Log.Info("cache release-index directory exists %s", cacheDir)
			}

			oci, err := o.Manifest.GetImageIndex(dir)
			if err != nil {
				o.Log.Error("[ReleaseImageCollector] %v ", err)
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, err)
			}

			//read the link to the manifest
			if len(oci.Manifests) == 0 {
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, "image index not found ")
			}
			validDigest, err := digest.Parse(oci.Manifests[0].Digest)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf("[ReleaseImageCollector] invalid digest for image index %s: %v", oci.Manifests[0].Digest, err)
			}

			manifest := validDigest.Encoded()
			o.Log.Debug("image index %v", manifest)

			manifestDir := filepath.Join(dir, blobsDir, manifest)
			mfst, err := o.Manifest.GetImageManifest(manifestDir)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, err)
			}
			o.Log.Debug("manifest %v ", oci.Config.Digest)

			fromDir := strings.Join([]string{dir, blobsDir}, "/")
			err = o.Manifest.ExtractLayersOCI(fromDir, cacheDir, releaseManifests, mfst)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, err)
			}
			o.Log.Debug("extracted layer %s ", cacheDir)

			// overkill but its used for consistency
			releaseDir := strings.Join([]string{cacheDir, releaseImageExtractFullPath}, "/")
			allRelatedImages, err := o.Manifest.GetReleaseSchema(releaseDir)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, err)
			}
			//add the release image itself
			allRelatedImages = append(allRelatedImages, v1alpha3.RelatedImage{Image: value.Source, Name: value.Source, Type: v1alpha2.TypeOCPRelease})
			tmpAllImages, err := o.prepareM2DCopyBatch(o.Log, allRelatedImages)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, err
			}
			allImages = append(allImages, tmpAllImages...)
		}
		// save the releasesForFilter to json cache,
		// so that it can be used during diskToMirror flow
		err := o.saveReleasesForFilter(releasesForFilter, filepath.Join(o.Opts.Global.WorkingDir, releaseFiltersDir))
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, fmt.Errorf("[ReleaseImageCollector] unable to save cincinnati response: %v", err)
		}

		if !o.Opts.IsPrepare() && o.Config.Mirror.Platform.Graph {
			o.Log.Info("creating graph data image")
			graphImgRef, err := o.CreateGraphImage(ctx, graphURL)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, err
			}
			o.Log.Info("graph image created and pushed to cache.")
			// still add the graph image to the `allImages` so that we later can add it in the tar.gz archive
			graphCopy := v1alpha3.CopyImageSchema{
				Source:      graphImgRef,
				Destination: graphImgRef,
				Origin:      graphImgRef,
				Type:        v1alpha2.TypeCincinnatiGraph,
			}
			allImages = append(allImages, graphCopy)
		}

	} else if o.Opts.IsDiskToMirror() {

		releaseImages, releaseFolders, err := o.identifyReleases()
		if err != nil {
			return allImages, err
		}
		o.Releases = []string{}
		for _, img := range releaseImages {
			o.Releases = append(o.Releases, img.Image)
		}

		allRelatedImages := []v1alpha3.RelatedImage{}

		// add the releaseImages so that they are added to the list of images to copy
		allRelatedImages = append(allRelatedImages, releaseImages...)

		for _, releaseDir := range releaseFolders {

			// get all release images from manifest (json)
			imageReferencesFile := filepath.Join(releaseDir, releaseManifests, imageReferences)
			releaseRelatedImages, err := o.Manifest.GetReleaseSchema(imageReferencesFile)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, err)
			}
			allRelatedImages = append(allRelatedImages, releaseRelatedImages...)
		}
		if o.Config.Mirror.Platform.Graph {
			o.Log.Info("adding graph data image")
			graphRelatedImage := v1alpha3.RelatedImage{
				Name: graphImageName,
				// Supposing that the mirror to disk saved the image with the latest tag
				// If this supposition is false, then we need to implement a mechanism to save
				// the digest of the graph image and use it here
				Image: filepath.Join(o.LocalStorageFQDN, graphImageName) + ":latest",
				Type:  v1alpha2.TypeCincinnatiGraph,
			}
			// OCPBUGS-26513: In order to get the destination for the graphDataImage
			// into `o.GraphDataImage`, we call `prepareD2MCopyBatch` on an array
			// containing only the graph image. This way we can easily identify the destination
			// of the graph image.
			graphImageSlice := []v1alpha3.RelatedImage{graphRelatedImage}
			graphCopySlice, err := o.prepareD2MCopyBatch(o.Log, graphImageSlice)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, err
			}
			// if there is no error, we are certain that the slice only contains 1 element
			// but double checking...
			if len(graphCopySlice) != 1 {
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf("error while calculating the destination reference for the graph image")
			}
			o.GraphDataImage = graphCopySlice[0].Destination
			allImages = append(allImages, graphCopySlice...)
		}
		releaseCopyImages, err := o.prepareD2MCopyBatch(o.Log, allRelatedImages)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}
		allImages = append(allImages, releaseCopyImages...)
	}

	return allImages, nil
}

func (o LocalStorageCollector) prepareD2MCopyBatch(log clog.PluggableLoggerInterface, images []v1alpha3.RelatedImage) ([]v1alpha3.CopyImageSchema, error) {
	var result []v1alpha3.CopyImageSchema
	for _, img := range images {
		var src string
		var dest string

		imgSpec, err := image.ParseRef(img.Image)
		if err != nil {
			o.Log.Error("%s", err.Error())
			return nil, err
		}
		if imgSpec.IsImageByDigest() {
			src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent + "@" + imgSpec.Algorithm + ":" + imgSpec.Digest}, "/")
			dest = strings.Join([]string{o.Opts.Destination, imgSpec.PathComponent + "@" + imgSpec.Algorithm + ":" + imgSpec.Digest}, "/")
		} else {
			src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
			dest = strings.Join([]string{o.Opts.Destination, imgSpec.PathComponent}, "/") + ":" + imgSpec.Tag
		}
		if src == "" || dest == "" {
			return result, fmt.Errorf("unable to determine src %s or dst %s for %s", src, dest, img.Name)
		}

		o.Log.Debug("source %s", src)
		o.Log.Debug("destination %s", dest)
		result = append(result, v1alpha3.CopyImageSchema{Origin: img.Image, Source: src, Destination: dest, Type: img.Type})

	}
	return result, nil
}

func (o LocalStorageCollector) prepareM2DCopyBatch(log clog.PluggableLoggerInterface, images []v1alpha3.RelatedImage) ([]v1alpha3.CopyImageSchema, error) {
	var result []v1alpha3.CopyImageSchema
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
			dest = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent + "@" + imgSpec.Algorithm + ":" + imgSpec.Digest}, "/")
		} else {
			dest = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent + ":" + imgSpec.Tag}, "/")

		}
		o.Log.Debug("source %s", src)
		o.Log.Debug("destination %s", dest)
		result = append(result, v1alpha3.CopyImageSchema{Source: src, Destination: dest, Type: img.Type})
	}
	return result, nil
}

func (o LocalStorageCollector) identifyReleases() ([]v1alpha3.RelatedImage, []string, error) {
	//Find the filter file, containing all the images that correspond to the filter
	rff := releasesForFilter{
		Filter: o.Config.Mirror.Platform,
	}
	filter := fmt.Sprintf("%v", rff.Filter)
	filterFileName := fmt.Sprintf("%x", md5.Sum([]byte(filter)))[0:32]
	filterFilePath := filepath.Join(o.Opts.Global.WorkingDir, releaseFiltersDir, filterFileName)
	dat, err := os.ReadFile(filterFilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to read file %s: %v", filterFilePath, err)
	}

	err = json.Unmarshal(dat, &rff)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to unmarshall contents of %s: %v", filterFilePath, err)
	}

	releaseImageCopies := rff.Releases
	releaseFolders := []string{}
	releaseImages := []v1alpha3.RelatedImage{}
	for _, copy := range releaseImageCopies {
		releasePath := strings.TrimPrefix(copy.Destination, ociProtocol)
		releasePath = strings.TrimPrefix(releasePath, ociProtocolTrimmed)
		releaseHoldPath := strings.Replace(releasePath, releaseImageDir, releaseImageExtractDir, 1)
		releaseFolders = append(releaseFolders, releaseHoldPath)
		releaseImages = append(releaseImages, v1alpha3.RelatedImage{Name: copy.Source, Image: copy.Source, Type: v1alpha2.TypeOCPRelease})
	}
	return releaseImages, releaseFolders, nil
}

func (o LocalStorageCollector) saveReleasesForFilter(r releasesForFilter, to string) error {
	toJson, err := json.Marshal(r)
	if err != nil {
		return err
	}
	filter := fmt.Sprintf("%v", r.Filter)
	filterFileName := fmt.Sprintf("%x", md5.Sum([]byte(filter)))[0:32]

	if _, err := os.Stat(to); errors.Is(err, os.ErrNotExist) {
		o.Log.Info("copying  cincinnati response to %s", to)
		err := os.MkdirAll(to, 0755)
		if err != nil {
			return err
		}
	}

	filterFile, err := os.Create(filepath.Join(to, filterFileName))
	if err != nil {
		return err
	}
	defer filterFile.Close()

	_, err = filterFile.Write([]byte(toJson))
	if err != nil {
		return err
	}
	return nil
}

// assumes this is called during DiskToMirror workflow.
// this method doesn't verify if the graphImage has been generated
// by the collector.
func (o *LocalStorageCollector) GraphImage() (string, error) {
	if o.GraphDataImage == "" {
		sourceGraphDataImage := filepath.Join(o.LocalStorageFQDN, graphImageName) + ":latest"
		graphRelatedImage := []v1alpha3.RelatedImage{
			{
				Name:  "release",
				Image: sourceGraphDataImage,
				Type:  v1alpha2.TypeCincinnatiGraph,
			},
		}
		graphCopyImage, err := o.prepareD2MCopyBatch(nil, graphRelatedImage)
		if err != nil {
			return "", fmt.Errorf("collector could not establish the destination for the graph image: %v", err)
		}
		o.GraphDataImage = graphCopyImage[0].Destination
	}
	return o.GraphDataImage, nil
}

// assumes that this is called during DiskToMirror workflow.
// it relies on the previously saved release-filters in order
// to get the list of releases to mirror (saved during mirrorToDisk
// after the call to cincinnati API)
func (o *LocalStorageCollector) ReleaseImage() (string, error) {
	if len(o.Releases) == 0 {
		releaseImages, _, err := o.identifyReleases()
		if err != nil {
			return "", fmt.Errorf("collector could not establish the destination for the release image: %v", err)
		}
		o.Releases = []string{}
		for _, img := range releaseImages {
			o.Releases = append(o.Releases, img.Image)
		}
	}
	if len(o.Releases) > 0 {
		releaseRelatedImage := []v1alpha3.RelatedImage{
			{
				Name:  "release",
				Image: o.Releases[0],
				Type:  v1alpha2.TypeOCPRelease,
			},
		}
		releaseCopyImage, err := o.prepareD2MCopyBatch(nil, releaseRelatedImage)
		if err != nil {
			return "", fmt.Errorf("collector could not establish the destination for the release image: %v", err)
		}
		return releaseCopyImage[0].Destination, nil

	} else {
		return "", fmt.Errorf("collector could not establish the destination for the release image")
	}
}
