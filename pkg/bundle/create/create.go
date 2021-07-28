package create

import (
	"github.com/sirupsen/logrus"

	"github.com/RedHatGov/bundle/pkg/bundle"
	"github.com/RedHatGov/bundle/pkg/config"
)

// CreateFull performs all tasks in creating full imagesets
func CreateFull(rootDir string) error {
	err := bundle.MakeCreateDirs(rootDir)
	if err != nil {
		logrus.Error(err)
		return err
	}
	// Open Metadata
	metadata, err := config.LoadMetadata(rootDir)
	if err != nil {
		logrus.Error(err)
		return err
	}
	lastRun := metadata.PastMirrors[len(metadata.PastMirrors)-1]
	logrus.Info(lastRun)

	// Read the imageset-config.yaml
	cfg, err := config.LoadConfig(rootDir)
	if err != nil {
		logrus.Error(err)
		return err
	}
	logrus.Info(cfg)

	if len(cfg.Mirror.OCP.Channels) != 0 {
		if err := bundle.GetReleases(&lastRun, cfg, rootDir); err != nil {
			return err
		}
	}

	/*if &config.Mirror.Operators != nil {
	//GetOperators(*config, rootDir)
	//}
	//if &config.Mirror.Samples != nil {
	//GetSamples(*config, rootDir)
	//}*/

	if len(cfg.Mirror.AdditionalImages) != 0 {
		if err := bundle.GetAdditional(cfg, rootDir); err != nil {
			return err
		}
	}

	return nil
}

// CreateDiff performs all tasks in creating differential imagesets
//func CreateDiff(rootDir string) error {
//    return err
//}

//func downloadObjects() {
//
//}
