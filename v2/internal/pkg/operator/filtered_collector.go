package operator

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/types"
	"github.com/otiai10/copy"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/emoji"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/spinners"
)

type FilterCollector struct {
	OperatorCollector
}

// OperatorImageCollector - this looks into the operator index image
// taking into account the mode we are in (mirrorToDisk, diskToMirror)
// the image is downloaded (oci format) and the index.json is inspected
// once unmarshalled, the links to manifests are inspected
func (o *FilterCollector) OperatorImageCollector(ctx context.Context) (v2alpha1.CollectorSchema, error) {
	o.Log.Debug(collectorPrefix+"setting copy option o.Opts.MultiArch=%s when collecting operator images", o.Opts.MultiArch)

	relatedImages := make(map[string][]v2alpha1.RelatedImage)
	collectorSchema := v2alpha1.CollectorSchema{
		CatalogToFBCMap: make(map[string]v2alpha1.CatalogFilterResult),
	}
	copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{
		OperatorsByImage: make(map[string]map[string]struct{}),
		BundlesByImage:   make(map[string]map[string]string),
	}

	// We are going to try to collect all operators before returning.
	// This slice holds the errors found for each operator plus any error
	// found during the preparation of the images.
	allErrs := []error{}

	p := mpb.New(mpb.PopCompletedMode(), mpb.ContainerOptional(mpb.WithOutput(io.Discard), !o.Opts.Global.IsTerminal))
	for _, op := range o.Config.Mirror.Operators {
		// download the operator index image
		o.Log.Debug(collectorPrefix+"copying operator image %s", op.Catalog)

		if !o.Opts.Global.IsTerminal {
			o.Log.Info("Collecting catalog %s", op.Catalog)
		}
		// prepare spinner
		spinner := p.AddSpinner(
			1, mpb.BarFillerMiddleware(spinners.PositionSpinnerLeft),
			mpb.BarWidth(3),
			mpb.PrependDecorators(
				decor.OnComplete(spinners.EmptyDecorator(), emoji.SpinnerCheckMark),
				decor.OnAbort(spinners.EmptyDecorator(), emoji.SpinnerCrossMark),
			),
			mpb.AppendDecorators(
				decor.Name("("),
				decor.Elapsed(decor.ET_STYLE_GO),
				decor.Name(") Collecting catalog "+op.Catalog+" "),
			),
			mpb.BarFillerClearOnComplete(),
			spinners.BarFillerClearOnAbort(),
		)

		// CLID-27 ensure we pick up oci:// (on disk) catalogs
		imgSpec, err := image.ParseRef(op.Catalog)
		if err != nil {
			spinner.Abort(true)
			spinner.Wait()
			allErrs = append(allErrs, fmt.Errorf("parse catalog %q: %w", op.Catalog, err))
			continue
		}

		result, err := o.collectOperator(ctx, op, relatedImages, copyImageSchemaMap)
		if err != nil {
			spinner.Abort(true)
			spinner.Wait()
			allErrs = append(allErrs, fmt.Errorf("collect catalog %q: %w", op.Catalog, err))
			continue
		}
		collectorSchema.CatalogToFBCMap[imgSpec.ReferenceWithTransport] = result

		spinner.Increment()
		if !o.Opts.Global.IsTerminal {
			o.Log.Info("Collected catalog %s", op.Catalog)
		}
	}
	p.Wait()

	o.Log.Debug(collectorPrefix+"related images length %d ", len(relatedImages))
	count := 0
	for _, v := range relatedImages {
		count += len(v)
	}
	o.Log.Debug(collectorPrefix+"images to copy (before duplicates) %d ", count)

	var err error
	var allImages []v2alpha1.CopyImageSchema
	// check the mode
	switch {
	case o.Opts.IsMirrorToDisk():
		allImages, err = o.prepareM2DCopyBatch(relatedImages)
	case o.Opts.IsMirrorToMirror():
		allImages, err = o.dispatchImagesForM2M(relatedImages)
	case o.Opts.IsDiskToMirror() || o.Opts.Mode == string(mirror.DeleteMode):
		allImages, err = o.prepareD2MCopyBatch(relatedImages)
	}
	if err != nil {
		allErrs = append(allErrs, err)
	}

	collectorSchema.AllImages = allImages
	collectorSchema.CopyImageSchemaMap = *copyImageSchemaMap

	return collectorSchema, errors.Join(allErrs...)
}

