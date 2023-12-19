package mirror

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/uuid"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

// Create will plan a mirroring operation based on provided configuration
func (o *MirrorOptions) Create(ctx context.Context, cfg v1alpha2.ImageSetConfiguration) (v1alpha2.Metadata, image.TypedImageMapping, error) {
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
			return meta, image.TypedImageMapping{}, fmt.Errorf("error opening backend: %v", err)
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
			return meta, image.TypedImageMapping{}, fmt.Errorf("error opening backend: %v", err)
		}
	}
	thisRun := v1alpha2.PastMirror{
		Timestamp: int(time.Now().Unix()),
	}
	// Run full or diff mirror.
	merr := backend.ReadMetadata(ctx, &meta, config.MetadataBasePath)
	if merr != nil && !errors.Is(merr, storage.ErrMetadataNotExist) {
		return meta, image.TypedImageMapping{}, merr
	}
	// New metadata files get a full mirror, with complete/heads-only catalogs, release images,
	// and a new UUID. Otherwise, use data from the last mirror to mirror just the layer diff.
	switch {
	case merr != nil:
		klog.Info("No metadata detected, creating new workspace")
		meta.Uid = uuid.New()
		thisRun.Sequence = 1
		thisRun.Mirror = cfg.Mirror
		f := func(ctx context.Context, cfg v1alpha2.ImageSetConfiguration) (image.TypedImageMapping, error) {
			if len(cfg.Mirror.Operators) != 0 {
				operator := NewOperatorOptions(o)
				operator.SkipImagePin = o.SkipImagePin
				return operator.PlanFull(ctx, cfg)
			}
			return image.TypedImageMapping{}, nil
		}
		mmapping, err := o.run(ctx, &cfg, meta, f)
		meta.PastMirror = thisRun
		return meta, mmapping, err
	default:
		lastRun := meta.PastMirror
		thisRun.Sequence = lastRun.Sequence + 1
		thisRun.Mirror = cfg.Mirror
		f := func(ctx context.Context, cfg v1alpha2.ImageSetConfiguration) (image.TypedImageMapping, error) {
			if len(cfg.Mirror.Operators) != 0 {
				operator := NewOperatorOptions(o)
				operator.SkipImagePin = o.SkipImagePin
				return operator.PlanDiff(ctx, cfg, lastRun)
			}
			return image.TypedImageMapping{}, nil
		}
		mmapping, err := o.run(ctx, &cfg, meta, f)
		meta.PastMirror = thisRun
		return meta, mmapping, err
	}
}

/*
operatorFunc is a function signature for operator planning operations

# Arguments

• ctx: A cancellation context

• cfg: An ImageSetConfiguration that should be processed

# Returns

• image.TypedImageMapping: Any src->dest mappings found during planning. Will be nil if an error occurs, non-nil otherwise.

• error: non-nil if an error occurs, nil otherwise
*/
type operatorFunc func(
	ctx context.Context,
	cfg v1alpha2.ImageSetConfiguration,
) (image.TypedImageMapping, error)

