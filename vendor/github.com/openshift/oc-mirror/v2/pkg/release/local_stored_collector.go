package release

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

type releasesForFilter struct {
	Filter   v1alpha2.Platform          `json:"filter"`
	Releases []v1alpha3.CopyImageSchema `json:"releases"`
}

func NewWithLocalStorage(log clog.PluggableLoggerInterface,
	config v1alpha2.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
	cincinnati CincinnatiInterface,
	localStorageFQDN string,
) CollectorInterface {
	return &LocalStorageCollector{Log: log, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest, Cincinnati: cincinnati, LocalStorageFQDN: localStorageFQDN}
}

type LocalStorageCollector struct {
	Log              clog.PluggableLoggerInterface
	Mirror           mirror.MirrorInterface
	Manifest         manifest.ManifestInterface
	Config           v1alpha2.ImageSetConfiguration
	Opts             mirror.CopyOptions
	Cincinnati       CincinnatiInterface
	LocalStorageFQDN string
}

func (o *LocalStorageCollector) ReleaseImageCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error) {
	var allImages []v1alpha3.CopyImageSchema
	var imageIndexDir string
	filterCopy := o.Config.Mirror.Platform.DeepCopy()
	if o.Opts.Mode == mirrorToDisk {
		releases := o.Cincinnati.GetReleaseReferenceImages(ctx)

		releasesForFilter := releasesForFilter{
			Filter: filterCopy,
			//cannot directly use the array releases here as the Destinations are still empty
			Releases: []v1alpha3.CopyImageSchema{},
		}

		f, err := os.Create(logFile)
		if err != nil {
			o.Log.Error("[ReleaseImageCollector] %v", err)
		}

		writer := bufio.NewWriter(f)
		defer f.Close()
		for _, value := range releases {
			hld := strings.Split(value.Source, "/")
			imageIndexDir = strings.Replace(hld[len(hld)-1], ":", "/", -1)
			cacheDir := filepath.Join(o.Opts.Global.Dir, releaseImageExtractDir, imageIndexDir)
			dir := filepath.Join(o.Opts.Global.Dir, releaseImageDir, imageIndexDir)

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

				err = o.Mirror.Run(ctx, src, dest, "copy", &o.Opts, *writer)
				if err != nil {
					return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, err)
				}
				o.Log.Debug("copied release index image %s ", value.Source)

				// TODO: create common function to show logs
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
			manifest := strings.Split(oci.Manifests[0].Digest, ":")[1]
			o.Log.Debug("image index %v", manifest)

			manifestDir := strings.Join([]string{dir, blobsDir, manifest}, "/")
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
			allRelatedImages = append(allRelatedImages, v1alpha3.RelatedImage{Image: value.Source, Name: value.Source})
			tmpAllImages, err := o.prepareM2DCopyBatch(o.Log, allRelatedImages)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, err
			}
			allImages = append(allImages, tmpAllImages...)

		}
		// save the releasesForFilter to json cache,
		// so that it can be used during diskToMirror flow
		err = o.saveReleasesForFilter(releasesForFilter, filepath.Join(o.Opts.Global.Dir, releaseFiltersDir))
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, fmt.Errorf("[ReleaseImageCollector] unable to save cincinnati response: %v", err)
		}
	} else if o.Opts.Mode == diskToMirror {

		releaseImages, releaseFolders, err := o.identifyReleases()
		if err != nil {
			return allImages, err
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
		allImages, err = o.prepareD2MCopyBatch(o.Log, allRelatedImages)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}

	}

	return allImages, nil
}

func (o *LocalStorageCollector) prepareD2MCopyBatch(log clog.PluggableLoggerInterface, images []v1alpha3.RelatedImage) ([]v1alpha3.CopyImageSchema, error) {
	var result []v1alpha3.CopyImageSchema
	for _, img := range images {
		// TODO Make this more complete
		// This logic will be useful for operators and releases
		// strip the domain name from the img.Name
		src := ""
		dst := ""

		domainAndPathComps := img.Image
		// pathComponents := img.Name
		// temporarily strip out the transport
		transportAndRef := strings.Split(domainAndPathComps, "://")
		if len(transportAndRef) > 1 {
			domainAndPathComps = transportAndRef[1]
		}
		src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, domainAndPathComps}, "/")
		dst = strings.Join([]string{o.Opts.Destination, domainAndPathComps}, "/")

		// the following is for having the destination without the initial domain name => later
		// domainAndPathCompsArray := strings.Split(domainAndPathComps, "/")
		// if len(domainAndPathCompsArray) > 2 {
		// 	pathComponents = strings.Join(domainAndPathCompsArray[1:], "/")
		// } else {
		// 	return allImages, fmt.Errorf("unable to parse image %s correctly", img.Name)
		// }
		// src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathComponents}, "/")
		// dst = strings.Join([]string{o.Opts.Destination, pathComponents}, "/") // already has a transport protocol

		if src == "" || dst == "" {
			return result, fmt.Errorf("unable to determine src %s or dst %s for %s", src, dst, img.Name)
		}

		o.Log.Debug("source %s", src)
		o.Log.Debug("destination %s", dst)
		result = append(result, v1alpha3.CopyImageSchema{Source: src, Destination: dst})

	}
	return result, nil
}

func (o *LocalStorageCollector) prepareM2DCopyBatch(log clog.PluggableLoggerInterface, images []v1alpha3.RelatedImage) ([]v1alpha3.CopyImageSchema, error) {
	var result []v1alpha3.CopyImageSchema
	for _, img := range images {
		imgName := img.Image
		src := ""
		if !strings.Contains(src, "://") { // no transport was provided, assume docker://
			src = dockerProtocol + imgName
		} else {
			transportAndRef := strings.Split(imgName, "://")
			imgName = transportAndRef[1] // because we are reusing this to construct dest
		}

		dest := dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgName}, "/")
		o.Log.Debug("source %s", src)
		o.Log.Debug("destination %s", dest)
		result = append(result, v1alpha3.CopyImageSchema{Source: src, Destination: dest})
	}
	return result, nil
}

func (o *LocalStorageCollector) identifyReleases() ([]v1alpha3.RelatedImage, []string, error) {
	//Find the filter file, containing all the images that correspond to the filter
	rff := releasesForFilter{
		Filter: o.Config.Mirror.Platform,
	}
	filter := fmt.Sprintf("%v", rff.Filter)
	filterFileName := fmt.Sprintf("%x", md5.Sum([]byte(filter)))[0:32]
	filterFilePath := filepath.Join(o.Opts.Global.Dir, releaseFiltersDir, filterFileName)
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
		releaseImages = append(releaseImages, v1alpha3.RelatedImage{Name: copy.Source, Image: copy.Source})
	}
	return releaseImages, releaseFolders, nil
}

func (o *LocalStorageCollector) saveReleasesForFilter(r releasesForFilter, to string) error {
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
