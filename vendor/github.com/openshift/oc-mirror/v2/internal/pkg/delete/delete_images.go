package delete

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/archive"
	"github.com/openshift/oc-mirror/v2/internal/pkg/batch"
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

	var items []v2alpha1.DeleteItem
	for _, img := range images {

		item := v2alpha1.DeleteItem{
			ImageName:      img.Origin,
			ImageReference: img.Destination,
		}

		items = append(items, item)
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
		lsUpdated = strings.Replace(img.ImageReference, o.Opts.Global.DeleteDestination, dockerProtocol+o.LocalStorageFQDN, -1)

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
	if o.Opts.Global.ForceCacheDelete {
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