func isFullCatalog(catalog v2alpha1.Operator) bool {
	return len(catalog.IncludeConfig.Packages) == 0 && catalog.Full
}

func createFolders(paths []string) error {
	var errs []error
	for _, path := range paths {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			err = os.MkdirAll(path, 0755)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func digestOfFilter(catalog v2alpha1.Operator) (string, error) {
	c := catalog
	c.TargetCatalog = ""
	c.TargetTag = ""
	c.TargetCatalogSourceTemplate = ""
	pkgs, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", md5.Sum(pkgs))[0:32], nil
}

func (o FilterCollector) isAlreadyFiltered(ctx context.Context, srcImage, filteredImageDigest string) bool {
	imgSpec, err := image.ParseRef(srcImage)
	if err != nil {
		o.Log.Debug(errMsg, err.Error())
		return false
	}

	sourceCtx, err := o.Opts.SrcImage.NewSystemContext()
	if err != nil {
		return false
	}
	// OCPBUGS-37948 : No TLS verification when getting manifests from the cache registry
	if strings.Contains(srcImage, o.Opts.LocalStorageFQDN) { // when copying from cache, use HTTP
		sourceCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
	}

	catalogDigest, err := o.Manifest.ImageDigest(ctx, sourceCtx, imgSpec.ReferenceWithTransport)
	if err != nil {
		o.Log.Debug(errMsg, err.Error())
		return false
	}
	return filteredImageDigest == catalogDigest
}

// isDeleteOfV1CatalogFromDisk returns true when trying to delete an operator catalog mirrored by oc-mirror v1 and the catalog was on disk (using oci:// on the ImageSetConfiguration)
// TODO remove me when the migration from oc-mirror v1 to v2 ends
func (o *FilterCollector) isDeleteOfV1CatalogFromDisk() bool {
	return o.Opts.IsDiskToMirror() && o.Opts.IsDelete() && o.generateV1DestTags
}

// TODO remove me when the migration from oc-mirror v1 to v2 ends
func addOriginFromOperatorCatalogOnDisk(relatedImages *map[string][]v2alpha1.RelatedImage) {
	for key, images := range *relatedImages {
		for i := range images {
			// Modify the RelatedImage object as needed
			images[i].OriginFromOperatorCatalogOnDisk = true
		}
		(*relatedImages)[key] = images
	}
}

func (o FilterCollector) collectOperator( //nolint:cyclop // TODO: this needs further refactoring
	ctx context.Context,
	op v2alpha1.Operator,
	relatedImages map[string][]v2alpha1.RelatedImage,
	copyImageSchemaMap *v2alpha1.CopyImageSchemaMap,
) (v2alpha1.CatalogFilterResult, error) {
	// CLID-47 double check that targetCatalog is valid
	if op.TargetCatalog != "" && !v2alpha1.IsValidPathComponent(op.TargetCatalog) {
		return v2alpha1.CatalogFilterResult{}, fmt.Errorf("invalid targetCatalog %s", op.TargetCatalog)
	}

	// CLID-27 ensure we pick up oci:// (on disk) catalogs
	imgSpec, err := image.ParseRef(op.Catalog)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	catalogDigest, err := o.getCatalogDigest(ctx, op)
	if err != nil {
		// OCPBUGS-36548 (manifest unknown)
		return v2alpha1.CatalogFilterResult{}, err
	}

	result, err := o.filterOperator(ctx, op, imgSpec, catalogDigest)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	ri, err := o.ctlgHandler.getRelatedImagesFromCatalog(result.DeclConfig, copyImageSchemaMap)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}
	o.Log.Debug("Found %d related images for catalog %q", len(ri), op.Catalog)

	// OCPBUGS-45059
	// TODO: remove me when the migration from oc-mirror v1 to v2 ends
	if imgSpec.Transport == ociProtocol && o.isDeleteOfV1CatalogFromDisk() {
		addOriginFromOperatorCatalogOnDisk(&ri)
	}

	maps.Copy(relatedImages, ri)

	targetTag := op.TargetTag
	if len(targetTag) == 0 && imgSpec.Transport == ociProtocol {
		// for this case only, img.ParseRef(in its current state)
		// will not be able to determine the digest.
		// this leaves the oci imgSpec with no tag nor digest as it
		// goes to prepareM2DCopyBatch/prepareD2MCopyBath. This is
		// why we set the digest read from manifest in targetTag
		targetTag = "latest"
	}

	catalogName := op.TargetCatalog
	if len(catalogName) == 0 {
		catalogName = path.Base(imgSpec.Reference)
	}

	catalogImage := op.Catalog
	if imgSpec.Transport == ociProtocol {
		// ensure correct oci format and directory lookup
		sourceOCIDir, err := filepath.Abs(imgSpec.Reference)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, fmt.Errorf("failed to get OCI image path: %w", err)
		}
		catalogImage = ociProtocol + sourceOCIDir
	}

	rebuiltTag := ""
	if !isFullCatalog(op) {
		tag, err := digestOfFilter(op)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, err
		}
		rebuiltTag = tag
	}

	// OCPBUGS-52470
	// check if the original operator was mirrored by digest
	componentName := imgSpec.ComponentName() + "." + result.Digest
	if imgSpec.IsImageByDigestOnly() && o.Opts.IsMirrorToDisk() {
		tag, err := digestOfFilter(op)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, err
		}
		componentName = imgSpec.ComponentName() + "." + tag
	}

	relatedImages[componentName] = []v2alpha1.RelatedImage{
		{
			Name:          catalogName,
			Image:         catalogImage,
			Type:          v2alpha1.TypeOperatorCatalog,
			TargetTag:     targetTag,
			TargetCatalog: op.TargetCatalog,
			RebuiltTag:    rebuiltTag,
		},
	}
	return result, nil
}

