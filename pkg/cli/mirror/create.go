package mirror

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/uuid"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	oc "github.com/openshift/oc-mirror/pkg/cli/mirror/operatorcatalog"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

// Create will plan a mirroring operation based on provided configuration
func (o *MirrorOptions) Create(ctx context.Context, cfg v1alpha2.ImageSetConfiguration) (v1alpha2.Metadata, image.TypedImageMapping, map[string]map[oc.OperatorCatalogPlatform]oc.CatalogMetadata, error) {
	// Determine stateless or stateful mode.
	// Empty storage configuration will trigger a metadata cleanup
	// action and labels metadata as single use
	path := filepath.Join(o.Dir, config.SourceDir)
	meta := v1alpha2.NewMetadata()
	var backend storage.Backend
	var err error
	if !cfg.StorageConfig.IsSet() {
		meta.SingleUse = true
		klog.Warningf("backend is not configured in %s, using stateless mode", o.ConfigPath)
		cfg.StorageConfig.Local = &v1alpha2.LocalConfig{Path: path}
		backend, err = storage.ByConfig(path, cfg.StorageConfig)
		if err != nil {
			return meta, image.TypedImageMapping{}, map[string]map[oc.OperatorCatalogPlatform]oc.CatalogMetadata{}, fmt.Errorf("error opening backend: %v", err)
		}
		defer func() {
			if err := backend.Cleanup(ctx, config.MetadataBasePath); err != nil {
				klog.Error(err)
			}
		}()
	} else {
		meta.SingleUse = false
		backend, err = storage.ByConfig(path, cfg.StorageConfig)
		if err != nil {
			return meta, image.TypedImageMapping{}, map[string]map[oc.OperatorCatalogPlatform]oc.CatalogMetadata{}, fmt.Errorf("error opening backend: %v", err)
		}
	}
	thisRun := v1alpha2.PastMirror{
		Timestamp: int(time.Now().Unix()),
	}
	// Run full or diff mirror.
	merr := backend.ReadMetadata(ctx, &meta, config.MetadataBasePath)
	if merr != nil && !errors.Is(merr, storage.ErrMetadataNotExist) {
		return meta, image.TypedImageMapping{}, map[string]map[oc.OperatorCatalogPlatform]oc.CatalogMetadata{}, merr
	}
	// New metadata files get a full mirror, with complete/heads-only catalogs, release images,
	// and a new UUID. Otherwise, use data from the last mirror to mirror just the layer diff.
	switch {
	case merr != nil:
		klog.Info("No metadata detected, creating new workspace")
		meta.Uid = uuid.New()
		thisRun.Sequence = 1
		thisRun.Mirror = cfg.Mirror
		f := func(ctx context.Context, cfg v1alpha2.ImageSetConfiguration, allCatalogs map[string]map[oc.OperatorCatalogPlatform]oc.CatalogMetadata) (image.TypedImageMapping, error) {
			if len(cfg.Mirror.Operators) != 0 {
				operator := NewOperatorOptions(o)
				operator.SkipImagePin = o.SkipImagePin
				return operator.PlanFull(ctx, cfg, allCatalogs)
			}
			return image.TypedImageMapping{}, nil
		}
		mmapping, allCatalogs, err := o.run(ctx, &cfg, meta, f)
		meta.PastMirror = thisRun
		return meta, mmapping, allCatalogs, err
	default:
		lastRun := meta.PastMirror
		thisRun.Sequence = lastRun.Sequence + 1
		thisRun.Mirror = cfg.Mirror
		f := func(ctx context.Context, cfg v1alpha2.ImageSetConfiguration, allCatalogs map[string]map[oc.OperatorCatalogPlatform]oc.CatalogMetadata) (image.TypedImageMapping, error) {
			if len(cfg.Mirror.Operators) != 0 {
				operator := NewOperatorOptions(o)
				operator.SkipImagePin = o.SkipImagePin
				return operator.PlanDiff(ctx, cfg, allCatalogs, lastRun)
			}
			return image.TypedImageMapping{}, nil
		}
		mmapping, allCatalogs, err := o.run(ctx, &cfg, meta, f)
		meta.PastMirror = thisRun
		return meta, mmapping, allCatalogs, err
	}
}

/*
operatorFunc is a function signature for operator planning operations

# Arguments

• ctx: A cancellation context

• cfg: An ImageSetConfiguration that should be processed

• allCatalogs: A pre-populated map of all catalog metadata loaded with as much data as possible.
For each v1alpha2.Operator entry in cfg, there's a corresponding map entry. The key is the
v1alpha2.Operator.Catalog string, and the value is map[OperatorCatalogPlatform]CatalogMetadata
containing whatever we've discovered so far.

# Returns

• image.TypedImageMapping: Any src->dest mappings found during planning. Will be nil if an error occurs, non-nil otherwise.

• error: non-nil if an error occurs, nil otherwise
*/
type operatorFunc func(
	ctx context.Context,
	cfg v1alpha2.ImageSetConfiguration,
	allCatalogs map[string]map[oc.OperatorCatalogPlatform]oc.CatalogMetadata,
) (image.TypedImageMapping, error)

