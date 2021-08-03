package create

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/RedHatGov/bundle/pkg/bundle"
	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/RedHatGov/bundle/pkg/operator"
)

// CreateFull performs all tasks in creating full imagesets
func CreateFull(rootDir string, dryRun, insecure bool) error {
	ctx := context.Background()

	err := bundle.MakeCreateDirs(rootDir)
	if err != nil {
		return err
	}
	// Open Metadata
	metadata, err := config.LoadMetadata(rootDir)
	if err != nil {
		return err
	}

	// TODO: this isn't the best way to handle metadata vs. no metadata.
	// Needs refactoring.
	var lastRun v1alpha1.PastMirror
	if len(metadata.PastMirrors) != 0 {
		lastRun = metadata.PastMirrors[len(metadata.PastMirrors)-1]
		logrus.Debug(lastRun)
	} else {
		logrus.Debugf("no metadata found, creating full payload")
	}

	// Read the imageset-config.yaml
	cfg, err := config.LoadConfig(rootDir)
	if err != nil {
		return err
	}
	logrus.Info(cfg)

	if len(cfg.Mirror.OCP.Channels) != 0 {
		if err := bundle.GetReleases(&lastRun, cfg, rootDir); err != nil {
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
		if err := bundle.GetAdditional(cfg, rootDir); err != nil {
			return err
		}
	}

	return nil
}

// CreateDiff performs all tasks in creating differential imagesets
func CreateDiff(rootDir string) error {
	_ = context.Background()

	err := bundle.MakeCreateDirs(rootDir)
	if err != nil {
		return err
	}
	// Open Metadata
	metadata, err := config.LoadMetadata(rootDir)
	if err != nil {
		return err
	}

	// TODO: this isn't the best way to handle metadata vs. no metadata.
	// Needs refactoring.
	var lastRun v1alpha1.PastMirror
	if len(metadata.PastMirrors) != 0 {
		lastRun = metadata.PastMirrors[len(metadata.PastMirrors)-1]
		logrus.Debug(lastRun)
	} else {
		logrus.Debugf("no metadata found, creating diff payload")
	}

	// Read the imageset-config.yaml
	cfg, err := config.LoadConfig(rootDir)
	if err != nil {
		return err
	}
	logrus.Debug(cfg)

	if len(cfg.Mirror.OCP.Channels) != 0 {
		logrus.Debugf("release image diff not implemented")
	}

	if len(cfg.Mirror.Operators) != 0 {
		logrus.Debugf("operator catalog image diff not implemented")
	}

	if len(cfg.Mirror.Samples) != 0 {
		logrus.Debugf("sample images diff not implemented")
	}

	if len(cfg.Mirror.AdditionalImages) != 0 {
		logrus.Debugf("additional images diff not implemented")
	}

	return err
}
