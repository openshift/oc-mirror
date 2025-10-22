package bundle

import (
	"os"
	"path/filepath"

	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/config"
)

// MakeWorkspaceDir creates the initial
// directories needed for image processing.
func MakeWorkspaceDirs(rootDir string) error {
	paths := []string{
		filepath.Join(config.SourceDir, config.PublishDir),
		filepath.Join(config.SourceDir, config.V2Dir),
		filepath.Join(config.SourceDir, config.HelmDir),
		filepath.Join(config.SourceDir, config.ReleaseSignatureDir),
	}
	for _, p := range paths {
		dir := filepath.Join(rootDir, p)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			klog.Infof("Creating directory: %v", dir)
			err := os.MkdirAll(dir, os.ModePerm)
			if err != nil {
				return err
			}
		} else {
			klog.Infof("Found: %v", dir)
		}
	}
	return nil
}
