package operator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/otiai10/copy"
)

type LocalStorageCollector struct {
	Log              clog.PluggableLoggerInterface
	LogsDir          string
	Mirror           mirror.MirrorInterface
	Manifest         manifest.ManifestInterface
	Config           v2alpha1.ImageSetConfiguration
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
func (o *LocalStorageCollector) OperatorImageCollector(ctx context.Context) (v2alpha1.CollectorSchema, error) {

	var (
		allImages   []v2alpha1.CopyImageSchema
		label       string
		dir         string
		catalogName string
	)
	o.Log.Debug(collectorPrefix+"setting copy option o.Opts.MultiArch=%s when collecting operator images", o.Opts.MultiArch)

	relatedImages := make(map[string][]v2alpha1.RelatedImage)
	collectorSchema := v2alpha1.CollectorSchema{}
	copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: make(map[string]map[string]struct{}), BundlesByImage: make(map[string]map[string]string)}

	for _, op := range o.Config.Mirror.Operators {
		// download the operator index image
		o.Log.Debug(collectorPrefix+"copying operator image %s", op.Catalog)

		// CLID-47 double check that targetCatalog is valid
		if op.TargetCatalog != "" && !v2alpha1.IsValidPathComponent(op.TargetCatalog) {
			o.Log.Error(collectorPrefix+"invalid targetCatalog %s", op.TargetCatalog)
			return v2alpha1.CollectorSchema{}, fmt.Errorf(collectorPrefix+"invalid targetCatalog %s", op.TargetCatalog)
		}
		// CLID-27 ensure we pick up oci:// (on disk) catalogs
		imgSpec, err := image.ParseRef(op.Catalog)
		if err != nil {
			o.Log.Error(errMsg, err.Error())
			return v2alpha1.CollectorSchema{}, err
		}
		//OCPBUGS-36214: For diskToMirror (and delete), access to the source registry is not guaranteed
		catalogDigest := ""
		if o.Opts.Mode == mirror.DiskToMirror || o.Opts.Mode == string(mirror.DeleteMode) {
			d, err := o.catalogDigest(ctx, op)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
				return v2alpha1.CollectorSchema{}, err
			}
			catalogDigest = d
		} else {
			sourceCtx, err := o.Opts.SrcImage.NewSystemContext()
			if err != nil {
				return v2alpha1.CollectorSchema{}, err
			}
			d, err := o.Manifest.GetDigest(ctx, sourceCtx, imgSpec.ReferenceWithTransport)
			// OCPBUGS-36548 (manifest unknown)
			if err != nil {
				o.Log.Warn(collectorPrefix+"catalog %s : SKIPPING", err.Error())
				continue
			}
			catalogDigest = d
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
					o.Log.Error(errMsg, err.Error())
					return v2alpha1.CollectorSchema{}, err
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
					o.Log.Error(errMsg, err.Error())
					return v2alpha1.CollectorSchema{}, err
				}
				src := dockerProtocol + op.Catalog
				dest := ociProtocolTrimmed + dir

				optsCopy := o.Opts
				optsCopy.Stdout = io.Discard

				err = o.Mirror.Run(ctx, src, dest, "copy", &optsCopy)

				if err != nil {
					o.Log.Error(errMsg, err.Error())
				}
			}
		}

		// it's in oci format so we can go directly to the index.json file
		oci, err := o.Manifest.GetImageIndex(dir)
		if err != nil {
			o.Log.Error(errMsg, err.Error())
			return v2alpha1.CollectorSchema{}, err
		}

		var catalogImage string
		if isMultiManifestIndex(*oci) && imgSpec.Transport == ociProtocol {
			err = o.Manifest.ConvertIndexToSingleManifest(dir, oci)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
				return v2alpha1.CollectorSchema{}, err
			}

			oci, err = o.Manifest.GetImageIndex(dir)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
				return v2alpha1.CollectorSchema{}, err
			}

			catalogImage = ociProtocol + dir
		} else {
			catalogImage = op.Catalog
		}

		if len(oci.Manifests) == 0 {
			o.Log.Error(collectorPrefix+"no manifests found for %s ", op.Catalog)
			return v2alpha1.CollectorSchema{}, fmt.Errorf(collectorPrefix+"no manifests found for %s ", op.Catalog)
		}

		validDigest, err := digest.Parse(oci.Manifests[0].Digest)
		if err != nil {
			o.Log.Error(collectorPrefix+"the digests seem to be incorrect for %s: %s ", op.Catalog, err.Error())
			return v2alpha1.CollectorSchema{}, fmt.Errorf(collectorPrefix+"the digests seem to be incorrect for %s: %s ", op.Catalog, err.Error())
		}

		manifest := validDigest.Encoded()
		o.Log.Debug(collectorPrefix+"manifest %s", manifest)
		// read the operator image manifest
		manifestDir := filepath.Join(dir, blobsDir, manifest)
		oci, err = o.Manifest.GetImageManifest(manifestDir)
		if err != nil {
			o.Log.Error(errMsg, err.Error())
			return v2alpha1.CollectorSchema{}, err
		}

		// we need to check if oci returns multi manifests
		// (from manifest list) also oci.Config will be nil
		// we are only interested in the first manifest as all
		// architecture "configs" will be exactly the same
		if len(oci.Manifests) > 1 && oci.Config.Size == 0 {
			subDigest, err := digest.Parse(oci.Manifests[0].Digest)
			if err != nil {
				o.Log.Error(collectorPrefix+"the digests seem to be incorrect for %s: %s ", op.Catalog, err.Error())
				return v2alpha1.CollectorSchema{}, fmt.Errorf(collectorPrefix+"the digests seem to be incorrect for %s: %s ", op.Catalog, err.Error())
			}
			manifestDir := filepath.Join(dir, blobsDir, subDigest.Encoded())
			oci, err = o.Manifest.GetImageManifest(manifestDir)
			if err != nil {
				o.Log.Error(collectorPrefix+"manifest %s: %s ", op.Catalog, err.Error())
				return v2alpha1.CollectorSchema{}, fmt.Errorf(collectorPrefix+"manifest %s: %s ", op.Catalog, err.Error())
			}
		}

		// read the config digest to get the detailed manifest
		// looking for the lable to search for a specific folder
		configDigest, err := digest.Parse(oci.Config.Digest)
		if err != nil {
			o.Log.Error(collectorPrefix+"the digests seem to be incorrect for %s: %s ", op.Catalog, err.Error())
			return v2alpha1.CollectorSchema{}, fmt.Errorf(collectorPrefix+"the digests seem to be incorrect for %s: %s ", op.Catalog, err.Error())
		}
		catalogDir := filepath.Join(dir, blobsDir, configDigest.Encoded())
		ocs, err := o.Manifest.GetOperatorConfig(catalogDir)
		if err != nil {
			o.Log.Error(errMsg, err.Error())
			return v2alpha1.CollectorSchema{}, err
		}

		label = ocs.Config.Labels.OperatorsOperatorframeworkIoIndexConfigsV1
		o.Log.Debug(collectorPrefix+"label %s", label)

		// untar all the blobs for the operator
		// if the layer with "label (from previous step) is found to a specific folder"
		fromDir := strings.Join([]string{dir, blobsDir}, "/")
		err = o.Manifest.ExtractLayersOCI(fromDir, cacheDir, label, oci)
		if err != nil {
			return v2alpha1.CollectorSchema{}, err
		}

		operatorCatalog, err := o.Manifest.GetCatalog(filepath.Join(cacheDir, label))
		if err != nil {
			return v2alpha1.CollectorSchema{}, err
		}

		ri, err := o.Manifest.GetRelatedImagesFromCatalog(operatorCatalog, op, copyImageSchemaMap)
		if err != nil {
			return v2alpha1.CollectorSchema{}, err
		}

		maps.Copy(relatedImages, ri)

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
			targetTag = "latest"
		}

		if len(op.TargetCatalog) > 0 {
			targetCatalog = op.TargetCatalog

		}

		componentName := imgSpec.ComponentName() + "." + catalogDigest

		relatedImages[componentName] = []v2alpha1.RelatedImage{
			{
				Name:          catalogName,
				Image:         catalogImage,
				Type:          v2alpha1.TypeOperatorCatalog,
				TargetTag:     targetTag,
				TargetCatalog: targetCatalog,
			},
		}
	}

	o.Log.Debug(collectorPrefix+"related images length %d ", len(relatedImages))
	var count = 0
	for _, v := range relatedImages {
		count = count + len(v)
	}
	o.Log.Debug(collectorPrefix+"images to copy (before duplicates) %d ", count)
	var err error
	// check the mode
	if o.Opts.IsMirrorToDisk() || o.Opts.IsMirrorToMirror() {
		allImages, err = o.prepareM2DCopyBatch(relatedImages)
		if err != nil {
			o.Log.Error(errMsg, err.Error())
			return v2alpha1.CollectorSchema{}, err
		}
	}

	if o.Opts.IsDiskToMirror() {
		allImages, err = o.prepareD2MCopyBatch(relatedImages)
		if err != nil {
			o.Log.Error(errMsg, err.Error())
			return v2alpha1.CollectorSchema{}, err
		}
	}

	sort.Sort(ByTypePriority(allImages))

	collectorSchema.AllImages = allImages
	collectorSchema.CopyImageSchemaMap = *copyImageSchemaMap

	return collectorSchema, nil
}

