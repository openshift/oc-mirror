package mirror

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

	"github.com/openshift/oc-mirror/pkg/archive"
	"github.com/openshift/oc-mirror/pkg/bundle"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

func (o *MirrorOptions) Create(ctx context.Context, flags *pflag.FlagSet) error {

	// Read the imageset-config.yaml
	cfg, err := config.LoadConfig(o.ConfigPath)
	if err != nil {
		return err
	}

	// Determine stateless or stateful mode.
	// Empty storage configuration will trigger a metadata cleanup
	// action and labels metadata as single use
	var backend storage.Backend
	var meta v1alpha1.Metadata
	path := filepath.Join(o.Dir, config.SourceDir)
	if (v1alpha1.StorageConfig{} == cfg.StorageConfig) {
		meta.SingleUse = true
		logrus.Warnf("backend is not configured in %s, using stateless mode", o.ConfigPath)
		cfg.StorageConfig.Local = &v1alpha1.LocalConfig{Path: path}
		backend, err = storage.ByConfig(path, cfg.StorageConfig)
		if err != nil {
			return fmt.Errorf("error opening backend: %v", err)
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
			return fmt.Errorf("error opening backend: %v", err)
		}
	}

	// Run full or diff mirror.
	merr := backend.ReadMetadata(ctx, &meta, config.MetadataBasePath)
	if merr != nil && !errors.Is(merr, storage.ErrMetadataNotExist) {
		return merr
	}

	if err := bundle.MakeCreateDirs(o.Dir); err != nil {
		return err
	}
	if !o.SkipCleanup {
		defer func() {
			if err := os.RemoveAll(filepath.Join(o.Dir, config.SourceDir)); err != nil {
				logrus.Error(err)
			}
		}()
	}

	// Ensure meta has the latest OPM image, and if not add it to cfg for mirroring.
	addOPMImage(&cfg, meta)

	thisRun := v1alpha1.PastMirror{
		Timestamp: int(time.Now().Unix()),
	}
	// New metadata files get a full mirror, with complete/heads-only catalogs, release images,
	// and a new UUID. Otherwise, use data from the last mirror to mirror just the layer diff.

	var backupMeta v1alpha1.Metadata
	rollbackMeta := func() error {
		logrus.Error("operation cancelled, initiating metadata rollback")
		return metadata.UpdateMetadata(ctx, backend, &backupMeta, o.SourceSkipTLS)
	}

	var assocs image.AssociationSet
	switch {
	case merr != nil || len(meta.PastMirrors) == 0:
		meta.Uid = uuid.New()
		thisRun.Sequence = 1

		assocs, err = o.createFull(ctx, flags, &cfg, meta)
		if err != nil {
			return err
		}

	default:
		backupMeta = meta
		lastRun := meta.PastMirrors[len(meta.PastMirrors)-1]
		thisRun.Sequence = lastRun.Sequence + 1

		assocs, err = o.createDiff(ctx, flags, &cfg, lastRun, meta)
		if err != nil {
			return err
		}
	}

	// Stop the process if DryRun
	if o.DryRun {
		return nil
	}

	if err := o.writeAssociations(assocs); err != nil {
		return fmt.Errorf("error writing association file: %v", err)
	}

	// Store mirror in the run
	thisRun.Mirror = cfg.Mirror

	// Update metadata files and get newly created filepaths.
	manifests, blobs, err := o.getFiles(meta)
	if err != nil {
		return err
	}
	// Add only the new manifests and blobs created to the current run.
	thisRun.Manifests = append(thisRun.Manifests, manifests...)
	thisRun.Blobs = append(thisRun.Blobs, blobs...)
	// Add this run and metadata to top level metadata.
	meta.PastMirrors = append(meta.PastMirrors, thisRun)
	meta.PastBlobs = append(meta.PastBlobs, blobs...)

	// Update the metadata.
	if err = metadata.UpdateMetadata(ctx, backend, &meta, o.SourceSkipTLS); err != nil {
		return err
	}

	// Allow for user interrupts
	ctx, done := o.CancelContext(ctx)
	defer done()

	// If any errors occur after the metadata is written
	// initiate metadata rollback
	if err := o.prepareArchive(ctx, cfg, backend, thisRun.Sequence, manifests, blobs); err != nil {
		if err := rollbackMeta(); err != nil {
			return fmt.Errorf("error occurred during metadata rollback: %v", err)
		}
		return err
	}

	if ctx.Err() == context.Canceled {
		o.interrupted = true
		return rollbackMeta()
	}

	/* Commenting out temporarily because no concrete types implement this
	if committer, isCommitter := backend.(storage.Committer); isCommitter {
		if err := committer.Commit(ctx); err != nil {
			return err
		}
	}*/

	return nil
}