func (o *MirrorOptions) run(
	ctx context.Context,
	cfg *v1alpha2.ImageSetConfiguration,
	meta v1alpha2.Metadata,
	operatorPlan operatorFunc,
) (image.TypedImageMapping, map[string]map[oc.OperatorCatalogPlatform]oc.CatalogMetadata, error) {

	mmappings := image.TypedImageMapping{}
	/*
		for each one of the v1alpha2.Operator entries we encounter, we need collect metadata
		about them, and add them into a map representing all catalogs we've discovered
		- key is the v1alpha2.Operator.Catalog for lookup during planning
		- value is map[OperatorCatalogPlatform]CatalogMetadata
	*/
	allCatalogs := map[string]map[oc.OperatorCatalogPlatform]oc.CatalogMetadata{}

	if len(cfg.Mirror.Platform.Channels) != 0 {
		release := NewReleaseOptions(o)
		mappings, err := release.Plan(ctx, meta.PastMirror, cfg)
		if err != nil {
			return mmappings, allCatalogs, err
		}
		mmappings.Merge(mappings)

		if cfg.Mirror.Platform.Graph {
			klog.Info("Adding graph data")
			// Always add the graph base image to the metadata if needed,
			// to ensure it does not get pruned before use.
			cfg.Mirror.AdditionalImages = append(cfg.Mirror.AdditionalImages, v1alpha2.Image{Name: graphBaseImage})

			releaseDir := filepath.Join(o.Dir, config.SourceDir, config.GraphDataDir)
			if err := os.MkdirAll(releaseDir, 0750); err != nil {
				return mmappings, allCatalogs, err
			}
			if err := downloadGraphData(ctx, releaseDir); err != nil {
				return mmappings, allCatalogs, err
			}
		}
	}

	// setup isInsecure flag outside the loop
	isInsecure := false
	if o.SourcePlainHTTP || o.SourceSkipTLS {
		isInsecure = true
	}

	for _, ctlg := range cfg.Mirror.Operators {
		var catRef string
		var layoutPath *layout.Path
		if ctlg.IsFBCOCI() {
			// OCI image reference needs special handling

			// get the catalog reference
			// TODO: still not thrilled with this... this tries to get an ID when it does not have to since that'll happen later
			//       in getImageDigests, and the code is still parsing a path like it means something
			// TODO: could this just be replaced with _, _, repo, _, _ := v1alpha2.ParseImageReference(ref) instead
			//       especially since this is the OCI pathway?
			parsedRef, err := image.ParseReference(ctlg.Catalog)
			if err != nil {
				return mmappings, allCatalogs, err
			}
			catRef = parsedRef.Ref.Name

			// this is OCI so we have a layout path
			lp, err := layout.FromPath(v1alpha2.TrimProtocol(ctlg.Catalog))
			if err != nil {
				return mmappings, allCatalogs, err
			}
			layoutPath = &lp
		} else {
			// docker image reference, use as-is
			catRef = ctlg.Catalog
			// no layout path for a docker reference
			layoutPath = nil
		}

		// get the digests to process... could be more than one if a manifest list image is provided
		// we do this here so we don't have to do it multiple times
		catalogMetadataByPlatform, err := getCatalogMetadataByPlatform(ctx, catRef, layoutPath, isInsecure)
		if err != nil {
			return mmappings, allCatalogs, fmt.Errorf("error fetching digests for catalog %s: %v", ctlg.Catalog, err)
		}
		allCatalogs[ctlg.Catalog] = catalogMetadataByPlatform
	}

	// err := o.createOlmArtifactsForOCI(ctx, *cfg)
	// if err != nil {
	// 	return mmappings, err
	// }

	mappings, err := operatorPlan(ctx, *cfg, allCatalogs)
	if err != nil {
		return mmappings, allCatalogs, err
	}
	mmappings.Merge(mappings)

	if len(cfg.Mirror.AdditionalImages) != 0 {
		additional := NewAdditionalOptions(o)
		mappings, err := additional.Plan(ctx, cfg.Mirror.AdditionalImages)
		if err != nil {
			return mmappings, allCatalogs, err
		}
		mmappings.Merge(mappings)
	}

	if len(cfg.Mirror.Helm.Local) != 0 || len(cfg.Mirror.Helm.Repositories) != 0 {
		helm := NewHelmOptions(o)
		mappings, err := helm.PullCharts(ctx, *cfg)
		if err != nil {
			return mmappings, allCatalogs, err
		}
		mmappings.Merge(mappings)
	}

	if len(cfg.Mirror.Samples) != 0 {
		klog.Info("sample images full not implemented")
	}

	return mmappings, allCatalogs, nil
}