func isMultiManifestIndex(oci v2alpha1.OCISchema) bool {
	return len(oci.Manifests) > 1
}

func (o LocalStorageCollector) catalogDigest(ctx context.Context, catalog v2alpha1.Operator) (string, error) {
	var src string

	srcImgSpec, err := image.ParseRef(catalog.Catalog)
	if err != nil {
		return "", fmt.Errorf("unable to determine cached reference for catalog %s: %v", catalog.Catalog, err)
	}

	// prepare the src and dest references
	switch {
	case len(catalog.TargetCatalog) > 0:
		src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, catalog.TargetCatalog}, "/")
	case srcImgSpec.Transport == ociProtocol:
		src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, path.Base(srcImgSpec.Reference)}, "/")
	default:
		src = dockerProtocol + strings.Join([]string{o.LocalStorageFQDN, srcImgSpec.PathComponent}, "/")
	}

	switch {
	case len(catalog.TargetTag) > 0: // applies only to catalogs
		src = src + ":" + catalog.TargetTag
	case srcImgSpec.Tag == "" && srcImgSpec.Digest != "":
		src = src + ":" + srcImgSpec.Digest
	case srcImgSpec.Tag == "" && srcImgSpec.Digest == "" && srcImgSpec.Transport == ociProtocol:
		src = src + ":latest"
	default:
		src = src + ":" + srcImgSpec.Tag
	}

	imgSpec, err := image.ParseRef(src)
	if err != nil {
		o.Log.Error(errMsg, err.Error())
		return "", err
	}

	sourceCtx, err := o.Opts.SrcImage.NewSystemContext()
	if err != nil {
		return "", err
	}

	catalogDigest, err := o.Manifest.GetDigest(ctx, sourceCtx, imgSpec.ReferenceWithTransport)
	if err != nil {
		o.Log.Error(errMsg, err.Error())
		return "", err
	}
	return catalogDigest, nil
}

