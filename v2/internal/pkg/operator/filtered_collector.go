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
	"sort"
	"strings"

	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/spinners"
	"github.com/otiai10/copy"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
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

	for _, op := range o.Config.Mirror.Operators {
		// download the operator index image
		o.Log.Debug(collectorPrefix+"copying operator image %s", op.Catalog)

		// prepare spinner
		p := mpb.New()
		spinner := p.AddSpinner(
			1, mpb.BarFillerMiddleware(spinners.PositionSpinnerLeft),
			mpb.BarWidth(3),
			mpb.PrependDecorators(
				decor.OnComplete(spinners.EmptyDecorator(), "\x1b[1;92m ✓ \x1b[0m"),
				decor.OnAbort(spinners.EmptyDecorator(), "\x1b[1;91m ✗ \x1b[0m"),
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
			o.Log.Error(errMsg, err.Error())
			spinner.Abort(true)
			spinner.Wait()
			return v2alpha1.CollectorSchema{}, err
		}

		result, err := o.collectOperator(ctx, op, relatedImages, copyImageSchemaMap)
		if err != nil {
			spinner.Abort(true)
			spinner.Wait()
			return v2alpha1.CollectorSchema{}, err
		}
		collectorSchema.CatalogToFBCMap[imgSpec.ReferenceWithTransport] = result

		spinner.Increment()
		p.Wait()
	}

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
		o.Log.Error(errMsg, err.Error())
		return v2alpha1.CollectorSchema{}, err
	}

	sort.Sort(ByTypePriority(allImages))

	collectorSchema.AllImages = allImages
	collectorSchema.CopyImageSchemaMap = *copyImageSchemaMap

	return collectorSchema, nil
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

	catalogDigest, err := o.Manifest.GetDigest(ctx, sourceCtx, imgSpec.ReferenceWithTransport)
	if err != nil {
		o.Log.Debug(errMsg, err.Error())
		return false
	}
	return filteredImageDigest == catalogDigest
}

func (o FilterCollector) collectOperator(ctx context.Context, op v2alpha1.Operator, relatedImages map[string][]v2alpha1.RelatedImage, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) (v2alpha1.CatalogFilterResult, error) {
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
			o.Log.Error(errMsg, err.Error())
			return v2alpha1.CatalogFilterResult{}, err
		}
		catalogImage = ociProtocol + sourceOCIDir
	}

	rebuiltTag := ""
	if result.ToRebuild {
		tag, err := digestOfFilter(op)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, err
		}
		rebuiltTag = tag
	}

	componentName := imgSpec.ComponentName() + "." + result.Digest
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

	return o.Manifest.GetDigest(ctx, srcCtx, imgSpec.ReferenceWithTransport)
}

