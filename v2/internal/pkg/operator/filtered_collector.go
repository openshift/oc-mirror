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
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/otiai10/copy"
)

type FilterCollector struct {
	OperatorCollector
}

// OperatorImageCollector - this looks into the operator index image
// taking into account the mode we are in (mirrorToDisk, diskToMirror)
// the image is downloaded (oci format) and the index.json is inspected
// once unmarshalled, the links to manifests are inspected
func (o *FilterCollector) OperatorImageCollector(ctx context.Context) (v2alpha1.CollectorSchema, error) {

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
			o.Log.Error(collectorPrefix+digestIncorrectMessage, op.Catalog, err.Error())
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
				o.Log.Error(collectorPrefix+digestIncorrectMessage, op.Catalog, err.Error())
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
			o.Log.Error(collectorPrefix+digestIncorrectMessage, op.Catalog, err.Error())
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

		dc, err := o.ctlgHandler.getDeclarativeConfig(filepath.Join(cacheDir, label))
		if err != nil {
			return v2alpha1.CollectorSchema{}, err
		}

		filteredDC, err := filterCatalog(ctx, *dc, op)
		if err != nil {
			return v2alpha1.CollectorSchema{}, err
		}

		filteredDCSavingDir := filepath.Join(o.Opts.Global.WorkingDir, filteredCatalogDir, imageIndexDir)
		err = saveDeclarativeConfig(*filteredDC, filteredDCSavingDir)
		if err != nil {
			return v2alpha1.CollectorSchema{}, err
		}

		if collectorSchema.CatalogToFBCMap == nil {
			collectorSchema.CatalogToFBCMap = make(map[string]v2alpha1.CatalogFilterResult)
		}
		result := v2alpha1.CatalogFilterResult{
			OperatorFilter:     op,
			FilteredConfigPath: filteredDCSavingDir,
		}
		collectorSchema.CatalogToFBCMap[op.Catalog] = result

		ri, err := o.ctlgHandler.getRelatedImagesFromCatalog(filteredDC, copyImageSchemaMap)
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
	if o.Opts.Global.LogLevel == "debug" {
		for _, v := range relatedImages {
			count = count + len(v)
		}
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
