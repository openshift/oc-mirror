package delete

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/archive"
	"github.com/openshift/oc-mirror/v2/internal/pkg/batch"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type DeleteImages struct {
	Log              clog.PluggableLoggerInterface
	Opts             mirror.CopyOptions
	Batch            batch.BatchInterface
	Blobs            archive.BlobsGatherer
	Config           v2alpha1.ImageSetConfiguration
	Manifest         manifest.ManifestInterface
	LocalStorageDisk string
	LocalStorageFQDN string
}

// WriteDeleteMetaData
func (o DeleteImages) WriteDeleteMetaData(images []v2alpha1.CopyImageSchema) error {
	o.Log.Info("📄 Generating delete file...")
	o.Log.Info("%s file created", o.Opts.Global.WorkingDir+deleteDir)

	// we write the image and related blobs in yaml format to file for further processing
	filename := filepath.Join(o.Opts.Global.WorkingDir, deleteImagesYaml)
	discYamlFile := filepath.Join(o.Opts.Global.WorkingDir, discYaml)
	// used for versioning and comparing
	if len(o.Opts.Global.DeleteID) > 0 {
		filename = filepath.Join(o.Opts.Global.WorkingDir, strings.ReplaceAll(deleteImagesYaml, ".", "-"+o.Opts.Global.DeleteID+"."))
		discYamlFile = filepath.Join(o.Opts.Global.WorkingDir, strings.ReplaceAll(discYaml, ".", "-"+o.Opts.Global.DeleteID+"."))
	}
	// create the delete folder
	err := os.MkdirAll(o.Opts.Global.WorkingDir+deleteDir, 0755)
	if err != nil {
		o.Log.Error("%v ", err)
	}
	var items_map = make(map[string]v2alpha1.DeleteItem)

	// gather related blobs
	for _, img := range images {

		item := v2alpha1.DeleteItem{
			ImageName:      img.Origin,
			ImageReference: img.Destination,
		}
		if err != nil {
			o.Log.Error("%v ", err)
		}
		i, err := o.Blobs.GatherBlobs(context.Background(), img.Source)
		if err != nil {
			o.Log.Error("%v image : %s", err, i)
		}
		if err != nil {
			o.Log.Error(deleteImagesErrMsg, err)
		}
		var blobs []string
		for k := range i {
			blobs = append(blobs, k)
			if err != nil {
				o.Log.Error("unable to write blob %s %v", k, err)
			}
		}
		sort.SliceStable(blobs, func(i, j int) bool {
			return blobs[i] < blobs[j]
		})
		item.RelatedBlobs = blobs
		items_map[img.Destination] = item
	}

	var items []v2alpha1.DeleteItem
	// convert back
	for _, v := range items_map {
		items = append(items, v)
	}
	// sort the items
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].ImageReference < items[j].ImageReference
	})
	// marshal to yaml and write to file
	deleteImageList := v2alpha1.DeleteImageList{
		Kind:       "DeleteImageList",
		APIVersion: "mirror.openshift.io/v2alpha1",
		Items:      items,
	}
	ymlData, err := yaml.Marshal(deleteImageList)
	if err != nil {
		o.Log.Error(deleteImagesErrMsg, err)
	}
	err = os.WriteFile(filename, ymlData, 0755)
	if err != nil {
		o.Log.Error(deleteImagesErrMsg, err)
	}
	// finally copy the deleteimagesetconfig for reference
	disc := v2alpha1.DeleteImageSetConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DeleteImageSetConfiguration",
			APIVersion: "mirror.openshift.io/v2alpha1",
		},
		DeleteImageSetConfigurationSpec: v2alpha1.DeleteImageSetConfigurationSpec{
			Delete: v2alpha1.Delete{
				Platform:         o.Config.Mirror.Platform,
				Operators:        o.Config.Mirror.Operators,
				AdditionalImages: o.Config.Mirror.AdditionalImages,
			},
		},
	}
	discYamlData, err := yaml.Marshal(disc)
	if err != nil {
		o.Log.Error("%v ", err)
	}
	err = os.WriteFile(discYamlFile, discYamlData, 0755)
	if err != nil {
		o.Log.Error(deleteImagesErrMsg, err)
	}
	return nil
}

