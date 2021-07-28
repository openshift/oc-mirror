package create

import (
	bundle "github.com/RedHatGov/bundle/pkg/bundle"
	"github.com/sirupsen/logrus"
)

// CreateFull performs all tasks in creating full imagesets
func CreateFull(rootDir string) error {
	err := bundle.MakeCreateDirs(rootDir)
	if err != nil {
		logrus.Error(err)
		return err
	}
	// Open Metadata
	metadata, err := bundle.ReadMeta(rootDir)
	if err != nil {
		logrus.Error(err)
		return err
	}
	lastRun := metadata.Imagesets[len(metadata.Imagesets)-1]
	logrus.Info(lastRun)

	// Read the bundle-config.yaml
	config, err := bundle.ReadBundleConfig(rootDir)
	if err != nil {
		logrus.Error(err)
		return err
	}
	logrus.Info(config)
	if len(config.Mirror.Ocp.Channels) != 0 {
		bundle.GetReleases(&lastRun, config, rootDir)
	}
	/*if &config.Mirror.Operators != nil {
	//GetOperators(*config, rootDir)
	//}
	//if &config.Mirror.Samples != nil {
	//GetSamples(*config, rootDir)
	//}*/
	if len(config.Mirror.AdditionalImages) != 0 {
		bundle.GetAdditional(config, rootDir)
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
