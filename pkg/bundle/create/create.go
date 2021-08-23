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
	FullMetadataError = errors.New("prior run metadata found, please run 'create diff' instead")

	// FullMetadataError should be returned when no past runs exist in metadata.
	DiffMetadataError = errors.New("no prior run metadata found, please run 'create full' instead")
)

// CreateFull performs all tasks in creating full imagesets
func CreateFull(ctx context.Context, configPath, rootDir, outputDir string, dryRun, skipTLS, skipCleanup bool) error {

	if err := bundle.MakeCreateDirs(rootDir); err != nil {
		return err
	}

	// TODO: make backend configurable.
	backend, err := storage.NewLocalBackend(rootDir)
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
		return FullMetadataError
	}

	meta.Uid = uuid.New()
	run := v1alpha1.PastMirror{
		Sequence:  1,
		Timestamp: int(time.Now().Unix()),
	}

	allAssocs := image.Associations{}

	// Read the imageset-config.yaml
	cfg, err := config.LoadConfig(configPath)

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
		opts.RootDestDir = rootDir
		opts.DryRun = dryRun
		opts.SkipTLS = skipTLS
		assocs, err := opts.GetReleasesInitial(cfg)
		if err != nil {
			return err
		}
		allAssocs.Merge(assocs)
	}

	if len(cfg.Mirror.Operators) != 0 {
		opts := operator.MirrorOptions{}
		opts.RootDestDir = rootDir
		opts.DryRun = dryRun
		opts.SkipTLS = skipTLS
		opts.SkipCleanup = skipCleanup
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
		opts.DestDir = rootDir
		opts.DryRun = dryRun
		opts.SkipTLS = skipTLS
		assocs, err := opts.GetAdditional(run, cfg)
		if err != nil {
			return err
		}
		allAssocs.Merge(assocs)
	}

	if err := writeAssociations(rootDir, allAssocs); err != nil {
		return fmt.Errorf("error writing association file: %v", err)
	}

	// Update metadata files
	sourceDir := filepath.Join(rootDir, config.SourceDir)
	files, err := getFiles(sourceDir, &meta)
	if err != nil {
		return err
	}

	// Add mirror as a new PastMirror
	run.Mirror = cfg.Mirror
	meta.PastMirrors = append(meta.PastMirrors, run)

	// Update the metadata.
	if err = metadata.UpdateMetadata(ctx, backend, &meta, rootDir, skipTLS); err != nil {
		return err
	}

	// Run archiver
	if err := prepareArchive(cfg, sourceDir, outputDir, files); err != nil {
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
func CreateDiff(ctx context.Context, configPath, rootDir, outputDir string, dryRun, skipTLS, skipCleanup bool) error {

	if err := bundle.MakeCreateDirs(rootDir); err != nil {
		return err
	}

	// TODO: make backend configurable.
	backend, err := storage.NewLocalBackend(rootDir)
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
		return DiffMetadataError
	}

	lastRun := meta.PastMirrors[len(meta.PastMirrors)-1]
	run := v1alpha1.PastMirror{
		Sequence:  lastRun.Sequence + 1,
		Timestamp: int(time.Now().Unix()),
	}

	allAssocs := image.Associations{}

	// Read the imageset-config.yaml
	cfg, err := config.LoadConfig(configPath)

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
		opts.RootDestDir = rootDir
		opts.DryRun = dryRun
		opts.SkipTLS = skipTLS
		assocs, err := opts.GetReleasesDiff(lastRun, cfg)
		if err != nil {
			return err
		}
		allAssocs.Merge(assocs)
	}

	if len(cfg.Mirror.Operators) != 0 {
		opts := operator.MirrorOptions{}
		opts.RootDestDir = rootDir
		opts.DryRun = dryRun
		opts.SkipTLS = skipTLS
		opts.SkipCleanup = skipCleanup
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
		opts.DestDir = rootDir
		opts.DryRun = dryRun
		opts.SkipTLS = skipTLS
		assocs, err := opts.GetAdditional(lastRun, cfg)
		if err != nil {
			return err
		}
		allAssocs.Merge(assocs)
	}

	if err := writeAssociations(rootDir, allAssocs); err != nil {
		return fmt.Errorf("error writing association file: %v", err)
	}

	// Update metadata files
	sourceDir := filepath.Join(rootDir, config.SourceDir)
	files, err := getFiles(sourceDir, &meta)
	if err != nil {
		return err
	}

	// Add mirror as a new PastMirror
	run.Mirror = cfg.Mirror
	meta.PastMirrors = append(meta.PastMirrors, run)

	// Update the metadata.
	if err = metadata.UpdateMetadata(ctx, backend, &meta, rootDir, skipTLS); err != nil {
		return err
	}

	// Run archiver
	if err := prepareArchive(cfg, sourceDir, outputDir, files); err != nil {
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

func prepareArchive(cfg v1alpha1.ImageSetConfiguration, rootDir, outputDir string, files []string) error {

	// Default to a 500GiB archive size.
	var segSize int64 = 500
	if cfg.ImageSetConfigurationSpec.ArchiveSize != 0 {
		segSize = cfg.ImageSetConfigurationSpec.ArchiveSize
	}
	segSize *= 1024 * 1024 * 1024

	cwd, err := os.Getwd()

	if err != nil {
		return err
	}

	// Create archiver
	arc := archive.NewArchiver()

	// Set get absolute path to output dir
	output, err := filepath.Abs(outputDir)

	if err != nil {
		return err
	}

	// Change dir before archiving to avoid issues with symlink paths
	if err := os.Chdir(rootDir); err != nil {
		return err
	}
	defer os.Chdir(cwd)

	// Create tar archive
	if err := archive.CreateSplitArchive(arc, segSize, output, ".", "bundle", files); err != nil {
		return fmt.Errorf("failed to create archive: %v", err)
	}

	return nil

}

func getFiles(rootDir string, meta *v1alpha1.Metadata) ([]string, error) {

	cwd, err := os.Getwd()

	if err != nil {
		return nil, err
	}

	// Change dir before archiving to avoid issues with symlink paths
	if err := os.Chdir(rootDir); err != nil {
		return nil, err
	}
	defer os.Chdir(cwd)

	// Gather files we pulled
	return bundle.ReconcileFiles(meta, ".")
}

func writeAssociations(rootDir string, assocs image.Associations) error {
	assocPath := filepath.Join(rootDir, config.AssociationsBasePath)
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