func (o FilterCollector) getCatalogDigest(ctx context.Context, op v2alpha1.Operator) (string, error) {
	// OCPBUGS-36214: For diskToMirror (and delete), access to the source registry is not guaranteed
	if o.Opts.IsDiskToMirror() || o.Opts.IsDeleteMode() {
		return o.catalogDigest(ctx, op)
	}

	imgSpec, err := image.ParseRef(op.Catalog)
	if err != nil {
		return "", err
	}

	srcCtx, err := o.Opts.SrcImage.NewSystemContext()
	if err != nil {
		return "", err
	}

	return o.Manifest.ImageDigest(ctx, srcCtx, imgSpec.ReferenceWithTransport)
}

func (o FilterCollector) filterOperator(ctx context.Context, op v2alpha1.Operator, imgSpec image.ImageSpec, catalogDigest string) (v2alpha1.CatalogFilterResult, error) { //nolint:cyclop // TODO: this needs further refactoring
	o.Log.Debug("Filtering catalog %q", op.Catalog)
	imageIndexDir := filepath.Join(o.Opts.Global.WorkingDir, operatorCatalogsDir, imgSpec.ComponentName(), catalogDigest)
	filteredCatalogsDir := filepath.Join(imageIndexDir, operatorCatalogFilteredDir)

	filterDigest, err := digestOfFilter(op)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	var isAlreadyFiltered bool
	filteredImageDigest, err := os.ReadFile(filepath.Join(filteredCatalogsDir, filterDigest, "digest"))
	if err != nil {
		// If there was an error reading the digest file, we assume the catalog has not been filtered
		isAlreadyFiltered = false
	} else {
		// digest read
		srcFilteredCatalog, err := o.cachedCatalog(op, filterDigest)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, err
		}

		isAlreadyFiltered = o.isAlreadyFiltered(ctx, srcFilteredCatalog, string(filteredImageDigest))
	}

	if isAlreadyFiltered {
		filterConfigDir := filepath.Join(filteredCatalogsDir, filterDigest, operatorCatalogConfigDir)
		filteredDC, err := o.ctlgHandler.getDeclarativeConfig(filterConfigDir)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, fmt.Errorf("retrieve filtered catalog config from %s: %w", filterConfigDir, err)
		}
		return v2alpha1.CatalogFilterResult{
			OperatorFilter:     op,
			FilteredConfigPath: filterConfigDir,
			ToRebuild:          false,
			DeclConfig:         filteredDC,
			Digest:             string(filteredImageDigest),
		}, nil
	}
	o.Log.Debug("Catalog has not been filtered previously")

	if err := o.ensureCatalogInOCIFormat(ctx, imgSpec, op.Catalog, imageIndexDir); err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	// It's now in oci format so we can go directly to the index.json file
	dcPath, err := o.extractOCIConfigLayers(op.Catalog, imgSpec, imageIndexDir)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	originalDC, err := o.ctlgHandler.getDeclarativeConfig(dcPath)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	// No filtering needs to be done if we're copying the whole catalog
	if isFullCatalog(op) {
		return v2alpha1.CatalogFilterResult{
			OperatorFilter:     op,
			FilteredConfigPath: "", // this value is not relevant: no rebuilding
			ToRebuild:          false,
			DeclConfig:         originalDC,
			Digest:             catalogDigest,
		}, nil
	}

	filteredDC, err := filterCatalog(ctx, *originalDC, op)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	filteredDigestPath := filepath.Join(filteredCatalogsDir, filterDigest, operatorCatalogConfigDir)
	if err := createFolders([]string{filteredDigestPath}); err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	if err := saveDeclarativeConfig(*filteredDC, filteredDigestPath); err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	return v2alpha1.CatalogFilterResult{
		OperatorFilter:     op,
		FilteredConfigPath: filteredDigestPath,
		ToRebuild:          true,
		DeclConfig:         filteredDC,
		Digest:             catalogDigest,
	}, nil
}

