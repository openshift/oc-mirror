package operator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/otiai10/copy"
	filter "github.com/sherine-k/catalog-filter/pkg/filter/mirror-config/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/folder"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type CatalogHandler struct {
	Log      clog.PluggableLoggerInterface
	Manifest manifest.ManifestInterface
	Mirror   mirror.MirrorInterface
}

func (o CatalogHandler) GetDeclarativeConfig(ctx context.Context, filePath string) (*declcfg.DeclarativeConfig, error) {
	dc, err := declcfg.LoadFS(ctx, os.DirFS(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to load catalog config: %w", err)
	}
	return dc, nil
}

func saveDeclarativeConfig(fbc declcfg.DeclarativeConfig, path string) error {
	if err := declcfg.WriteFS(fbc, path, declcfg.WriteJSON, ".json"); err != nil {
		return fmt.Errorf("failed to save catalog config: %w", err)
	}
	return nil
}

func filterPackage(op v2alpha1.IncludePackage) filter.Package {
	p := filter.Package{
		Name:           op.Name,
		DefaultChannel: op.DefaultChannel,
	}
	if op.MinVersion != "" {
		p.VersionRange = ">=" + op.MinVersion
	}
	if op.MaxVersion != "" {
		p.VersionRange += " <=" + op.MaxVersion
	}

	if len(op.Channels) == 0 {
		return p
	}

	p.Channels = make([]filter.Channel, 0, len(op.Channels))
	for _, ch := range op.Channels {
		filterChan := filter.Channel{
			Name: ch.Name,
		}
		if ch.MinVersion != "" {
			filterChan.VersionRange = ">=" + ch.MinVersion
		}
		if ch.MaxVersion != "" {
			filterChan.VersionRange += " <=" + ch.MaxVersion
		}
		p.Channels = append(p.Channels, filterChan)
	}

	return p
}

func filterFromImageSetConfig(iscCatalogFilter v2alpha1.Operator) (filter.FilterConfiguration, error) {
	catFilter := filter.FilterConfiguration{
		TypeMeta: v1.TypeMeta{
			Kind:       "FilterConfiguration",
			APIVersion: "olm.operatorframework.io/filter/mirror/v1alpha1",
		},
	}

	catFilter.Packages = make([]filter.Package, 0, len(iscCatalogFilter.Packages))
	for _, op := range iscCatalogFilter.Packages {
		catFilter.Packages = append(catFilter.Packages, filterPackage(op))
	}

	if err := catFilter.Validate(); err != nil {
		return catFilter, fmt.Errorf("failed to validate catalog filter: %w", err)
	}
	return catFilter, nil
}

func filterCatalog(ctx context.Context, operatorCatalog declcfg.DeclarativeConfig, iscCatalogFilter v2alpha1.Operator) (*declcfg.DeclarativeConfig, error) {
	config, err := filterFromImageSetConfig(iscCatalogFilter)
	if err != nil {
		return nil, err
	}
	ctlgFilter := filter.NewMirrorFilter(config, []filter.FilterOption{filter.InFull(iscCatalogFilter.Full)}...)
	dc, err := ctlgFilter.FilterCatalog(ctx, &operatorCatalog)
	if err != nil {
		return nil, fmt.Errorf("failed to filter catalog: %w", err)
	}
	return dc, nil
}

func (o CatalogHandler) getRelatedImagesFromCatalog(dc *declcfg.DeclarativeConfig, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) (map[string][]v2alpha1.RelatedImage, error) {
	var errs []error
	relatedImages := make(map[string][]v2alpha1.RelatedImage)
	for _, bundle := range dc.Bundles {
		ris, err := handleRelatedImages(bundle, bundle.Package, copyImageSchemaMap)
		if err != nil {
			o.Log.Warn("%s SKIPPING bundle %s of operator %s", err.Error(), bundle.Name, bundle.Package)
			errs = append(errs, err)
			continue
		}
		relatedImages[bundle.Name] = ris
	}

	if len(relatedImages) == 0 {
		errs = append(errs, errors.New("no related images found"))
	}

	return relatedImages, errors.Join(errs...)
}

// handleRelatedImages returns the images to mirror for a bundle: its bundle image,
// when olm.bundle.image is set, followed by the bundle's related images.
func handleRelatedImages(bundle declcfg.Bundle, operatorName string, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) ([]v2alpha1.RelatedImage, error) {
	var relatedImages []v2alpha1.RelatedImage

	// OCPBUGS-132865: the bundle image (olm.bundle.image) must be mirrored even when the
	// catalog does not repeat it in relatedImages. When it is repeated, the first matching
	// entry provides its name and the image is collected only once.
	if bundle.Image != "" {
		bundleImage := v2alpha1.RelatedImage{
			Image: bundle.Image,
			Type:  v2alpha1.TypeOperatorBundle,
		}
		i := slices.IndexFunc(bundle.RelatedImages, func(ri declcfg.RelatedImage) bool {
			return ri.Image == bundle.Image
		})
		if i >= 0 {
			bundleImage.Name = bundle.RelatedImages[i].Name
		}
		if err := addOperatorImage(bundle, operatorName, bundleImage.Image, copyImageSchemaMap); err != nil {
			return relatedImages, err
		}
		relatedImages = append(relatedImages, bundleImage)
	}

	for _, ri := range bundle.RelatedImages {
		if bundle.Image != "" && ri.Image == bundle.Image {
			// Already collected as the bundle image.
			continue
		}
		if err := addOperatorImage(bundle, operatorName, ri.Image, copyImageSchemaMap); err != nil {
			return relatedImages, err
		}
		relatedImages = append(relatedImages, v2alpha1.RelatedImage{
			Name:  ri.Name,
			Image: ri.Image,
			Type:  v2alpha1.TypeOperatorRelatedImage,
		})
	}

	return relatedImages, nil
}

// addOperatorImage validates an image referenced by a bundle and records that the
// bundle and its operator depend on it.
func addOperatorImage(bundle declcfg.Bundle, operatorName, img string, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) error {
	if strings.Contains(img, consts.OciProtocol) {
		return fmt.Errorf("invalid image: %s 'oci' is not supported in operator catalogs", img)
	}

	imgSpec, err := image.ParseRef(img)
	if err != nil {
		return fmt.Errorf("error parsing image %s: %w", img, err)
	}

	operators := copyImageSchemaMap.OperatorsByImage[imgSpec.ReferenceWithTransport]

	if _, found := operators[operatorName]; !found {
		if operators == nil {
			copyImageSchemaMap.OperatorsByImage[imgSpec.ReferenceWithTransport] = make(map[string]struct{})
		}
		copyImageSchemaMap.OperatorsByImage[imgSpec.ReferenceWithTransport][operatorName] = struct{}{}
	}

	bundles := copyImageSchemaMap.BundlesByImage[imgSpec.ReferenceWithTransport]
	if _, found := bundles[bundle.Name]; !found {
		if bundles == nil {
			copyImageSchemaMap.BundlesByImage[imgSpec.ReferenceWithTransport] = make(map[string]string)
		}
		copyImageSchemaMap.BundlesByImage[imgSpec.ReferenceWithTransport][bundle.Image] = bundle.Name
	}

	return nil
}

// EnsureCatalogInOCIFormat makes the catalog available as an OCI layout in the
// catalog-image directory under imageIndexDir. A catalog referenced with oci:// is
// copied from its local layout if it is not there yet. Any other catalog is copied
// from its registry, except in diskToMirror (but not delete), where the copy
// extracted from the mirror archive is used and the source registry is never
// contacted.
func (o CatalogHandler) EnsureCatalogInOCIFormat(ctx context.Context, imgSpec image.ImageSpec, catalog, imageIndexDir string, opts mirror.CopyOptions) error {
	o.Log.Debug("Ensuring catalog %q is in OCI format", catalog)
	catalogImageDir := filepath.Join(imageIndexDir, operatorCatalogImageDir)

	if imgSpec.Transport != consts.OciProtocol {
		// OCPBUGS-136389: diskToMirror must not depend on the source registry being
		// reachable. The catalog was already copied to the working-dir during
		// mirrorToDisk and shipped in the archive, so use that copy instead.
		if opts.IsDiskToMirror() && !opts.IsDeleteMode() {
			return o.ensureArchivedCatalog(catalogImageDir)
		}

		// modify a copy, no the pointed value
		var gOpts mirror.GlobalOptions
		if opts.Global != nil {
			gOpts = *opts.Global
		}
		localOpts := opts
		localOpts.Global = &gOpts
		localOpts.Stdout = io.Discard
		localOpts.RemoveSignatures = true
		localOpts.Global.SecurePolicy = false

		src := consts.DockerProtocol + catalog
		dest := consts.OciProtocolTrimmed + catalogImageDir

		// Prepare folders
		if err := folder.CreateFolders(catalogImageDir); err != nil {
			return err
		}
		return o.Mirror.Run(ctx, src, dest, mirror.CopyMode, &localOpts)
	}

	o.Log.Debug("Catalog %q already in OCI format", catalog)
	if _, err := os.Stat(filepath.Join(catalogImageDir, "index.json")); err != nil {
		// If we cannot determine whether the catalog exists in OCI format at
		// the working-dir destination, either because of `stat` failures or
		// because it's the first time we are doing this
		//
		// delete the existing directory and untarred cache contents
		if err := folder.RemoveFolders(catalogImageDir, filepath.Join(imageIndexDir, operatorCatalogConfigDir)); err != nil {
			return fmt.Errorf("failed to delete old content: %w", err)
		}
		// copy all contents to the working dir
		if err := copy.Copy(imgSpec.PathComponent, catalogImageDir); err != nil {
			return fmt.Errorf("copy OCI contents to working-dir: %w", err)
		}
	}

	return nil
}

// ensureArchivedCatalog verifies that the OCI copy of the catalog extracted from
// the mirror archive is present in the working-dir. It never falls back to the
// source registry: if the copy is missing, diskToMirror fails with a local error.
// The content of the OCI layout is validated later, when its layers are extracted.
func (o CatalogHandler) ensureArchivedCatalog(catalogImageDir string) error {
	if _, err := os.Stat(filepath.Join(catalogImageDir, "index.json")); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("catalog not found in the mirror archive (expected at %s): ensure all archive files are present and the ImageSetConfiguration matches the one used for mirrorToDisk, diskToMirror does not access the source registry: %w", catalogImageDir, err)
		}
		return fmt.Errorf("unable to access the catalog from the mirror archive at %s: %w", catalogImageDir, err)
	}
	o.Log.Debug("Using the catalog copy extracted from the mirror archive")
	return nil
}

