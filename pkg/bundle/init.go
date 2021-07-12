package bundle

import (
	"os"

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
		"/bundle/publish",
		"/bundle/v2",
		"/src/publish",
		"/src/v2",
	}
	for _, p := range paths {
		if _, err := os.Stat(rootDir + p); os.IsNotExist(err) {
			err := os.MkdirAll(rootDir+p, os.ModePerm)
			if err != nil {
				logrus.Errorln(err)
			}
			logrus.Infof("Creating directory: %v", rootDir+p)
		} else {
			logrus.Infof("Found %v", rootDir+p)
		}
	}
	return nil
}

//func makePublishDirs() {
//
//}
