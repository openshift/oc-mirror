package operator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/otiai10/copy"
)

type LocalStorageCollector struct {
	Log              clog.PluggableLoggerInterface
	LogsDir          string
	Mirror           mirror.MirrorInterface
	Manifest         manifest.ManifestInterface
	Config           v1alpha2.ImageSetConfiguration
	Opts             mirror.CopyOptions
	LocalStorageFQDN string
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

// OperatorImageCollector - this looks into the operator index image
// taking into account the mode we are in (mirrorToDisk, diskToMirror)
// the image is downloaded (oci format) and the index.json is inspected
// once unmarshalled, the links to manifests are inspected
func (o *LocalStorageCollector) OperatorImageCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error) {

	var (
		allImages   []v1alpha3.CopyImageSchema
		label       string
		dir         string
		catalogName string
	)
	o.Log.Debug("multiArch=%v for operator collections", o.Opts.MultiArch)

	relatedImages := make(map[string][]v1alpha3.RelatedImage)

	for _, op := range o.Config.Mirror.Operators {
		// download the operator index image
		o.Log.Info("copying operator image %v", op.Catalog)

		// CLID-47 double check that targetCatalog is valid
		if op.TargetCatalog != "" && !v1alpha2.IsValidPathComponent(op.TargetCatalog) {
			return []v1alpha3.CopyImageSchema{}, fmt.Errorf("invalid targetCatalog %s", op.TargetCatalog)
		}
		// CLID-27 ensure we pick up oci:// (on disk) catalogs
		imgSpec, err := image.ParseRef(op.Catalog)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}

		sourceCtx, err := o.Opts.SrcImage.NewSystemContext()
		if err != nil {
			return nil, err
		}

		catalogDigest, err := o.Manifest.GetDigest(ctx, sourceCtx, imgSpec.ReferenceWithTransport)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}

		imageIndexDir := filepath.Join(imgSpec.ComponentName(), catalogDigest)
		cacheDir := filepath.Join(o.Opts.Global.WorkingDir, operatorImageExtractDir, imageIndexDir)
		dir = filepath.Join(o.Opts.Global.WorkingDir, operatorImageDir, imageIndexDir)

		if imgSpec.Transport == ociProtocol {
			if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
				// delete the existing directory and untarred cache contents
				os.RemoveAll(dir)
				os.RemoveAll(cacheDir)
				// copy all contents to the working dir
				err := copy.Copy(imgSpec.PathComponent, dir)
				if err != nil {
					return []v1alpha3.CopyImageSchema{}, err
				}
			}

			if len(op.TargetCatalog) > 0 {
				catalogName = op.TargetCatalog
			} else {
				catalogName = path.Base(imgSpec.Reference)
			}
		} else {
			if _, err := os.Stat(cacheDir); errors.Is(err, os.ErrNotExist) {
				err := os.MkdirAll(dir, 0755)
				if err != nil {
					return []v1alpha3.CopyImageSchema{}, err
				}
				src := dockerProtocol + op.Catalog
				dest := ociProtocolTrimmed + dir
				err = o.Mirror.Run(ctx, src, dest, "copy", &o.Opts)
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
		}

		// it's in oci format so we can go directly to the index.json file
		oci, err := o.Manifest.GetImageIndex(dir)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}

		var catalogImage string
		if isMultiManifestIndex(*oci) && imgSpec.Transport == ociProtocol {
			err = o.Manifest.ConvertIndexToSingleManifest(dir, oci)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, err
			}

			oci, err = o.Manifest.GetImageIndex(dir)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, err
			}

			catalogImage = ociProtocol + dir
		} else {
			catalogImage = op.Catalog
		}

		if len(oci.Manifests) == 0 {
			return []v1alpha3.CopyImageSchema{}, fmt.Errorf("[OperatorImageCollector] no manifests found for %s ", op.Catalog)
		}

		validDigest, err := digest.Parse(oci.Manifests[0].Digest)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, fmt.Errorf("[OperatorImageCollector] the digests seem to be incorrect for %s: %v ", op.Catalog, err)
		}

		manifest := validDigest.Encoded()
		o.Log.Info("manifest %v", manifest)
		// read the operator image manifest
		manifestDir := filepath.Join(dir, blobsDir, manifest)
		oci, err = o.Manifest.GetImageManifest(manifestDir)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}

		// we need to check if oci returns multi manifests
		// (from manifest list) also oci.Config will be nil
		// we are only interested in the first manifest as all
		// architecture "configs" will be exactly the same
		if len(oci.Manifests) > 1 && oci.Config.Size == 0 {
			subDigest, err := digest.Parse(oci.Manifests[0].Digest)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf("[OperatorImageCollector] the digests seem to be incorrect for %s: %v ", op.Catalog, err)
			}
			manifestDir := filepath.Join(dir, blobsDir, subDigest.Encoded())
			oci, err = o.Manifest.GetImageManifest(manifestDir)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf("[OperatorImageCollector] manifest %s: %v ", op.Catalog, err)
			}
		}

		// read the config digest to get the detailed manifest
		// looking for the lable to search for a specific folder
		configDigest, err := digest.Parse(oci.Config.Digest)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, fmt.Errorf("[OperatorImageCollector] the digests seem to be incorrect for %s: %v ", op.Catalog, err)
		}
		catalogDir := filepath.Join(dir, blobsDir, configDigest.Encoded())
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

		operatorCatalog, err := o.Manifest.GetCatalog(filepath.Join(cacheDir, label))
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}

		ri, err := o.Manifest.GetRelatedImagesFromCatalog(operatorCatalog, op)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}
		for k, v := range ri {
			relatedImages[k] = v
		}

		var targetTag string
		var targetCatalog string
		if len(op.TargetTag) > 0 {
			targetTag = op.TargetTag
		} else if imgSpec.Transport == ociProtocol {
			// for this case only, img.ParseRef(in its current state)
			// will not be able to determine the digest.
			// this leaves the oci imgSpec with no tag nor digest as it
			// goes to prepareM2DCopyBatch/prepareD2MCopyBath. This is
			// why we set the digest read from manifest in targetTag
			targetTag = validDigest.Encoded()
		}

		if len(op.TargetCatalog) > 0 {
			targetCatalog = op.TargetCatalog

		}

		componentName := imgSpec.ComponentName() + "." + catalogDigest

		relatedImages[componentName] = []v1alpha3.RelatedImage{
			{
				Name:          catalogName,
				Image:         catalogImage,
				Type:          v1alpha2.TypeOperatorCatalog,
				TargetTag:     targetTag,
				TargetCatalog: targetCatalog,
			},
		}
	}

	o.Log.Info("related images length %d ", len(relatedImages))
	var count = 0
	for _, v := range relatedImages {
		count = count + len(v)
	}
	o.Log.Info("images to copy (before duplicates) %d ", count)
	var err error
	// check the mode
	if o.Opts.IsMirrorToDisk() || o.Opts.IsMirrorToMirror() || o.Opts.IsPrepare() {
		allImages, err = o.prepareM2DCopyBatch(o.Log, dir, relatedImages)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}
	}

	if o.Opts.IsDiskToMirror() {
		allImages, err = o.prepareD2MCopyBatch(o.Log, dir, relatedImages)
		if err != nil {
			return []v1alpha3.CopyImageSchema{}, err
		}
	}
	return allImages, nil
}

