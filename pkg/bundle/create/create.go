package create

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/RedHatGov/bundle/pkg/bundle"
	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/RedHatGov/bundle/pkg/operator"
)

// CreateFull performs all tasks in creating full imagesets
func CreateFull(configPath, rootDir string, dryRun, insecure bool) error {

	ctx := context.Background()

	sourceDir := filepath.Join(rootDir, config.SourcePath)
	metadataPath := filepath.Join(sourceDir, "publish", config.MetadataFile)

	err := bundle.MakeCreateDirs(rootDir)
	if err != nil {
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

	// Gather files we pulled
	if err = bundle.ReconcileFiles(&metadata.MetadataSpec, rootDir); err != nil {
		return err
	}

	// Add mirror as a new PastMirror
	metadata.PastMirrors = append(metadata.PastMirrors, lastRun)
	if err := config.WriteMetadata(metadata, rootDir); err != nil {
		return fmt.Errorf("error writing metadata: %v", err)
	}

	return nil
}

// CreateDiff performs all tasks in creating differential imagesets
func CreateDiff(configPath, rootDir string) error {

	_ = context.Background()

	sourceDir := filepath.Join(rootDir, config.SourcePath)
	metadataPath := filepath.Join(sourceDir, "publish", config.MetadataFile)

	err := bundle.MakeCreateDirs(rootDir)
	if err != nil {
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

	// Gather files we pulled
	if err = bundle.ReconcileFiles(&metadata.MetadataSpec, rootDir); err != nil {
		return err
	}

	// Add mirror as a new PastMirror
	lastRun.Sequence++
	metadata.PastMirrors = append(metadata.PastMirrors, lastRun)
	if err := config.WriteMetadata(metadata, rootDir); err != nil {
		return fmt.Errorf("error writing metadata: %v", err)
	}

	return nil
}
