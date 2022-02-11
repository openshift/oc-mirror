package mirror

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

// Create will plan a mirroring operation based on provided configuration
func (o *MirrorOptions) Create(ctx context.Context, cfg v1alpha1.ImageSetConfiguration) (v1alpha1.Metadata, image.TypedImageMapping, error) {
	// Determine stateless or stateful mode.
	// Empty storage configuration will trigger a metadata cleanup
	// action and labels metadata as single use
	path := filepath.Join(o.Dir, config.SourceDir)
	meta := v1alpha1.NewMetadata()
	var backend storage.Backend
	var err error
	if !cfg.StorageConfig.IsSet() {
		meta.SingleUse = true
		logrus.Warnf("backend is not configured in %s, using stateless mode", o.ConfigPath)
		cfg.StorageConfig.Local = &v1alpha1.LocalConfig{Path: path}
		backend, err = storage.ByConfig(path, cfg.StorageConfig)
		if err != nil {
			return meta, image.TypedImageMapping{}, fmt.Errorf("error opening backend: %v", err)
		}
		defer func() {
			if err := backend.Cleanup(ctx, config.MetadataBasePath); err != nil {
				logrus.Error(err)
			}
		}()
	} else {
		meta.SingleUse = false
		backend, err = storage.ByConfig(path, cfg.StorageConfig)
		if err != nil {
			return meta, image.TypedImageMapping{}, fmt.Errorf("error opening backend: %v", err)
		}
	}
	thisRun := v1alpha1.PastMirror{
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
		logrus.Info("No metadata detected, creating new workspace")
		meta.Uid = uuid.New()
		thisRun.Sequence = 1
		thisRun.Mirror = cfg.Mirror
		f := func(ctx context.Context, cfg v1alpha1.ImageSetConfiguration) (image.TypedImageMapping, error) {
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
		f := func(ctx context.Context, cfg v1alpha1.ImageSetConfiguration) (image.TypedImageMapping, error) {
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

func (o *MirrorOptions) run(ctx context.Context, cfg *v1alpha1.ImageSetConfiguration, meta v1alpha1.Metadata, operatorPlan operatorFunc) (image.TypedImageMapping, error) {

	// Ensure meta has the latest OPM image, and if not add it to cfg for mirroring.
	addOPMImage(cfg, meta)
	mmappings := image.TypedImageMapping{}

	if len(cfg.Mirror.OCP.Channels) != 0 {
		release := NewReleaseOptions(o)
		mappings, err := release.Plan(ctx, meta.PastMirror, cfg)
		if err != nil {
			return mmappings, err
		}
		mmappings.Merge(mappings)
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

	if len(cfg.Mirror.Helm.Local) != 0 || len(cfg.Mirror.Helm.Repos) != 0 {
		helm := NewHelmOptions(o)
		mappings, err := helm.PullCharts(ctx, *cfg)
		if err != nil {
			return mmappings, err
		}
		mmappings.Merge(mappings)
	}

	if len(cfg.Mirror.Samples) != 0 {
		logrus.Debugf("sample images full not implemented")
	}

	return mmappings, nil
}

type operatorFunc func(ctx context.Context, cfg v1alpha1.ImageSetConfiguration) (image.TypedImageMapping, error)

// Make sure the latest `opm` image exists during the publishing step
// in case it does not exist in a past mirror.
func addOPMImage(cfg *v1alpha1.ImageSetConfiguration, meta v1alpha1.Metadata) {

	for _, img := range meta.PastMirror.Mirror.AdditionalImages {
		if img.Image.Name == OPMImage {
			return
		}
	}

	cfg.Mirror.AdditionalImages = append(cfg.Mirror.AdditionalImages, v1alpha1.AdditionalImages{
		Image: v1alpha1.Image{Name: OPMImage},
	})
	return
}
