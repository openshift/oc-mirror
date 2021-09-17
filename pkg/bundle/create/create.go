package create

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/RedHatGov/bundle/pkg/archive"
	"github.com/RedHatGov/bundle/pkg/bundle"
	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/RedHatGov/bundle/pkg/image"
	"github.com/RedHatGov/bundle/pkg/metadata"
	"github.com/RedHatGov/bundle/pkg/metadata/storage"
	"github.com/RedHatGov/bundle/pkg/operator"
)

// TODO: refactor into Complete() -> Validate() -> Run() CLI pattern (thing `oc`).

var (
	// FullMetadataError should be returned when past runs exist in metadata.
	ErrFullMetadata = errors.New("prior run metadata found, please run 'create diff' instead")

	// FullMetadataError should be returned when no past runs exist in metadata.
	ErrDiffMetadata = errors.New("no prior run metadata found, please run 'create full' instead")
)

// RunFull performs all tasks in creating full imagesets
func (o *Options) RunFull(ctx context.Context) error {
	f := func(ctx context.Context, cfg v1alpha1.ImageSetConfiguration, backend storage.Backend) (meta v1alpha1.Metadata, run v1alpha1.PastMirror, err error) {

		// Read in current metadata
		switch err := backend.ReadMetadata(ctx, &meta, config.MetadataBasePath); {
		case err != nil && !errors.Is(err, storage.ErrMetadataNotExist):
			return meta, run, err
		case err == nil && len(meta.PastMirrors) != 0:
			// No past run(s) are allowed when generating a full imageset.
			// QUESTION: should the command just log a warning and ignore this metadata instead?
			return meta, run, ErrFullMetadata
		}

		meta.Uid = uuid.New()
		run = v1alpha1.PastMirror{
			Sequence:  1,
			Timestamp: int(time.Now().Unix()),
		}

		allAssocs := image.AssociationSet{}

		if len(cfg.Mirror.OCP.Channels) != 0 {
			opts := bundle.NewReleaseOptions(*o.RootOptions)
			assocs, err := opts.GetReleasesInitial(cfg)
			if err != nil {
				return meta, run, err
			}
			allAssocs.Merge(assocs)
		}

		if len(cfg.Mirror.Operators) != 0 {
			opts := operator.NewMirrorOptions(*o.RootOptions)
			assocs, err := opts.Full(ctx, cfg)
			if err != nil {
				return meta, run, err
			}
			allAssocs.Merge(assocs)
		}

		if len(cfg.Mirror.Samples) != 0 {
			logrus.Debugf("sample images full not implemented")
		}

		if len(cfg.Mirror.AdditionalImages) != 0 {
			opts := bundle.NewAdditionalOptions(*o.RootOptions)
			assocs, err := opts.GetAdditional(run, cfg)
			if err != nil {
				return meta, run, err
			}
			allAssocs.Merge(assocs)
		}

		if err := o.writeAssociations(allAssocs); err != nil {
			return meta, run, fmt.Errorf("error writing association file: %v", err)
		}

		return meta, run, nil
	}

	return o.create(ctx, f)
}

// RunDiff performs all tasks in creating differential imagesets
func (o *Options) RunDiff(ctx context.Context) error {
	f := func(ctx context.Context, cfg v1alpha1.ImageSetConfiguration, backend storage.Backend) (meta v1alpha1.Metadata, run v1alpha1.PastMirror, err error) {

		// Read in current metadata
		switch err := backend.ReadMetadata(ctx, &meta, config.MetadataBasePath); {
		case err != nil && !errors.Is(err, storage.ErrMetadataNotExist):
			return meta, run, err
		case (err != nil && errors.Is(err, storage.ErrMetadataNotExist)) || len(meta.PastMirrors) == 0:
			// Some past run(s) are required to generate a diff.
			return meta, run, ErrDiffMetadata
		}

		lastRun := meta.PastMirrors[len(meta.PastMirrors)-1]
		run = v1alpha1.PastMirror{
			Sequence:  lastRun.Sequence + 1,
			Timestamp: int(time.Now().Unix()),
		}

		allAssocs := image.AssociationSet{}

		if len(cfg.Mirror.OCP.Channels) != 0 {
			opts := bundle.NewReleaseOptions(*o.RootOptions)
			assocs, err := opts.GetReleasesInitial(cfg)
			if err != nil {
				return meta, run, err
			}
			allAssocs.Merge(assocs)
		}

		if len(cfg.Mirror.Operators) != 0 {
			opts := operator.NewMirrorOptions(*o.RootOptions)
			opts.SkipImagePin = o.SkipImagePin
			assocs, err := opts.Diff(ctx, cfg, lastRun)
			if err != nil {
				return meta, run, err
			}
			allAssocs.Merge(assocs)
		}

		if len(cfg.Mirror.Samples) != 0 {
			logrus.Debugf("sample images diff not implemented")
		}

		if len(cfg.Mirror.AdditionalImages) != 0 {
			opts := bundle.NewAdditionalOptions(*o.RootOptions)
			assocs, err := opts.GetAdditional(lastRun, cfg)
			if err != nil {
				return meta, run, err
			}
			allAssocs.Merge(assocs)
		}

		if err := o.writeAssociations(allAssocs); err != nil {
			return meta, run, fmt.Errorf("error writing association file: %v", err)
		}

		return meta, run, nil
	}

	return o.create(ctx, f)
}

