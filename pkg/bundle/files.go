package bundle

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

// ReconcileFiles gather all files that were collected during a run
// and checks against the current list
func ReconcileFiles(i *v1alpha1.MetadataSpec, rootDir string) error {

	foundFiles := make(map[string]struct{}, len(i.PastFiles))
	for _, fpath := range i.PastFiles {
		foundFiles[fpath.Name] = struct{}{}
	}
	err := filepath.Walk(rootDir, func(fpath string, info os.FileInfo, err error) error {

		if err != nil {
			return fmt.Errorf("traversing %s: %v", fpath, err)
		}
		if info == nil {
			return fmt.Errorf("no file info")
		}

		file := v1alpha1.File{
			Name: fpath,
		}

		if _, found := foundFiles[fpath]; !found {
			i.PastFiles = append(i.PastFiles, file)
			foundFiles[fpath] = struct{}{}
		}

		return nil
	})

	return err
}