// DeleteRegistryImages - deletes both remote and local registries
func (o DeleteImages) DeleteRegistryImages(images v2alpha1.DeleteImageList) error {
	o.Log.Debug("deleting images from remote registry")
	var rrUpdatedImages []v2alpha1.CopyImageSchema
	var lsUpdatedImages []v2alpha1.CopyImageSchema
	var lsUpdated string

	var batchError, cacheBatchError error

	for _, img := range images.Items {
		cis := v2alpha1.CopyImageSchema{
			Origin:      img.ImageName,
			Destination: img.ImageReference,
		}
		o.Log.Debug("deleting images %v", cis.Destination)
		rrUpdatedImages = append(rrUpdatedImages, cis)
		// prepare for local storage delete
		if o.Opts.Mode == mirror.MirrorToMirror {
			lsUpdated = img.ImageReference
		} else {
			lsUpdated = strings.Replace(img.ImageReference, o.Opts.Global.DeleteDestination, dockerProtocol+o.LocalStorageFQDN, -1)
		}
		lsCis := v2alpha1.CopyImageSchema{
			Origin:      img.ImageName,
			Destination: lsUpdated,
		}
		o.Log.Debug("deleting images local cache %v", lsCis.Destination)
		lsUpdatedImages = append(lsUpdatedImages, lsCis)
	}

	// ensure output is suppressed
	o.Opts.Stdout = io.Discard
	if !o.Opts.Global.DeleteGenerate && len(o.Opts.Global.DeleteDestination) > 0 {
		if _, err := o.Batch.Worker(context.Background(), v2alpha1.CollectorSchema{AllImages: rrUpdatedImages}, o.Opts); err != nil {
			if _, ok := err.(batch.UnsafeError); ok {
				return err
			} else {
				batchError = err
			}
		}
	}
	// if mirrortoMirror mode no conetents were stored to the cache
	// so just skip
	if o.Opts.Global.ForceCacheDelete && o.Opts.Mode != mirror.MirrorToMirror {
		if _, err := o.Batch.Worker(context.Background(), v2alpha1.CollectorSchema{AllImages: lsUpdatedImages}, o.Opts); err != nil {
			if _, ok := err.(batch.UnsafeError); ok {
				return err
			} else {
				cacheBatchError = err
			}
		}
	}

	if batchError != nil {
		o.Log.Warn("error during registry deletion: %v", batchError)
	}
	if cacheBatchError != nil {
		o.Log.Warn("error during cache deletion: %v", cacheBatchError)
	}
	return nil
}

// ReadDeleteMetaData - read the list of images to delete
// used to verify the delete yaml is well formed as well as being
// the base for both local cache delete and remote registry delete
func (o DeleteImages) ReadDeleteMetaData() (v2alpha1.DeleteImageList, error) {
	o.Log.Info("👀 Reading delete file...")
	var list v2alpha1.DeleteImageList
	var fileName string

	if len(o.Opts.Global.DeleteYaml) == 0 {
		fileName = filepath.Join(o.Opts.Global.WorkingDir, deleteImagesYaml)
		if _, err := os.Stat(fileName); os.IsNotExist(err) {
			return list, fmt.Errorf("delete yaml file %s does not exist (please perform a delete with --dry-run)", fileName)
		}
	} else {
		fileName = o.Opts.Global.DeleteYaml
	}

	data, err := os.ReadFile(fileName)
	if err != nil {
		return list, err
	}
	// lets parse the file to get the images
	err = yaml.Unmarshal(data, &list)
	if err != nil {
		return list, err
	}
	return list, nil
}