func (o FilterCollector) filterOperator(ctx context.Context, op v2alpha1.Operator, imgSpec image.ImageSpec, catalogDigest string) (v2alpha1.CatalogFilterResult, error) {
	imageIndexDir := filepath.Join(o.Opts.Global.WorkingDir, operatorCatalogsDir, imgSpec.ComponentName(), catalogDigest)
	configsDir := filepath.Join(imageIndexDir, operatorCatalogConfigDir)
	catalogImageDir := filepath.Join(imageIndexDir, operatorCatalogImageDir)
	filteredCatalogsDir := filepath.Join(imageIndexDir, operatorCatalogFilteredDir)

	filterDigest, err := digestOfFilter(op)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	filteredImageDigest, err := os.ReadFile(filepath.Join(filteredCatalogsDir, filterDigest, "digest"))
	if err != nil {
		// If there was an error reading the digest file, we assume the catalog has not been filtered
	} else { // digest read
		srcFilteredCatalog, err := o.cachedCatalog(op, filterDigest)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, err
		}

		if o.isAlreadyFiltered(ctx, srcFilteredCatalog, string(filteredImageDigest)) {
			filterConfigDir := filepath.Join(filteredCatalogsDir, filterDigest)
			filteredDC, err := o.ctlgHandler.getDeclarativeConfig(filterConfigDir)
			if err != nil {
				return v2alpha1.CatalogFilterResult{}, err
			}
			return v2alpha1.CatalogFilterResult{
				OperatorFilter:     op,
				FilteredConfigPath: filterConfigDir,
				ToRebuild:          false,
				DeclConfig:         filteredDC,
				Digest:             string(filteredImageDigest),
			}, nil
		}
	}

	if err := o.ensureCatalogInOCIFormat(ctx, imgSpec, op.Catalog, imageIndexDir); err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	// It's now in oci format so we can go directly to the index.json file
	oci, err := o.Manifest.GetImageIndex(catalogImageDir)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	if isMultiManifestIndex(*oci) && imgSpec.Transport == ociProtocol {
		if err := o.Manifest.ConvertIndexToSingleManifest(catalogImageDir, oci); err != nil {
			return v2alpha1.CatalogFilterResult{}, err
		}

		oci, err = o.Manifest.GetImageIndex(catalogImageDir)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, err
		}
	}

	if len(oci.Manifests) == 0 {
		return v2alpha1.CatalogFilterResult{}, fmt.Errorf("no manifests found for %s", op.Catalog)
	}

	validDigest, err := digest.Parse(oci.Manifests[0].Digest)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, fmt.Errorf("the digests seem to be incorrect for %s: %w", op.Catalog, err)
	}

	manifest := validDigest.Encoded()
	o.Log.Debug(collectorPrefix+"manifest %s", manifest)
	manifestDir := filepath.Join(catalogImageDir, blobsDir, manifest)
	oci, err = o.Manifest.GetImageManifest(manifestDir)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	// we need to check if oci returns multi manifests
	// (from manifest list) also oci.Config will be nil
	// we are only interested in the first manifest as all
	// architectures "configs" will be exactly the same
	if len(oci.Manifests) > 1 && oci.Config.Size == 0 {
		subDigest, err := digest.Parse(oci.Manifests[0].Digest)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, fmt.Errorf("the digests seem to be incorrect for %s: %w", op.Catalog, err)
		}
		manifestDir := filepath.Join(catalogImageDir, blobsDir, subDigest.Encoded())
		oci, err = o.Manifest.GetImageManifest(manifestDir)
		if err != nil {
			return v2alpha1.CatalogFilterResult{}, fmt.Errorf("manifest %s: %w", op.Catalog, err)
		}
	}

	// read the config digest to get the detailed manifest
	// looking for the label to search for a specific folder
	configDigest, err := digest.Parse(oci.Config.Digest)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, fmt.Errorf("the digests seem to be incorrect for %s: %w", op.Catalog, err)
	}
	catalogDir := filepath.Join(catalogImageDir, blobsDir, configDigest.Encoded())
	ocs, err := o.Manifest.GetOperatorConfig(catalogDir)
	if err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	label := ocs.Config.Labels.OperatorsOperatorframeworkIoIndexConfigsV1
	o.Log.Debug(collectorPrefix+"label %s", label)

	// untar all the blobs for the operator
	// if the layer with "label" (from previous step) is found to a specific folder
	fromDir := filepath.Join(catalogImageDir, blobsDir)
	if err := o.Manifest.ExtractLayersOCI(fromDir, configsDir, label, oci); err != nil {
		return v2alpha1.CatalogFilterResult{}, err
	}

	originalDC, err := o.ctlgHandler.getDeclarativeConfig(filepath.Join(configsDir, label))
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
	catalogImageDir := filepath.Join(imageIndexDir, operatorCatalogImageDir)

	if imgSpec.Transport != ociProtocol {
		opts := o.Opts
		opts.Stdout = io.Discard

		src := dockerProtocol + catalog
		dest := ociProtocolTrimmed + catalogImageDir

		// Prepare folders
		if err := createFolders([]string{catalogImageDir}); err != nil {
			return err
		}
		return o.Mirror.Run(ctx, src, dest, mirror.CopyMode, &opts)
	}

	if _, err := os.Stat(filepath.Join(catalogImageDir, "index.json")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// delete the existing directory and untarred cache contents
			os.RemoveAll(catalogImageDir)
			os.RemoveAll(filepath.Join(imageIndexDir, operatorCatalogConfigDir))
			// copy all contents to the working dir
			return copy.Copy(imgSpec.PathComponent, catalogImageDir)
		} else {
			// FIXME: what to do here?
		}
	}

	return nil
}
