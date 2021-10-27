package bundle

import (
	"os"
	"path/filepath"

	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/sirupsen/logrus"
)

func MakeCreateDirs(rootDir string) error {
	paths := []string{
		filepath.Join(config.SourceDir, config.PublishDir),
		filepath.Join(config.SourceDir, "v2"),
		filepath.Join(config.SourceDir, config.HelmDir),
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