// ConvertReleaseImages
func (o DeleteImages) ConvertReleaseImages(ri []v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	// convert and format the collection
	var allImages []v2alpha1.CopyImageSchema
	var err error
	var copyIS v2alpha1.CopyImageSchema

	for _, img := range ri {
		// replace the destination registry with our local registry
		if o.Opts.Mode == mirror.MirrorToMirror {
			copyIS, err = buildFormatedCopyImageSchema(img.Image, o.Opts.Global.DeleteDestination, o.Opts.Global.DeleteDestination)
		} else {
			copyIS, err = buildFormatedCopyImageSchema(img.Image, dockerProtocol+o.LocalStorageFQDN, o.Opts.Global.DeleteDestination)
		}
		if err != nil {
			return []v2alpha1.CopyImageSchema{}, err
		}
		allImages = append(allImages, copyIS)
	}
	return allImages, nil
}

// buildFormatedCopyImageSchema - simple private utility to build the CopyImageSchema data
func buildFormatedCopyImageSchema(img, cacheRegistry, targetRegistry string) (v2alpha1.CopyImageSchema, error) {
	var src, dest string
	imgSpec, err := image.ParseRef(img)
	if err != nil {
		return v2alpha1.CopyImageSchema{}, err
	}
	if imgSpec.IsImageByDigest() {
		src = strings.Join([]string{cacheRegistry, imgSpec.PathComponent + "@sha256:" + imgSpec.Digest}, "/")
		dest = strings.Join([]string{targetRegistry, imgSpec.PathComponent + "@sha256:" + imgSpec.Digest}, "/")
	} else {
		src = strings.Join([]string{cacheRegistry, imgSpec.PathComponent + ":" + imgSpec.Tag}, "/")
		dest = strings.Join([]string{targetRegistry, imgSpec.PathComponent + ":" + imgSpec.Tag}, "/")
	}

	is := v2alpha1.CopyImageSchema{
		Source:      src,
		Destination: dest,
		Origin:      img,
	}
	return is, nil
}

// FilterReleasesForDelete returns a map of releases that should be deleted
func (o DeleteImages) FilterReleasesForDelete() (map[string][]v2alpha1.RelatedImage, error) {
	// get the release data from the deleteimagesetconfig
	ri := map[string][]v2alpha1.RelatedImage{}
	release_hold_path := filepath.Join(o.Opts.Global.WorkingDir, releaseImageExtractDir, ocpRelease)
	folders, err := os.ReadDir(release_hold_path)
	if err != nil {
		return nil, err
	}

	// iterate through the hold-release folder structure
	for _, dir := range folders {
		// this should always be in the format semver-arch
		semver_dir := strings.Split(dir.Name(), "-")
		current, err := semver.Parse(semver_dir[0])
		architecture := semver_dir[1]
		if err != nil {
			return nil, err
		}
		// if no architecture is specified default to x86_64
		if len(o.Config.Mirror.Platform.Architectures) == 0 {
			o.Config.Mirror.Platform.Architectures = []string{x86_64}
		} else {
			// if multi is set
			if o.Config.Mirror.Platform.Architectures[0] == "multi" {
				o.Config.Mirror.Platform.Architectures = []string{multi}
			}
		}
		for _, arch := range o.Config.Mirror.Platform.Architectures {
			if arch == "amd64" {
				arch = x86_64
			}
			if arch == "arm64" {
				arch = aarch64
			}
			for _, ch := range o.Config.Mirror.Platform.Channels {
				semverMin := semver.MustParse("0.0.0")
				semverMax := semver.MustParse("9999.9999.9999")
				if ch.MinVersion != "" {
					semverMin = semver.MustParse(ch.MinVersion)
				}
				if ch.MaxVersion != "" {
					semverMax = semver.MustParse(ch.MaxVersion)
				}
				if current.GTE(semverMin) && current.LTE(semverMax) && arch == architecture {
					rm := filepath.Join(release_hold_path, dir.Name(), releaseImageExtractFullPath)
					releaseImages, err := o.Manifest.GetReleaseSchema(rm)
					if err != nil {
						return nil, err
					}
					releaseImage := v2alpha1.RelatedImage{
						Name:  releaseRepo + ":" + dir.Name(),
						Image: releaseRepo + ":" + dir.Name(),
						Type:  v2alpha1.TypeOCPRelease,
					}
					releaseImages = append(releaseImages, releaseImage)
					ri[dir.Name()] = releaseImages
				}
			}
		}
	}
	return ri, nil
}