// createFull performs all tasks in creating full imagesets
func (o *MirrorOptions) createFull(ctx context.Context, flags *pflag.FlagSet, cfg *v1alpha1.ImageSetConfiguration, meta v1alpha1.Metadata) (image.AssociationSet, error) {

	allAssocs := image.AssociationSet{}

	if len(cfg.Mirror.OCP.Channels) != 0 {
		opts := NewReleaseOptions(o, flags)
		assocs, err := opts.GetReleases(ctx, meta, cfg)
		if err != nil {
			return allAssocs, err
		}
		allAssocs.Merge(assocs)
	}

	if len(cfg.Mirror.Operators) != 0 {
		opts := NewOperatorOptions(o)
		opts.SkipImagePin = o.SkipImagePin
		assocs, err := opts.Full(ctx, *cfg)
		if err != nil {
			return allAssocs, err
		}
		allAssocs.Merge(assocs)
	}

	if len(cfg.Mirror.Samples) != 0 {
		logrus.Debugf("sample images full not implemented")
	}

	if len(cfg.Mirror.AdditionalImages) != 0 {
		opts := NewAdditionalOptions(o)
		assocs, err := opts.GetAdditional(*cfg, cfg.Mirror.AdditionalImages)
		if err != nil {
			return allAssocs, err
		}
		allAssocs.Merge(assocs)
	}

	if len(cfg.Mirror.Helm.Local) != 0 || len(cfg.Mirror.Helm.Repos) != 0 {
		opts := NewHelmOptions(o)
		assocs, err := opts.PullCharts(*cfg)
		if err != nil {
			return allAssocs, err
		}
		allAssocs.Merge(assocs)
	}

	return allAssocs, nil
}

// createDiff performs all tasks in creating differential imagesets
func (o *MirrorOptions) createDiff(ctx context.Context, flags *pflag.FlagSet, cfg *v1alpha1.ImageSetConfiguration, lastRun v1alpha1.PastMirror, meta v1alpha1.Metadata) (image.AssociationSet, error) {

	allAssocs := image.AssociationSet{}

	if len(cfg.Mirror.OCP.Channels) != 0 {
		opts := NewReleaseOptions(o, flags)
		assocs, err := opts.GetReleases(ctx, meta, cfg)
		if err != nil {
			return allAssocs, err
		}
		allAssocs.Merge(assocs)
	}

	if len(cfg.Mirror.Operators) != 0 {
		opts := NewOperatorOptions(o)
		opts.SkipImagePin = o.SkipImagePin
		assocs, err := opts.Diff(ctx, *cfg, lastRun)
		if err != nil {
			return allAssocs, err
		}
		allAssocs.Merge(assocs)
	}

	if len(cfg.Mirror.Samples) != 0 {
		logrus.Debugf("sample images diff not implemented")
	}

	if len(cfg.Mirror.AdditionalImages) != 0 {
		opts := NewAdditionalOptions(o)
		assocs, err := opts.GetAdditional(*cfg, cfg.Mirror.AdditionalImages)
		if err != nil {
			return allAssocs, err
		}
		allAssocs.Merge(assocs)
	}

	if len(cfg.Mirror.Helm.Local) != 0 || len(cfg.Mirror.Helm.Repos) != 0 {
		opts := NewHelmOptions(o)
		assocs, err := opts.PullCharts(*cfg)
		if err != nil {
			return allAssocs, err
		}
		allAssocs.Merge(assocs)
	}

	return allAssocs, nil
}

func (o *MirrorOptions) prepareArchive(ctx context.Context, cfg v1alpha1.ImageSetConfiguration, backend storage.Backend, seq int, manifests []v1alpha1.Manifest, blobs []v1alpha1.Blob) error {

	// Default to a 500GiB archive size.
	var segSize int64 = 500
	if cfg.ImageSetConfigurationSpec.ArchiveSize != 0 {
		segSize = cfg.ImageSetConfigurationSpec.ArchiveSize
		logrus.Debugf("Using user provider archive size %d GiB", segSize)
	}
	segSize *= 1024 * 1024 * 1024

	// Set get absolute path to output dir
	// to avoid issue with directory change
	output, err := filepath.Abs(o.OutputDir)
	if err != nil {
		return err
	}

	// Change directory before archiving to
	// avoid broken symlink paths
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(filepath.Join(o.Dir, config.SourceDir)); err != nil {
		return err
	}
	defer os.Chdir(cwd)

	packager := archive.NewPackager(manifests, blobs)
	prefix := fmt.Sprintf("mirror_seq%d", seq)
	if err := packager.CreateSplitArchive(ctx, backend, segSize, output, ".", prefix, o.SkipCleanup); err != nil {
		return fmt.Errorf("failed to create archive: %v", err)
	}

	return nil
}

func (o *MirrorOptions) getFiles(meta v1alpha1.Metadata) ([]v1alpha1.Manifest, []v1alpha1.Blob, error) {
	diskPath := filepath.Join(o.Dir, config.SourceDir, "v2")
	// Define a map that associates locations
	// on disk to location in archive
	paths := map[string]string{diskPath: "v2"}
	return bundle.ReconcileV2Dir(meta, paths)
}

func (o *MirrorOptions) writeAssociations(assocs image.AssociationSet) error {
	assocPath := filepath.Join(o.Dir, config.SourceDir, config.AssociationsBasePath)
	if err := os.MkdirAll(filepath.Dir(assocPath), 0755); err != nil {
		return fmt.Errorf("mkdir image associations file: %v", err)
	}
	f, err := os.OpenFile(assocPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		return fmt.Errorf("open image associations file: %v", err)
	}
	defer f.Close()
	return assocs.Encode(f)
}

// Make sure the latest `opm` image exists during the publishing step
// in case it does not exist in a past mirror.
func addOPMImage(cfg *v1alpha1.ImageSetConfiguration, meta v1alpha1.Metadata) {
	for _, pm := range meta.PastMirrors {
		for _, img := range pm.Mirror.AdditionalImages {
			if img.Image.Name == OPMImage {
				return
			}
		}
	}

	cfg.Mirror.AdditionalImages = append(cfg.Mirror.AdditionalImages, v1alpha1.AdditionalImages{
		Image: v1alpha1.Image{Name: OPMImage},
	})
	return
}
