package create

import (
	"context"
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
	"github.com/RedHatGov/bundle/pkg/operator"
)

// CreateFull performs all tasks in creating full imagesets
func CreateFull(configPath, rootDir, outputDir string, dryRun, insecure bool) error {

	ctx := context.Background()

	sourceDir := filepath.Join(rootDir, config.SourcePath)
	metadataPath := filepath.Join(sourceDir, "publish", config.MetadataFile)

	if err := bundle.MakeCreateDirs(rootDir); err != nil {
		return err
	}

	// Read in current metadata
	metadata, err := config.LoadMetadata(rootDir)
	if err != nil {
		return err
	}

	// If metdata is found throw an error, else create new metadata
	if len(metadata.PastMirrors) != 0 {
		return config.NewFullMetadataError(metadataPath)
	}

	lastRun := v1alpha1.PastMirror{
		Sequence: 1,
		Uid:      uuid.New(),
	}

	// Set timestamp
	lastRun.Timestamp = int(time.Now().Unix())

	// Read the imageset-config.yaml
	cfg, err := config.LoadConfig(configPath)

	if err != nil {
		return err
	}

	if len(cfg.Mirror.OCP.Channels) != 0 {

		if err = bundle.GetReleases(&lastRun, cfg, sourceDir); err != nil {
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

		if err = bundle.GetAdditional(&lastRun, cfg, sourceDir); err != nil {
			return err
		}
	}

	// Add mirror as a new PastMirror
	metadata.PastMirrors = append(metadata.PastMirrors, lastRun)

	// Update metadata files
	files, err := getFiles(sourceDir, &metadata)

	if err != nil {
		return fmt.Errorf("error updates metadata files: %v", err)
	}

	// Write updated metadata
	if err := config.WriteMetadata(metadata, rootDir); err != nil {
		return fmt.Errorf("error writing metadata: %v", err)
	}

	var segSize int64

	if cfg.ImageSetConfigurationSpec.ArchiveSize != 0 {
		segSize = cfg.ImageSetConfigurationSpec.ArchiveSize * 1024 * 1024 * 1024
	} else {
		segSize = 500 * 1024 * 1024 * 1024
	}

	// Run archiver
	if err := prepareArchive(sourceDir, outputDir, segSize, &metadata, files); err != nil {
		return err
	}

	return nil
}

// CreateDiff performs all tasks in creating differential imagesets
func CreateDiff(configPath, rootDir, outputDir string, dryRun, insecure bool) error {

	_ = context.Background()

	sourceDir := filepath.Join(rootDir, config.SourcePath)
	metadataPath := filepath.Join(sourceDir, "publish", config.MetadataFile)

	if err := bundle.MakeCreateDirs(rootDir); err != nil {
		return err
	}

	// Read in current metadata
	metadata, err := config.LoadMetadata(rootDir)
	if err != nil {
		return err
	}

	// If metdata is not found throw an error, else set lastRun
	if len(metadata.PastMirrors) == 0 {
		return config.NewDiffMetadataError(metadataPath)
	}

	lastRun := metadata.PastMirrors[len(metadata.PastMirrors)-1]

	// Set timestamp
	lastRun.Timestamp = int(time.Now().Unix())

	// Read the imageset-config.yaml
	cfg, err := config.LoadConfig(configPath)

	if err != nil {
		return err
	}

	if len(cfg.Mirror.OCP.Channels) != 0 {

		if err = bundle.GetReleases(&lastRun, cfg, sourceDir); err != nil {
			return err
		}
	}

	if len(cfg.Mirror.Operators) != 0 {
		logrus.Debugf("operator catalog image diff not implemented")
	}

	if len(cfg.Mirror.Samples) != 0 {
		logrus.Debugf("sample images diff not implemented")
	}

	if len(cfg.Mirror.AdditionalImages) != 0 {

		if err = bundle.GetAdditional(&lastRun, cfg, sourceDir); err != nil {
			return err
		}
	}

	if err != nil {
		return err
	}

	// Add mirror as a new PastMirror
	lastRun.Sequence++
	metadata.PastMirrors = append(metadata.PastMirrors, lastRun)

	// Update metadata files
	files, err := getFiles(sourceDir, &metadata)

	if err != nil {
		return fmt.Errorf("error updates metadata files: %v", err)
	}

	// Write updated metadata
	if err := config.WriteMetadata(metadata, rootDir); err != nil {
		return fmt.Errorf("error writing metadata: %v", err)
	}

	var segSize int64

	if cfg.ImageSetConfigurationSpec.ArchiveSize != 0 {
		segSize = cfg.ImageSetConfigurationSpec.ArchiveSize * 1024 * 1024
	} else {
		segSize = 1024 * 1024 * 1024
	}

	// Run archiver
	if err := prepareArchive(sourceDir, outputDir, segSize, &metadata, files); err != nil {
		return err
	}

	return nil
}

func prepareArchive(rootDir, outputDir string, segSize int64, metadata *v1alpha1.Metadata, files []string) error {

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

func getFiles(rootDir string, metadata *v1alpha1.Metadata) ([]string, error) {

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
	return bundle.ReconcileFiles(&metadata.MetadataSpec, ".")
}
