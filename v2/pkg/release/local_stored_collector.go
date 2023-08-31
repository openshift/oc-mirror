package release

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

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

	if o.Opts.Mode == mirrorToDisk {
		releases := o.Cincinnati.GetReleaseReferenceImages(ctx)
		f, err := os.Create(logFile)
		if err != nil {
			o.Log.Error("[ReleaseImageCollector] %v", err)
		}

		writer := bufio.NewWriter(f)
		defer f.Close()
		for _, value := range releases {
			hld := strings.Split(value.Source, "/")
			imageIndexDir = strings.Replace(hld[len(hld)-1], ":", "/", -1)
			cacheDir := strings.Join([]string{o.Opts.Global.Dir, releaseImageExtractDir, imageIndexDir}, "/")
			dir := strings.Join([]string{o.Opts.Global.Dir, releaseImageDir, imageIndexDir}, "/")
			if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
				o.Log.Info("copying  %s ", value.Source)
				err := os.MkdirAll(dir, 0755)
				if err != nil {
					return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, err)
				}
				src := dockerProtocol + value.Source
				dest := ociProtocolTrimmed + dir
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
			tmpAllImages, err := o.prepareM2DCopyBatch(o.Log, allRelatedImages)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, err
			}
			allImages = append(allImages, tmpAllImages...)
			allImages = append(allImages, value)

		}
	} else if o.Opts.Mode == diskToMirror {
		releases := o.Cincinnati.GetReleaseReferenceImages(ctx)

		for _, value := range releases {
			hld := strings.Split(value.Source, "/")
			imageIndexDir = strings.Replace(hld[len(hld)-1], ":", "/", -1)
			// cacheDir := strings.Join([]string{o.Opts.Global.Dir, releaseImageExtractDir, imageIndexDir}, "/")
			dir := strings.Join([]string{o.Opts.Global.Dir, releaseImageDir, imageIndexDir}, "/")
			if _, err := os.Stat(dir); err != nil {
				return nil, fmt.Errorf("unable to find the release %s on disk : %v", value.Source, err)
			}

			// get all release images from manifest (json)
			allRelatedImages, err := o.Manifest.GetReleaseSchema(imageIndexDir)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, err)
			}

			tmpAllImages, err := o.prepareD2MCopyBatch(o.Log, allRelatedImages)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, err
			}
			allImages = append(allImages, tmpAllImages...)
			allImages = append(allImages, value)
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
		src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, img.Image}, "/")
		dst = strings.Join([]string{o.Opts.Destination, img.Image}, "/")

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
