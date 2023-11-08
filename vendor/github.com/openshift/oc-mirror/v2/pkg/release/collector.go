package release

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

type Collector struct {
	Log        clog.PluggableLoggerInterface
	Mirror     mirror.MirrorInterface
	Manifest   manifest.ManifestInterface
	Config     v1alpha2.ImageSetConfiguration
	Opts       mirror.CopyOptions
	Cincinnati CincinnatiInterface
}

// ReleaseImageCollector - this looks into the operator index image
// taking into account the mode we are in (mirrorToDisk, diskToMirror)
// the image is downloaded (preserve originator format could be dockckerv2 or oci)
// and the index.json is inspected once unmarshalled, the links to manifests are then inspected
func (o *Collector) ReleaseImageCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error) {

	var allImages []v1alpha3.CopyImageSchema
	var imageIndexDir string

	if o.Opts.IsMirrorToDisk() {
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

			tmpImages, err := batcWorkerConverter(o.Log, dir, allRelatedImages)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, err)
			}

			// need to append images
			allImages = append(allImages, tmpImages...)
		}
	}
	if o.Opts.IsDiskToMirror() && strings.Contains(o.Config.Mirror.Platform.Release, dirProtocol) {
		// we know the directory format is
		// release-images/name/version/
		// we can do some replacing from the directory passed as string
		// so tha twe can access the image-references
		str := strings.Replace(o.Config.Mirror.Platform.Release, "release-images", "hold-release", 1)
		// remove the file prefix
		str = strings.Replace(str, "dir://", "", 1)

		// get all release images from manifest (json)
		allRelatedImages, err := o.Manifest.GetReleaseSchema(str + "/" + releaseImageExtractFullPath)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, err)
		}

		// set up a regex for the manifest.json (checked in each directory)
		regex, e := regexp.Compile(indexJson)
		if e != nil {
			o.Log.Error("%v", e)
		}

		// walk through the directory structure to look for manifest.json files
		// get the base directory and do a lookup on the actual image to mirror
		imagesDir := strings.Replace(o.Config.Mirror.Platform.Release, "dir://", "", 1)
		imagesDir = imagesDir + "/images"
		errFP := filepath.Walk(imagesDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && regex.MatchString(info.Name()) {
				component := strings.Split(filepath.Dir(path), "/")
				img := findRelatedImage(component[len(component)-1], allRelatedImages)
				if len(img) > 0 {
					src := dirProtocolTrimmed + filepath.Dir(path)
					dest := o.Opts.Destination + "/" + img
					allImages = append(allImages, v1alpha3.CopyImageSchema{Source: src, Destination: dest})
				} else {
					o.Log.Warn("component not found %s", component[len(component)-1])
				}
			} else if err != nil {
				return err
			}
			return nil
		})
		if errFP != nil {
			return []v1alpha3.CopyImageSchema{}, e
		}
	}
	return allImages, nil
}

// batchWorkerConverter convert RelatedImages to strings for batch worker
func batcWorkerConverter(log clog.PluggableLoggerInterface, dir string, images []v1alpha3.RelatedImage) ([]v1alpha3.CopyImageSchema, error) {
	var result []v1alpha3.CopyImageSchema
	for _, img := range images {
		src := dockerProtocol + img.Image
		dest := dirProtocolTrimmed + strings.Join([]string{dir, "images", img.Name}, "/")
		// do a lookup on dist first
		if _, err := os.Stat(dir + "/images/" + img.Name); errors.Is(err, os.ErrNotExist) {
			err := os.MkdirAll(dir+"/images/"+img.Name, 0750)
			if err != nil {
				log.Error("[batchWorkerConverter] %v", err)
				return []v1alpha3.CopyImageSchema{}, err
			}
			log.Debug("source %s ", src)
			log.Debug("destination %s ", dest)
			result = append(result, v1alpha3.CopyImageSchema{Source: src, Destination: dest})
		} else {
			log.Info("image in cache %s ", dir+"/images/"+img.Name)
		}

	}
	return result, nil
}

// findRelatedImage
func findRelatedImage(name string, imgs []v1alpha3.RelatedImage) string {
	for _, img := range imgs {
		if name == img.Name {
			strip := strings.Split(img.Image, "/")
			return strings.Join(strip[1:], "/")
		}
	}
	return ""
}
