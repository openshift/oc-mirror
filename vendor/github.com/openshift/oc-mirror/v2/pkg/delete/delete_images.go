package delete

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/archive"
	"github.com/openshift/oc-mirror/v2/pkg/batch"
	"github.com/openshift/oc-mirror/v2/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type DeleteImages struct {
	Log              clog.PluggableLoggerInterface
	Opts             mirror.CopyOptions
	Batch            batch.BatchInterface
	Blobs            archive.BlobsGatherer
	Config           v1alpha2.ImageSetConfiguration
	Manifest         manifest.ManifestInterface
	LocalStorageDisk string
	LocalStorageFQDN string
}

// DeleteCacheBlobs - does what it says ;)
// for now we only report errors, it is not critical if the delete cache fails
// the cache can always be restored (refer to docs/design/v2/delete-functionality.md)
func (o DeleteImages) DeleteCacheBlobs(ctx context.Context, images []v1alpha3.CopyImageSchema) error {
	o.Log.Info("deleting images from local cache")
	// we write the image and related blobs in yaml format to file for further processing
	filename := filepath.Join(o.Opts.Global.WorkingDir, deleteImagesYaml, "/")
	discYamlFile := filepath.Join(o.Opts.Global.WorkingDir, discYaml, "/")
	if len(o.Opts.Global.DeleteID) > 0 {
		filename = filepath.Join(o.Opts.Global.WorkingDir, strings.ReplaceAll(deleteImagesYaml, ".", "-"+o.Opts.Global.DeleteID+"."), "/")
		discYamlFile = filepath.Join(o.Opts.Global.WorkingDir, strings.ReplaceAll(discYaml, ".", "-"+o.Opts.Global.DeleteID+"."), "/")
	}
	// create the delete folder
	err := os.MkdirAll(o.Opts.Global.WorkingDir+deleteDir, 0755)
	if err != nil {
		o.Log.Error("%v ", err)
	}
	var items []v1alpha3.DeleteItems
	// gather related blobs
	blobPath := filepath.Join(o.LocalStorageDisk, blobsDir, "/")
	for _, img := range images {
		copyIS, err := buildFormatedCopyImageSchema(img.Origin, img.Destination, o.LocalStorageFQDN)
		if err != nil {
			o.Log.Error("%v ", err)
		}
		// clean up the destination url
		// for our output yaml
		name := strings.Split(copyIS.Destination, o.LocalStorageFQDN)
		if len(name) > 0 {
			copyIS.Destination = name[1][1:]
		}
		item := v1alpha3.DeleteItems{
			ImageName:      copyIS.Origin,
			ImageReference: copyIS.Destination,
		}
		if err != nil {
			o.Log.Error("%v ", err)
		}
		i, err := o.Blobs.GatherBlobs(ctx, img.Destination)
		if err != nil {
			o.Log.Error("%v image : %s", err, i)
		}
		// physically delete blobs
		if err != nil {
			o.Log.Error(deleteImagesErrMsg, err)
		}
		var blobs []string
		for k := range i {
			//write related blob to file
			blobs = append(blobs, k)
			if err != nil {
				o.Log.Error("unable to write blob %s %v", k, err)
			}
			if !o.Opts.Global.DryRun && !o.Opts.Global.SkipCacheDelete {
				blob := strings.Split(k, "sha256:")
				if len(blob) > 1 {
					blobFile := filepath.Join(blobPath, blob[1][0:2], blob[1], "/")
					err := os.RemoveAll(blobFile)
					if err != nil {
						o.Log.Error("unable to delete blob %s %v", blobFile, err)
					}
					o.Log.Debug("blob %s", blobFile)
				} else {
					o.Log.Warn("blob format seems to be incorrect %s", k)
				}
			}
		}
		item.RelatedBlobs = blobs
		items = append(items, item)
	}
	// marshal to yaml and write to file
	deleteImageList := v1alpha3.DeleteImageList{
		Kind:       "DeleteImageList",
		APIVersion: "mirror.openshift.io/v1alpha2",
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
	disc := v1alpha2.DeleteImageSetConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DeleteImageSetConfiguration",
			APIVersion: "mirror.openshift.io/v1alpha2",
		},
		DeleteImageSetConfigurationSpec: v1alpha2.DeleteImageSetConfigurationSpec{
			Delete: v1alpha2.Delete{
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

// DeleteRegistryImages - does what it says ;)
func (o DeleteImages) DeleteRegistryImages(ctx context.Context, imgs []v1alpha3.CopyImageSchema) error {
	o.Log.Info("deleting images from remote registry")
	var updatedImages []v1alpha3.CopyImageSchema

	for _, img := range imgs {
		// check for LocalStorageFQDN and remove it
		if strings.Contains(img.Destination, o.LocalStorageFQDN) {
			img.Destination = strings.Split(img.Destination, o.LocalStorageFQDN)[1][1:]
		}
		// prefix the destination registry
		img.Destination = strings.Join([]string{o.Opts.Global.DeleteDestination, img.Destination}, "/")
		cis := v1alpha3.CopyImageSchema{
			Source:      img.Source,
			Origin:      img.Origin,
			Destination: img.Destination,
		}
		o.Log.Debug("deleting images %v", cis.Destination)
		updatedImages = append(updatedImages, cis)
	}
	if !o.Opts.Global.DryRun && !o.Opts.Global.SkipRegistryDelete {
		err := o.Batch.Worker(ctx, updatedImages, o.Opts)
		if err != nil {
			return err
		}
	}
	return nil
}

// ReadDeleteMetaData - read the list of images to delete
// used to verify the delete yaml is well formed
func (o DeleteImages) ReadDeleteMetaData() ([]v1alpha3.CopyImageSchema, error) {
	var images []v1alpha3.CopyImageSchema
	var list v1alpha3.DeleteImageList

	filename := filepath.Join(o.Opts.Global.WorkingDir, deleteImagesYaml, "/")
	if len(o.Opts.Global.DeleteID) > 0 {
		filename = filepath.Join(o.Opts.Global.WorkingDir, strings.ReplaceAll(deleteImagesYaml, ".", "-"+o.Opts.Global.DeleteID+"."), "/")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	// lets parse the file to get the images
	err = yaml.Unmarshal(data, &list)
	if err != nil {
		return nil, err
	}
	for _, img := range list.Items {
		images = append(images, v1alpha3.CopyImageSchema{Destination: strings.TrimSpace(img.ImageReference)})
	}
	return images, nil
}

// CollectReleaseImages
func (o DeleteImages) CollectReleaseImages(releaseFolder string) ([]v1alpha3.CopyImageSchema, error) {
	var rs v1alpha3.ReleaseSchema
	releaseJson := filepath.Join(releaseFolder, releaseManifests, imageReferences, "/")
	data, err := os.ReadFile(releaseJson)
	if err != nil {
		return []v1alpha3.CopyImageSchema{}, err
	}
	err = json.Unmarshal(data, &rs)
	if err != nil {
		return []v1alpha3.CopyImageSchema{}, err
	}

	// collect all release images and add them to CopyImageSchema collection
	var allImages []v1alpha3.CopyImageSchema
	for _, img := range rs.Spec.Tags {
		copyIS, err := buildFormatedCopyImageSchema(img.Name, img.From.Name, o.LocalStorageFQDN)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}
		allImages = append(allImages, copyIS)
	}
	return allImages, nil
}

// CollectOperatorImages
func (o DeleteImages) CollectOperatorImages() ([]v1alpha3.CopyImageSchema, error) {
	var allImages []v1alpha3.CopyImageSchema
	for _, op := range o.Config.Mirror.Operators {
		imageIndexDir := filepath.Base(op.Catalog)
		cacheDir := strings.Join([]string{o.Opts.Global.WorkingDir, operatorImageExtractDir, strings.ReplaceAll(imageIndexDir, ":", "/")}, "/")
		// we dont know the subfolder name so lets get it
		dir, err := os.ReadDir(cacheDir)
		if err != nil {
			return nil, err
		}
		if len(dir) > 0 {
			// if there is more than one dir there could be a problem
			// for now we select the first one
			o.Log.Debug("label (directory) %s", dir[0].Name())
			operatorCatalog, err := o.Manifest.GetCatalog(filepath.Join(cacheDir, dir[0].Name()))
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, err
			}
			ri, err := o.Manifest.GetRelatedImagesFromCatalog(operatorCatalog, op)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, err
			}
			// collect all operator images and add them to CopyImageSchema collection
			for _, v := range ri {
				for _, i := range v {
					copyIS, err := buildFormatedCopyImageSchema(i.Name, i.Image, o.LocalStorageFQDN)
					if err != nil {
						o.Log.Error(deleteImagesErrMsg, err)
					}
					allImages = append(allImages, copyIS)
				}
			}
		} else {
			return nil, fmt.Errorf("no (label) directory found in %s", cacheDir)
		}
	}
	return allImages, nil
}

// CollectAdditionalImages
func (o DeleteImages) CollectAdditionalImages() ([]v1alpha3.CopyImageSchema, error) {
	var allImages []v1alpha3.CopyImageSchema
	for _, ai := range o.Config.Mirror.AdditionalImages {
		is, err := buildFormatedCopyImageSchema(ai.Name, ai.Name, o.LocalStorageFQDN)
		if err != nil {
			o.Log.Error(deleteImagesErrMsg, err)
		}

		allImages = append(allImages, is)
	}
	return allImages, nil
}

// buildFormatedCopyImageSchema - simple private utility to build the CopyImageSchema data
func buildFormatedCopyImageSchema(name, img, localStorageFQDN string) (v1alpha3.CopyImageSchema, error) {
	var dest string
	imgSpec, err := image.ParseRef(img)
	if err != nil {
		return v1alpha3.CopyImageSchema{}, err
	}
	if imgSpec.IsImageByDigest() {
		dest = dockerProtocol + strings.Join([]string{localStorageFQDN, imgSpec.PathComponent + "@sha256:" + imgSpec.Digest}, "/")
	} else {
		dest = dockerProtocol + strings.Join([]string{localStorageFQDN, imgSpec.PathComponent + ":" + imgSpec.Tag}, "/")
	}
	if len(name) == 0 {
		name = imgSpec.Name
	}
	is := v1alpha3.CopyImageSchema{
		Source:      imgSpec.Name,
		Destination: dest,
		Origin:      name,
	}
	return is, nil
}
