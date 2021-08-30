package bundle

import (
	"os"
	"path/filepath"

	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/sirupsen/logrus"
)

// Initialize and validate directories for create and publish tasks

//func validatePublishDir() {
//	// check for expected directory structure
//	// check for expected metadata locations
//	// check for complete metadata
//
//}
/*
func ValidateCreateDir(rootDir string) error {
	// check for declared imageset root directory
	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
		//return false, err
		logrus.Infoln("referenced directory does not exist")
		return err
	} else {
		//return true, err
		logrus.Infoln("referenced directory found")
		return err
	}
	// check for expected metadata locations
	// check for complete metadata
}
*/
func MakeCreateDirs(rootDir string) error {
	paths := []string{
		filepath.Join(config.SourceDir, config.PublishDir),
		filepath.Join(config.SourceDir, "v2"),
	}
	for _, p := range paths {
		dir := filepath.Join(rootDir, p)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			logrus.Infof("Creating directory: %v", dir)
			err := os.MkdirAll(dir, os.ModePerm)
			if err != nil {
				return err
			}
		} else {
			logrus.Infof("Found: %v", dir)
		}
	}
	return nil
}

//func makePublishDirs() {
//
//}