func (o *MirrorOptions) run(
	ctx context.Context,
	cfg *v1alpha2.ImageSetConfiguration,
	meta v1alpha2.Metadata,
	operatorPlan operatorFunc,
) (image.TypedImageMapping, error) {

	mmappings := image.TypedImageMapping{}

	if len(cfg.Mirror.Platform.Channels) != 0 {
		release := NewReleaseOptions(o)
		mappings, err := release.Plan(ctx, meta.PastMirror, cfg)
		if err != nil {
			return mmappings, err
		}
		mmappings.Merge(mappings)

		if cfg.Mirror.Platform.Graph {
			klog.Info("Adding graph data")
			// Always add the graph base image to the metadata if needed,
			// to ensure it does not get pruned before use.
			cfg.Mirror.AdditionalImages = append(cfg.Mirror.AdditionalImages, v1alpha2.Image{Name: graphBaseImage})

			releaseDir := filepath.Join(o.Dir, config.SourceDir, config.GraphDataDir)
			if err := os.MkdirAll(releaseDir, 0750); err != nil {
				return mmappings, err
			}
			if err := downloadGraphData(ctx, releaseDir); err != nil {
				return mmappings, err
			}
			klog.V(5).Infof("graph data download complete. Downloaded to %s", releaseDir)
		}
	}

	err := o.createOlmArtifactsForOCI(ctx, *cfg)
	if err != nil {
		return mmappings, err
	}

	mappings, err := operatorPlan(ctx, *cfg)
	if err != nil {
		return mmappings, err
	}
	mmappings.Merge(mappings)

	if len(cfg.Mirror.AdditionalImages) != 0 {
		additional := NewAdditionalOptions(o)
		mappings, err := additional.Plan(ctx, cfg.Mirror.AdditionalImages)
		if err != nil {
			return mmappings, err
		}
		mmappings.Merge(mappings)
	}

	if len(cfg.Mirror.Helm.Local) != 0 || len(cfg.Mirror.Helm.Repositories) != 0 {
		helm := NewHelmOptions(o)
		mappings, err := helm.PullCharts(ctx, *cfg)
		if err != nil {
			return mmappings, err
		}
		mmappings.Merge(mappings)
	}

	if len(cfg.Mirror.Samples) != 0 {
		klog.Info("sample images full not implemented")
	}

	return mmappings, nil
}

func (o *MirrorOptions) createOlmArtifactsForOCI(ctx context.Context, cfg v1alpha2.ImageSetConfiguration) error {
	for _, operator := range cfg.Mirror.Operators {
		if !operator.IsFBCOCI() {
			continue
		}

		ctlg, err := image.ParseReference(operator.Catalog)
		if err != nil {
			return err
		}

		// setup where the FBC content will be written to
		catalogContentsDir := filepath.Join(artifactsFolderName, ctlg.Ref.Name)
		// obtain the path to where the OCI image reference resides
		layoutPath := layout.Path(v1alpha2.TrimProtocol(ctlg.OCIFBCPath))

		// get its index.json and obtain its manifest
		rootIndex, err := layoutPath.ImageIndex()
		if err != nil {
			return err
		}
		rootIndexManifest, err := rootIndex.IndexManifest()
		if err != nil {
			return err
		}

		// attempt to find the first image reference in the layout...
		// for a manifest list only search one level deep.
		var img v1.Image
	loop:
		for _, descriptor := range rootIndexManifest.Manifests {

			if descriptor.MediaType.IsIndex() {
				// follow the descriptor using its digest to get the referenced index and its manifest
				childIndex, err := rootIndex.ImageIndex(descriptor.Digest)
				if err != nil {
					return err
				}
				childIndexManifest, err := childIndex.IndexManifest()
				if err != nil {
					return err
				}

				// at this point, find the first image and store it for later if possible
				for _, childDescriptor := range childIndexManifest.Manifests {
					if childDescriptor.MediaType.IsImage() {
						img, err = childIndex.Image(childDescriptor.Digest)
						if err != nil {
							return err
						}
						// no further processing necessary
						break loop
					}
				}

			} else if descriptor.MediaType.IsImage() {
				// this is a direct reference to an image, so just store it for later
				img, err = rootIndex.Image(descriptor.Digest)
				if err != nil {
					return err
				}
				// no further processing necessary
				break loop
			}
		}
		// if we get here and no image was found bail out
		if img == nil {
			return fmt.Errorf("unable to obtain image for %s", operator.Catalog)
		}
		// fullArtifactPath is set to <current working directory>/olm_artifacts/<repo>/<config folder>
		fullArtifactPath, err := extractDeclarativeConfigFromImage(img, catalogContentsDir)
		if err != nil {
			return err
		}

		// store the full artifact path for later so we don't have to recalculate the path.
		o.operatorCatalogToFullArtifactPath[operator.Catalog] = fullArtifactPath
	}
	return nil
}
