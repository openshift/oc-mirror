package operator

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

const (
	hashTruncLen int = 12
)

type LocalStorageCollector struct {
	Log              clog.PluggableLoggerInterface
	Mirror           mirror.MirrorInterface
	Manifest         manifest.ManifestInterface
	Config           v1alpha2.ImageSetConfiguration
	Opts             mirror.CopyOptions
	LocalStorageFQDN string
}

// OperatorImageCollector - this looks into the operator index image
// taking into account the mode we are in (mirrorToDisk, diskToMirror)
// the image is downloaded (oci format) and the index.json is inspected
// once unmarshalled, the links to manifests are inspected
func (o *LocalStorageCollector) OperatorImageCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error) {

	var (
		allImages []v1alpha3.CopyImageSchema
		label     string
		dir       string
	)
	compare := make(map[string]v1alpha3.ISCPackage)
	relatedImages := make(map[string][]v1alpha3.RelatedImage)

	// compile a map to compare channels,min & max versions
	for _, ops := range o.Config.Mirror.Operators {
		o.Log.Info("isc operators: %s\n", ops.Catalog)
		for _, pkg := range ops.Packages {
			o.Log.Info("catalog packages: %s \n", pkg.Name)
			for _, channel := range pkg.Channels {
				compare[pkg.Name] = v1alpha3.ISCPackage{Channel: channel.Name, MinVersion: channel.MinVersion, MaxVersion: channel.MaxVersion, Full: ops.Full}
				o.Log.Info("channels: %v \n", compare)
			}
		}
	}
	f, err := os.Create(logsFile)
	if err != nil {
		o.Log.Error(errMsg, err)
	}
	writer := bufio.NewWriter(f)
	defer f.Close()
	for _, op := range o.Config.Mirror.Operators {
		// download the operator index image
		o.Log.Info("copying operator image %v", op.Catalog)
		hld := strings.Split(op.Catalog, "/")
		imageIndexDir := strings.Replace(hld[len(hld)-1], ":", "/", -1)
		cacheDir := strings.Join([]string{o.Opts.Global.Dir, operatorImageExtractDir, imageIndexDir}, "/")
		dir = strings.Join([]string{o.Opts.Global.Dir, operatorImageDir, imageIndexDir}, "/")
		if _, err := os.Stat(cacheDir); errors.Is(err, os.ErrNotExist) {
			err := os.MkdirAll(dir, 0755)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, err
			}
			src := dockerProtocol + op.Catalog
			dest := ociProtocolTrimmed + dir
			err = o.Mirror.Run(ctx, src, dest, "copy", &o.Opts, *writer)
			writer.Flush()
			if err != nil {
				o.Log.Error(errMsg, err)
			}
			// read the logs
			f, _ := os.ReadFile(logsFile)
			lines := strings.Split(string(f), "\n")
			for _, s := range lines {
				if len(s) > 0 {
					o.Log.Debug("%s ", strings.ToLower(s))
				}
			}
		}

		// it's in oci format so we can go directly to the index.json file
		oci, err := o.Manifest.GetImageIndex(dir)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}

		//read the link to the manifest
		if len(oci.Manifests) == 0 {
			return []v1alpha3.CopyImageSchema{}, fmt.Errorf("[OperatorImageCollector] no manifests found for %s ", op.Catalog)
		} else {
			if !strings.Contains(oci.Manifests[0].Digest, "sha256") {
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf("[OperatorImageCollector] the disgets seems to incorrect for %s ", op.Catalog)
			}
		}
		manifest := strings.Split(oci.Manifests[0].Digest, ":")[1]
		o.Log.Info("manifest %v", manifest)

		// read the operator image manifest
		manifestDir := strings.Join([]string{dir, blobsDir, manifest}, "/")
		oci, err = o.Manifest.GetImageManifest(manifestDir)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}

		// read the config digest to get the detailed manifest
		// looking for the lable to search for a specific folder
		catalogDir := strings.Join([]string{dir, blobsDir, strings.Split(oci.Config.Digest, ":")[1]}, "/")
		ocs, err := o.Manifest.GetOperatorConfig(catalogDir)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}

		label = ocs.Config.Labels.OperatorsOperatorframeworkIoIndexConfigsV1
		o.Log.Info("label %s", label)

		// untar all the blobs for the operator
		// if the layer with "label (from previous step) is found to a specific folder"
		fromDir := strings.Join([]string{dir, blobsDir}, "/")
		err = o.Manifest.ExtractLayersOCI(fromDir, cacheDir, label, oci)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}

		// select all packages
		// this is the equivalent of the headOnly mode
		// only the latest version of each operator will be selected
		if len(op.Packages) == 0 {
			relatedImages, err = o.Manifest.GetRelatedImagesFromCatalog(cacheDir, label)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, err
			}
		} else {
			// iterate through each package
			relatedImages, err = o.Manifest.GetRelatedImagesFromCatalogByFilter(cacheDir, label, op, compare)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, err
			}
		}
		relatedImages["index"] = []v1alpha3.RelatedImage{
			{
				Name:  "index",
				Image: op.Catalog,
			},
		}
	}

	o.Log.Info("related images length %d ", len(relatedImages))
	var count = 0
	for _, v := range relatedImages {
		count = count + len(v)
	}
	o.Log.Info("images to copy (before duplicates) %d ", count)
	// check the mode
	if o.Opts.Mode == mirrorToDisk {

		allImages, err = o.prepareM2DCopyBatch(o.Log, dir, relatedImages)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}
	}

	if o.Opts.Mode == diskToMirror {
		allImages, err = o.prepareD2MCopyBatch(o.Log, dir, relatedImages)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}
	}
	return allImages, nil
}