func (o LocalStorageCollector) prepareD2MCopyBatch(images map[string][]v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	var result []v2alpha1.CopyImageSchema
	for _, relatedImgs := range images {
		for _, img := range relatedImgs {
			var src string
			var dest string
			if img.Image == "" { // OCPBUGS-31622 skipping empty related images
				continue
			}
			imgSpec, err := image.ParseRef(img.Image)
			if err != nil {
				// OCPBUGS-33081 - skip if parse error (i.e semver and other)
				o.Log.Warn("mirroring skipped : %v", err)
				continue
			}

			// prepare the src and dest references
			switch {
			case img.Type == v2alpha1.TypeOperatorCatalog && len(img.TargetCatalog) > 0: // applies only to catalogs
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
			case img.Type == v2alpha1.TypeOperatorCatalog && len(img.TargetTag) > 0: // applies only to catalogs
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
			result = append(result, v2alpha1.CopyImageSchema{Origin: imgSpec.ReferenceWithTransport, Source: src, Destination: dest, Type: img.Type})
		}
	}
	return result, nil
}

func (o LocalStorageCollector) prepareM2DCopyBatch(images map[string][]v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	var result []v2alpha1.CopyImageSchema
	for _, relatedImgs := range images {
		for _, img := range relatedImgs {
			var src string
			var dest string
			if img.Image == "" { // OCPBUGS-31622 skipping empty related images
				continue
			}
			imgSpec, err := image.ParseRef(img.Image)
			if err != nil {
				// OCPBUGS-33081 - skip if parse error (i.e semver and other)
				o.Log.Warn("%v : SKIPPING", err)
				continue
			}

			src = imgSpec.ReferenceWithTransport
			if img.Type == v2alpha1.TypeOperatorCatalog && len(img.TargetCatalog) > 0 {
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), img.TargetCatalog}, "/")
			} else if imgSpec.Transport == ociProtocol {
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), img.Name}, "/")
			} else {
				dest = dockerProtocol + strings.Join([]string{o.destinationRegistry(), imgSpec.PathComponent}, "/")
			}
			if img.Type == v2alpha1.TypeOperatorCatalog && len(img.TargetTag) > 0 {
				dest = dest + ":" + img.TargetTag
			} else if imgSpec.Tag == "" && imgSpec.Transport == ociProtocol {
				dest = dest + ":latest"
			} else if imgSpec.Tag == "" && imgSpec.Digest != "" {
				dest = dest + ":" + imgSpec.Digest
			} else {
				dest = dest + ":" + imgSpec.Tag
			}

			o.Log.Debug("source %s", src)
			o.Log.Debug("destination %s", dest)

			// OCPBUGS-33196 - check source image for tag and digest
			// skip mirroring
			if imgSpec.IsImageByTagAndDigest() {
				o.Log.Warn(collectorPrefix+"%s has both tag and digest : SKIPPING", imgSpec.Reference)
			} else {
				result = append(result, v2alpha1.CopyImageSchema{Source: src, Destination: dest, Origin: src, Type: img.Type})
			}

		}
	}
	return result, nil
}
