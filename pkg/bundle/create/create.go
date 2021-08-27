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

var (
	// FullMetadataError should be returned when past runs exist in metadata.
	ErrFullMetadata = errors.New("prior run metadata found, please run 'create diff' instead")

	// FullMetadataError should be returned when no past runs exist in metadata.
	ErrDiffMetadata = errors.New("no prior run metadata found, please run 'create full' instead")
)

type creator struct {
	configPath  string
	rootDir     string
	sourceDir   string
	outputDir   string
	dryRun      bool
	skipTLS     bool
	skipCleanup bool
}

// NewCreator creates a new creator to mirror and package ImageSets
func NewCreator(configPath, rootDir, outputDir string, dryRun, skipTLS, skipCleanup bool) creator {
	return creator{
		configPath:  configPath,
		rootDir:     rootDir,
		sourceDir:   filepath.Join(rootDir, config.SourceDir),
		outputDir:   outputDir,
		dryRun:      dryRun,
		skipTLS:     skipTLS,
		skipCleanup: skipCleanup,
	}
}

// CreateFull performs all tasks in creating full imagesets
func (c creator) CreateFull(ctx context.Context) error {

	if err := bundle.MakeCreateDirs(c.rootDir); err != nil {
		return err
	}

	if !c.skipCleanup {
		defer func() {
			if err := os.RemoveAll(filepath.Join(c.sourceDir, "v2")); err != nil {
				logrus.Fatal(err)
			}
		}()
	}

	defer func() {
		if err := os.RemoveAll(filepath.Join(c.sourceDir, "blobs")); err != nil {
			logrus.Fatal(err)
		}
	}()

	defer func() {
		if err := os.RemoveAll(filepath.Join(c.sourceDir, "manifests")); err != nil {
			logrus.Fatal(err)
		}
	}()

	// TODO: make backend configurable.
	backend, err := storage.NewLocalBackend(c.rootDir)
	if err != nil {
		return fmt.Errorf("error opening local backend: %v", err)
	}

	// Read in current metadata
	meta := v1alpha1.Metadata{}
	switch err := backend.ReadMetadata(ctx, &meta); {
	case err != nil && !errors.Is(err, storage.ErrMetadataNotExist):
		return err
	case err == nil && len(meta.PastMirrors) != 0:
		// No past run(s) are allowed when generating a full imageset.
		// QUESTION: should the command just log a warning and ignore this metadata instead?
		return ErrFullMetadata
	}

	meta.Uid = uuid.New()
	run := v1alpha1.PastMirror{
		Sequence:  1,
		Timestamp: int(time.Now().Unix()),
	}

	allAssocs := image.Associations{}

	// Read the imageset-config.yaml
	cfg, err := config.LoadConfig(c.configPath)

	if err != nil {
		return err
	}

	logrus.Info("Verifying pull secrets")
	// Validating pull secrets
	if err := config.ValidateSecret(cfg); err != nil {
		return err
	}

	if len(cfg.Mirror.OCP.Channels) != 0 {
		opts := bundle.NewReleaseOptions()
		opts.RootDestDir = c.rootDir
		opts.DryRun = c.dryRun
		opts.SkipTLS = c.skipTLS
		assocs, err := opts.GetReleasesInitial(cfg)
		if err != nil {
			return err
		}
		allAssocs.Merge(assocs)
	}

	if len(cfg.Mirror.Operators) != 0 {
		opts := operator.MirrorOptions{}
		opts.RootDestDir = c.rootDir
		opts.DryRun = c.dryRun
		opts.SkipTLS = c.skipTLS
		opts.SkipCleanup = c.skipCleanup
		assocs, err := opts.Full(ctx, cfg)
		if err != nil {
			return err
		}
		allAssocs.Merge(assocs)
	}

	if len(cfg.Mirror.Samples) != 0 {
		logrus.Debugf("sample images full not implemented")
	}

	if len(cfg.Mirror.AdditionalImages) != 0 {

		opts := bundle.NewAdditionalOptions()
		opts.DestDir = c.rootDir
		opts.DryRun = c.dryRun
		opts.SkipTLS = c.skipTLS
		assocs, err := opts.GetAdditional(run, cfg)
		if err != nil {
			return err
		}
		allAssocs.Merge(assocs)
	}

	if err := c.writeAssociations(allAssocs); err != nil {
		return fmt.Errorf("error writing association file: %v", err)
	}

	// Update metadata files
	if err := c.getFiles(&meta); err != nil {
		return err
	}

	// Add mirror as a new PastMirror
	run.Mirror = cfg.Mirror
	meta.PastMirrors = append(meta.PastMirrors, run)

	// Update the metadata.
	if err = metadata.UpdateMetadata(ctx, backend, &meta, c.rootDir, c.skipTLS); err != nil {
		return err
	}

	// Run archiver
	if err := c.prepareArchive(cfg); err != nil {
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

// CreateDiff performs all tasks in creating differential imagesets
func (c creator) CreateDiff(ctx context.Context) error {

	if err := bundle.MakeCreateDirs(c.rootDir); err != nil {
		return err
	}

	if !c.skipCleanup {
		defer func() {
			if err := os.RemoveAll(filepath.Join(c.sourceDir, "v2")); err != nil {
				logrus.Fatal(err)
			}
		}()
	}

	defer func() {
		if err := os.RemoveAll(filepath.Join(c.sourceDir, "blobs")); err != nil {
			logrus.Fatal(err)
		}
	}()

	defer func() {
		if err := os.RemoveAll(filepath.Join(c.sourceDir, "manifests")); err != nil {
			logrus.Fatal(err)
		}
	}()

	// TODO: make backend configurable.
	backend, err := storage.NewLocalBackend(c.rootDir)
	if err != nil {
		return fmt.Errorf("error opening local backend: %v", err)
	}

	// Read in current metadata
	meta := v1alpha1.Metadata{}
	switch err := backend.ReadMetadata(ctx, &meta); {
	case err != nil && !errors.Is(err, storage.ErrMetadataNotExist):
		return err
	case (err != nil && errors.Is(err, storage.ErrMetadataNotExist)) || len(meta.PastMirrors) == 0:
		// Some past run(s) are required to generate a diff.
		return ErrDiffMetadata
	}

	lastRun := meta.PastMirrors[len(meta.PastMirrors)-1]
	run := v1alpha1.PastMirror{
		Sequence:  lastRun.Sequence + 1,
		Timestamp: int(time.Now().Unix()),
	}

	allAssocs := image.Associations{}

	// Read the imageset-config.yaml
	cfg, err := config.LoadConfig(c.configPath)

	if err != nil {
		return err
	}

	logrus.Info("Verifying pull secrets")
	// Validating pull secrets
	if err := config.ValidateSecret(cfg); err != nil {
		return err
	}

	if len(cfg.Mirror.OCP.Channels) != 0 {
		opts := bundle.NewReleaseOptions()
		opts.RootDestDir = c.rootDir
		opts.DryRun = c.dryRun
		opts.SkipTLS = c.skipTLS
		assocs, err := opts.GetReleasesDiff(lastRun, cfg)
		if err != nil {
			return err
		}
		allAssocs.Merge(assocs)
	}

	if len(cfg.Mirror.Operators) != 0 {
		opts := operator.MirrorOptions{}
		opts.RootDestDir = c.rootDir
		opts.DryRun = c.dryRun
		opts.SkipTLS = c.skipTLS
		opts.SkipCleanup = c.skipCleanup
		assocs, err := opts.Diff(ctx, cfg, lastRun)
		if err != nil {
			return err
		}
		allAssocs.Merge(assocs)
	}

	if len(cfg.Mirror.Samples) != 0 {
		logrus.Debugf("sample images diff not implemented")
	}

	if len(cfg.Mirror.AdditionalImages) != 0 {
		opts := bundle.NewAdditionalOptions()
		opts.DestDir = c.rootDir
		opts.DryRun = c.dryRun
		opts.SkipTLS = c.skipTLS
		assocs, err := opts.GetAdditional(lastRun, cfg)
		if err != nil {
			return err
		}
		allAssocs.Merge(assocs)
	}

	if err := c.writeAssociations(allAssocs); err != nil {
		return fmt.Errorf("error writing association file: %v", err)
	}

	// Update metadata files
	if err := c.getFiles(&meta); err != nil {
		return err
	}

	// Add mirror as a new PastMirror
	run.Mirror = cfg.Mirror
	meta.PastMirrors = append(meta.PastMirrors, run)

	// Update the metadata.
	if err = metadata.UpdateMetadata(ctx, backend, &meta, c.rootDir, c.skipTLS); err != nil {
		return err
	}

	// Run archiver
	if err := c.prepareArchive(cfg); err != nil {
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

func (c creator) prepareArchive(cfg v1alpha1.ImageSetConfiguration) error {

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
	output, err := filepath.Abs(c.outputDir)

	if err != nil {
		return err
	}

	// Change dir before archiving to avoid issues with symlink paths
	if err := os.Chdir(c.sourceDir); err != nil {
		return err
	}
	defer os.Chdir(cwd)

	arc := archive.NewArchiver()

	// Create tar archive
	if err := archive.CreateSplitArchive(arc, segSize, output, ".", "bundle"); err != nil {
		return fmt.Errorf("failed to create archive: %v", err)
	}

	return nil

}

func (c creator) getFiles(meta *v1alpha1.Metadata) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Change dir before archiving to avoid issues with symlink paths
	if err := os.Chdir(c.sourceDir); err != nil {
		return err
	}
	defer os.Chdir(cwd)

	// Gather manifests we pulled
	if err := bundle.ReconcileManifests(meta, "."); err != nil {
		return err
	}

	if err := bundle.ReconcileBlobs(meta, "."); err != nil {
		return err
	}

	return nil
}

func (c creator) writeAssociations(assocs image.Associations) error {
	assocPath := filepath.Join(c.rootDir, config.AssociationsBasePath)
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