type createFunc func(context.Context, v1alpha1.ImageSetConfiguration, storage.Backend) (v1alpha1.Metadata, v1alpha1.PastMirror, error)

func (o *Options) create(ctx context.Context, f createFunc) error {

	// Read the imageset-config.yaml
	cfg, err := config.LoadConfig(o.ConfigPath)
	if err != nil {
		return err
	}

	// Make sure the `opm` image exists during the publish step
	// since catalog images need to be rebuilt.
	// TODO(estroz): vet that this is the correct image, and version correctly.
	cfg.Mirror.AdditionalImages = append(cfg.Mirror.AdditionalImages, v1alpha1.AdditionalImages{
		Image: v1alpha1.Image{Name: "quay.io/operator-framework/opm:latest"},
	})

	// Validating user path input
	if err := o.ValidatePaths(); err != nil {
		return err
	}

	logrus.Info("Verifying pull secrets")
	// Validating pull secrets
	if err := config.ValidateSecret(cfg); err != nil {
		return err
	}

	if err := bundle.MakeCreateDirs(o.Dir); err != nil {
		return err
	}

	// TODO: make backend configurable.
	backend, err := storage.NewLocalBackend(filepath.Join(o.Dir, config.SourceDir))
	if err != nil {
		return fmt.Errorf("error opening local backend: %v", err)
	}

	// Run full or diff mirror.
	meta, thisRun, err := f(ctx, cfg, backend)
	if err != nil {
		return err
	}

	// Update metadata files and get newly created filepaths.
	manifests, blobs, err := o.getFiles(meta)
	if err != nil {
		return err
	}
	// Store the config in the current run for reproducibility.
	thisRun.Mirror = cfg.Mirror
	// Add only the new manifests and blobs created to the current run.
	thisRun.Manifests = append(thisRun.Manifests, manifests...)
	thisRun.Blobs = append(thisRun.Blobs, blobs...)
	// Add this run and metadata to top level metadata.
	meta.PastMirrors = append(meta.PastMirrors, thisRun)
	meta.PastManifests = append(meta.PastManifests, manifests...)
	meta.PastBlobs = append(meta.PastBlobs, blobs...)

	// Update the metadata.
	if err = metadata.UpdateMetadata(ctx, backend, &meta, o.Dir, o.SkipTLS); err != nil {
		return err
	}

	// Run archiver
	if err := o.prepareArchive(cfg, manifests, blobs); err != nil {
		return err
	}

	// Handle Committer backends.
	if committer, isCommitter := backend.(storage.Committer); isCommitter {
		if err := committer.Commit(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (o *Options) prepareArchive(cfg v1alpha1.ImageSetConfiguration, manifests []v1alpha1.Manifest, blobs []v1alpha1.Blob) error {

	// Default to a 500GiB archive size.
	var segSize int64 = 500
	if cfg.ImageSetConfigurationSpec.ArchiveSize != 0 {
		segSize = cfg.ImageSetConfigurationSpec.ArchiveSize
		logrus.Debugf("Using user provider archive size %d GiB", segSize)
	}
	segSize *= 1024 * 1024 * 1024

	cwd, err := os.Getwd()

	if err != nil {
		return err
	}

	// Set get absolute path to output dir
	output, err := filepath.Abs(o.OutputDir)

	if err != nil {
		return err
	}

	// Change dir before archiving to avoid issues with symlink paths
	if err := os.Chdir(filepath.Join(o.Dir, config.SourceDir)); err != nil {
		return err
	}
	defer os.Chdir(cwd)

	packager := archive.NewPackager(manifests, blobs)

	// Create tar archive
	if err := packager.CreateSplitArchive(segSize, output, ".", "bundle", o.SkipCleanup); err != nil {
		return fmt.Errorf("failed to create archive: %v", err)
	}

	return nil

}

func (o *Options) getFiles(meta v1alpha1.Metadata) ([]v1alpha1.Manifest, []v1alpha1.Blob, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}

	// Change dir before archiving to avoid issues with symlink paths
	if err := os.Chdir(filepath.Join(o.Dir, config.SourceDir)); err != nil {
		return nil, nil, err
	}
	defer os.Chdir(cwd)

	// Gather manifests we pulled
	manifests, err := bundle.ReconcileManifests(meta, ".")

	if err != nil {
		return nil, nil, err
	}

	blobs, err := bundle.ReconcileBlobs(meta, ".")

	if err != nil {
		return nil, nil, err
	}

	return manifests, blobs, nil
}

func (o *Options) writeAssociations(assocs image.AssociationSet) error {
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
