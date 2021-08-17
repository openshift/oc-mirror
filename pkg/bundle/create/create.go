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
func CreateFull(configPath, rootDir, outputDir string, dryRun, insecure bool) error {

	ctx := context.Background()

	sourceDir := filepath.Join(rootDir, config.SourceDir)

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
		opts.RootDestDir = sourceDir
		opts.DryRun = dryRun
		opts.SkipTLS = insecure
		if err = opts.GetReleasesInitial(cfg); err != nil {
			return err
		}
	}

	if len(cfg.Mirror.Operators) != 0 {
		opts := operator.NewOperatorOptions()
		opts.RootDestDir = rootDir
		opts.DryRun = dryRun
		opts.SkipTLS = insecure
		if err := opts.Full(ctx, cfg); err != nil {
			return err
		}
	}

	if len(cfg.Mirror.Samples) != 0 {
		logrus.Debugf("sample images full not implemented")
	}

	if len(cfg.Mirror.AdditionalImages) != 0 {

		opts := bundle.NewAdditionalOptions()
		opts.DestDir = rootDir
		opts.DryRun = dryRun
		opts.SkipTLS = insecure
		if err := opts.GetAdditional(run, cfg); err != nil {
			return err
		}

	}

	// Update metadata files
	files, err := getFiles(sourceDir, &meta)
	if err != nil {
		return err
	}

	// Add mirror as a new PastMirror
	run.Mirror = cfg.Mirror
	meta.PastMirrors = append(meta.PastMirrors, run)

	// Update the metadata.
	if err = metadata.UpdateMetadata(ctx, backend, &meta, rootDir, insecure); err != nil {
		return err
	}

	var segSize int64

	if cfg.ImageSetConfigurationSpec.ArchiveSize != 0 {
		segSize = cfg.ImageSetConfigurationSpec.ArchiveSize * 1024 * 1024 * 1024
	} else {
		segSize = 500 * 1024 * 1024 * 1024
	}

	// Run archiver
	if err := prepareArchive(sourceDir, outputDir, segSize, files); err != nil {
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
func CreateDiff(configPath, rootDir, outputDir string, dryRun, insecure bool) error {

	ctx := context.Background()

	sourceDir := filepath.Join(rootDir, config.SourceDir)

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
		opts.RootDestDir = sourceDir
		opts.DryRun = dryRun
		opts.SkipTLS = insecure
		if err = opts.GetReleasesDiff(lastRun, cfg); err != nil {
			return err
		}
	}

	if len(cfg.Mirror.Operators) != 0 {
		opts := operator.NewOperatorOptions()
		opts.RootDestDir = rootDir
		opts.DryRun = dryRun
		opts.SkipTLS = insecure
		if err := opts.Diff(ctx, cfg, lastRun); err != nil {
			return err
		}
	}

	if len(cfg.Mirror.Samples) != 0 {
		logrus.Debugf("sample images diff not implemented")
	}

	if len(cfg.Mirror.AdditionalImages) != 0 {
		opts := bundle.NewAdditionalOptions()
		opts.DestDir = rootDir
		opts.DryRun = dryRun
		opts.SkipTLS = insecure
		if err := opts.GetAdditional(lastRun, cfg); err != nil {
			return err
		}
	}

	// Update metadata files
	files, err := getFiles(sourceDir, &meta)
	if err != nil {
		return err
	}

	// Add mirror as a new PastMirror
	run.Mirror = cfg.Mirror
	meta.PastMirrors = append(meta.PastMirrors, run)

	// Update the metadata.
	if err = metadata.UpdateMetadata(ctx, backend, &meta, rootDir, insecure); err != nil {
		return err
	}

	var segSize int64

	if cfg.ImageSetConfigurationSpec.ArchiveSize != 0 {
		segSize = cfg.ImageSetConfigurationSpec.ArchiveSize * 1024 * 1024
	} else {
		segSize = 1024 * 1024 * 1024
	}

	// Run archiver
	if err := prepareArchive(sourceDir, outputDir, segSize, files); err != nil {
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

func prepareArchive(rootDir, outputDir string, segSize int64, files []string) error {

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