func (o CatalogHandler) ExtractOCIConfigLayers(imgSpec image.ImageSpec, imageIndexDir string) (string, error) { //nolint:cyclop // TODO: this needs further refactoring
	o.Log.Debug("Extracting OCI catalog layers")
	configsDir := filepath.Join(imageIndexDir, operatorCatalogConfigDir)
	catalogImageDir := filepath.Join(imageIndexDir, operatorCatalogImageDir)

	if err := folder.CreateFolders(configsDir, catalogImageDir); err != nil {
		return "", err
	}

	// It's in oci format so we can go directly to the index.json file
	oci, err := o.Manifest.GetOCIImageIndex(filepath.Join(catalogImageDir, "index.json"))
	if err != nil {
		return "", err
	}

	if len(oci.Manifests) > 1 && imgSpec.Transport == consts.OciProtocol {
		if err := o.Manifest.ConvertOCIIndexToSingleManifest(catalogImageDir, oci); err != nil {
			return "", err
		}
	}

	img, err := o.Manifest.GetOCIImageFromIndex(catalogImageDir)
	if err != nil {
		return "", fmt.Errorf("failed to get catalog oci image: %w", err)
	}

	imgConfig, err := img.ConfigFile()
	if err != nil {
		return "", fmt.Errorf("failed to get catalog oci image config: %w", err)
	}

	label := imgConfig.Config.Labels[operatorsConfigsV1Label]
	o.Log.Debug(collectorPrefix+"label %q", label)

	// untar all the blobs for the operator
	// if the layer with "label" (from previous step) is found to a specific folder
	if err := o.Manifest.ExtractOCILayers(img, configsDir, label); err != nil {
		return "", err
	}

	return filepath.Join(configsDir, label), nil
}