func (o FilterCollector) ensureCatalogInOCIFormat(ctx context.Context, imgSpec image.ImageSpec, catalog, imageIndexDir string) error {
	o.Log.Debug("Ensuring catalog is in OCI format")
	catalogImageDir := filepath.Join(imageIndexDir, operatorCatalogImageDir)

	if imgSpec.Transport != ociProtocol {
		opts := o.Opts
		opts.Stdout = io.Discard
		opts.RemoveSignatures = true
		opts.Global.SecurePolicy = false

		src := dockerProtocol + catalog
		dest := ociProtocolTrimmed + catalogImageDir

		// Prepare folders
		if err := createFolders([]string{catalogImageDir}); err != nil {
			return err
		}
		return o.Mirror.Run(ctx, src, dest, mirror.CopyMode, &opts)
	}

	o.Log.Debug("Catalog %q already in OCI format", catalog)
	if _, err := os.Stat(filepath.Join(catalogImageDir, "index.json")); err != nil {
		// If we cannot determine whether the catalog exists in OCI format at
		// the working-dir destination, either because of `stat` failures or
		// because it's the first time we are doing this
		//
		// delete the existing directory and untarred cache contents
		os.RemoveAll(catalogImageDir)
		os.RemoveAll(filepath.Join(imageIndexDir, operatorCatalogConfigDir))
		// copy all contents to the working dir
		if err := copy.Copy(imgSpec.PathComponent, catalogImageDir); err != nil {
			return fmt.Errorf("copy OCI contents to working-dir: %w", err)
		}
	}

	return nil
}