func isMultiManifestIndex(oci v1alpha3.OCISchema) bool {
	return len(oci.Manifests) > 1
}

func (o LocalStorageCollector) prepareD2MCopyBatch(log clog.PluggableLoggerInterface, dir string, images map[string][]v1alpha3.RelatedImage) ([]v1alpha3.CopyImageSchema, error) {
	var result []v1alpha3.CopyImageSchema
	for _, relatedImgs := range images {
		for _, img := range relatedImgs {
			var src string
			var dest string

			imgSpec, err := image.ParseRef(img.Image)
			if err != nil {
				o.Log.Error("%s", err.Error())
				return nil, err
			}

			// prepare the src and dest references
			switch {
			case img.Type == v1alpha2.TypeOperatorCatalog && len(img.TargetCatalog) > 0: // applies only to catalogs
				src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, img.TargetCatalog}, "/")
				dest = strings.Join([]string{o.Opts.Destination, img.TargetCatalog}, "/")
			case imgSpec.Transport == ociProtocol:
				src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, img.Name}, "/")
				dest = strings.Join([]string{o.Opts.Destination, img.Name}, "/")
			default:
				src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, imgSpec.PathComponent}, "/")
				dest = strings.Join([]string{o.Opts.Destination, imgSpec.PathComponent}, "/")
			}

			// add the tag for src and dest
			switch {
			case img.Type == v1alpha2.TypeOperatorCatalog && len(img.TargetTag) > 0: // applies only to catalogs
				src = src + ":" + img.TargetTag
				dest = dest + ":" + img.TargetTag
			case imgSpec.Tag == "":
				src = src + ":" + imgSpec.Digest
				dest = dest + ":" + imgSpec.Digest
			default:
				src = src + ":" + imgSpec.Tag
				dest = dest + ":" + imgSpec.Tag
			}
			if src == "" || dest == "" {
				return result, fmt.Errorf("unable to determine src %s or dst %s for %s", src, dest, img.Image)
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dest)
			result = append(result, v1alpha3.CopyImageSchema{Origin: imgSpec.ReferenceWithTransport, Source: src, Destination: dest, Type: img.Type})
		}
	}
	return result, nil
}

func (o LocalStorageCollector) prepareM2DCopyBatch(log clog.PluggableLoggerInterface, dir string, images map[string][]v1alpha3.RelatedImage) ([]v1alpha3.CopyImageSchema, error) {
	var result []v1alpha3.CopyImageSchema
	for _, relatedImgs := range images {
		for _, img := range relatedImgs {
			var src string
			var dest string
			imgSpec, err := image.ParseRef(img.Image)
			if err != nil {
				return nil, err
			}

			src = imgSpec.ReferenceWithTransport
			if img.Type == v1alpha2.TypeOperatorCatalog && len(img.TargetCatalog) > 0 {
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), img.TargetCatalog}, "/")
			} else if imgSpec.Transport == ociProtocol {
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), img.Name}, "/")
			} else {
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), imgSpec.PathComponent}, "/")
			}
			if img.Type == v1alpha2.TypeOperatorCatalog && len(img.TargetTag) > 0 {
				dest = dest + ":" + img.TargetTag
			} else if imgSpec.Tag == "" {
				dest = dest + ":" + imgSpec.Digest
			} else {
				dest = dest + ":" + imgSpec.Tag
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dest)
			result = append(result, v1alpha3.CopyImageSchema{Source: src, Destination: dest, Origin: src, Type: img.Type})

		}
	}
	return result, nil
}
