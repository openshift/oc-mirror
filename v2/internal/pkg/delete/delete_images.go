package delete

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/archive"
	"github.com/openshift/oc-mirror/v2/internal/pkg/batch"
	"github.com/openshift/oc-mirror/v2/internal/pkg/emoji"
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
	o.Log.Info(emoji.PageFacingUp + " Generating delete file...")
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

	duplicates := []string{}
	var items []v2alpha1.DeleteItem
	for _, img := range images {
		if slices.Contains(duplicates, img.Origin) {
			o.Log.Debug("duplicate image found %s", img.Origin)
		} else {
			duplicates = append(duplicates, img.Origin)
			item := v2alpha1.DeleteItem{
				ImageName:      img.Origin,
				ImageReference: img.Destination,
				Type:           img.Type,
			}
			items = append(items, item)
		}
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
				Helm:             o.Config.Mirror.Helm,
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
func (o DeleteImages) DeleteRegistryImages(deleteImageList v2alpha1.DeleteImageList) error {
	o.Log.Debug("deleting images from remote registry")
	collectorSchema := v2alpha1.CollectorSchema{AllImages: []v2alpha1.CopyImageSchema{}}

	increment := 1
	if o.Opts.Global.ForceCacheDelete {
		increment = 2
	}

	for _, img := range deleteImageList.Items {
		// OCPBUGS-43489
		// Verify that the "delete" destination is set correctly
		// It does not hurt to check each entry :)
		// This will avoid the error "Image may not exist or is not stored with a v2 Schema in a v2 registry"
		// Reverts OCPBUGS-44448
		imgSpecName, err := image.ParseRef(img.ImageName)
		if err != nil {
			return err
		}
		imgSpecRef, err := image.ParseRef(img.ImageReference)
		if err != nil {
			return err
		}
		// remove dockerProtocol
		name := strings.Split(o.Opts.Global.DeleteDestination, dockerProtocol)
		// this should not occur - but just incase
		if len(name) < 2 {
			return fmt.Errorf("delete destination is not well formed (%s) - missing dockerProtocol?", o.Opts.Global.DeleteDestination)
		}
		assembleName := name[1] + "/" + imgSpecName.PathComponent
		// check image type for release or release content
		switch img.Type {
		case v2alpha1.TypeOCPReleaseContent:
			assembleName = name[1] + "/openshift/release"
		case v2alpha1.TypeOCPRelease:
			assembleName = name[1] + "/openshift/release-images"
		}
		// check the assembled name against the reference name
		if assembleName != imgSpecRef.Name {
			return fmt.Errorf("delete destination %s does not match values found in the delete-images yaml file (please verify full name)", o.Opts.Global.DeleteDestination)
		}
		cis := v2alpha1.CopyImageSchema{
			Origin:      img.ImageName,
			Destination: img.ImageReference,
			Type:        img.Type,
		}
		o.Log.Debug("deleting images %v", cis.Destination)
		collectorSchema.AllImages = append(collectorSchema.AllImages, cis)

		if o.Opts.Global.ForceCacheDelete {
			cis := v2alpha1.CopyImageSchema{
				Origin:      img.ImageName,
				Destination: strings.ReplaceAll(img.ImageReference, o.Opts.Global.DeleteDestination, dockerProtocol+o.LocalStorageFQDN),
				Type:        img.Type,
			}
			o.Log.Debug("deleting images local cache %v", cis.Destination)
			collectorSchema.AllImages = append(collectorSchema.AllImages, cis)
		}

		switch {
		case img.Type.IsRelease():
			collectorSchema.TotalReleaseImages += increment
		case img.Type.IsOperator():
			collectorSchema.TotalOperatorImages += increment
		case img.Type.IsAdditionalImage():
			collectorSchema.TotalAdditionalImages += increment
		case img.Type.IsHelmImage():
			collectorSchema.TotalHelmImages += increment
		}
	}

	o.Opts.Stdout = io.Discard
	if !o.Opts.Global.DeleteGenerate && len(o.Opts.Global.DeleteDestination) > 0 {
		if _, err := o.Batch.Worker(context.Background(), collectorSchema, o.Opts); err != nil {
			o.Log.Warn("error during registry deletion: %v", err)
		}
	}

	return nil
}

// ReadDeleteMetaData - read the list of images to delete
// used to verify the delete yaml is well formed as well as being
// the base for both local cache delete and remote registry delete
func (o DeleteImages) ReadDeleteMetaData() (v2alpha1.DeleteImageList, error) {
	o.Log.Info(emoji.Eyes + " Reading delete file...")
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
