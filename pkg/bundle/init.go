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

func validateCreateDir(rootDir string) (bool, error) {
	// check for declared imageset root directory
	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
		return false, err

	} else {
		return true, err
	}
	// check for expected metadata locations
	// check for complete metadata

}

func makeCreateDirs(rootDir string) error {
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
			logrus.Infoln("Creating directory: %v", rootDir+p)
		} else {
			logrus.Infoln("Found %v", rootDir+p)
		}
	}
	return nil
}

//func makePublishDirs() {
//
//}