func (o *LocalStorageCollector) prepareD2MCopyBatch(log clog.PluggableLoggerInterface, dir string, images map[string][]v1alpha3.RelatedImage) ([]v1alpha3.CopyImageSchema, error) {
	var result []v1alpha3.CopyImageSchema
	for _, relatedImgs := range images {
		for _, img := range relatedImgs {
			var src string
			var dest string

			imgRef := img.Image
			transportAndRef := strings.Split(imgRef, "://")
			if len(transportAndRef) > 1 {
				imgRef = transportAndRef[1]
			}

			pathWithoutDNS, err := pathWithoutDNS(imgRef)
			if err != nil {
				o.Log.Error("%s", err.Error())
				return nil, err
			}

			if isImageByDigest(imgRef) {
				src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathWithoutDNS + ":" + imageHash(imgRef)[:hashTruncLen]}, "/")
				dest = strings.Join([]string{o.Opts.Destination, pathWithoutDNS + ":" + imageHash(imgRef)[:hashTruncLen]}, "/")
			} else {
				src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathWithoutDNS}, "/")
				dest = strings.Join([]string{o.Opts.Destination, pathWithoutDNS}, "/")
			}

			if src == "" || dest == "" {
				return result, fmt.Errorf("unable to determine src %s or dst %s for %s", src, dest, img.Name)
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dest)
			result = append(result, v1alpha3.CopyImageSchema{Origin: img.Image, Source: src, Destination: dest})
		}
	}
	return result, nil
}

func (o *LocalStorageCollector) prepareM2DCopyBatch(log clog.PluggableLoggerInterface, dir string, images map[string][]v1alpha3.RelatedImage) ([]v1alpha3.CopyImageSchema, error) {
	var result []v1alpha3.CopyImageSchema
	for _, relatedImgs := range images {
		for _, img := range relatedImgs {
			imgRef := img.Image
			var src string
			var dest string
			if !strings.Contains(imgRef, "://") {
				src = dockerProtocol + imgRef
			} else {
				src = imgRef
				transportAndRef := strings.Split(imgRef, "://")
				imgRef = transportAndRef[1]
			}

			pathWithoutDNS, err := pathWithoutDNS(imgRef)
			if err != nil {
				o.Log.Error("%s", err.Error())
				return nil, err
			}

			if isImageByDigest(imgRef) {
				dest = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathWithoutDNS + ":" + imageHash(imgRef)[:hashTruncLen]}, "/")
			} else {
				dest = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, pathWithoutDNS}, "/")
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dest)
			result = append(result, v1alpha3.CopyImageSchema{Source: src, Destination: dest})

		}
	}
	return result, nil
}

func isImageByDigest(imgRef string) bool {
	return strings.Contains(imgRef, "@")
}

func pathWithoutDNS(imgRef string) (string, error) {

	var imageName []string
	if isImageByDigest(imgRef) {
		imageNameSplit := strings.Split(imgRef, "@")
		imageName = strings.Split(imageNameSplit[0], "/")
	} else {
		imageName = strings.Split(imgRef, "/")
	}

	if len(imageName) > 2 {
		return strings.Join(imageName[1:], "/"), nil
	} else if len(imageName) == 1 {
		return imageName[0], nil
	} else {
		return "", fmt.Errorf("unable to parse image %s correctly", imgRef)
	}
}

func imageHash(imgRef string) string {
	var hash string
	imgSplit := strings.Split(imgRef, "@")
	if len(imgSplit) > 1 {
		hash = strings.Split(imgSplit[1], ":")[1]
	}

	return hash
}
