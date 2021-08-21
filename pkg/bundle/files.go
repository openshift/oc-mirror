package bundle

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

// ReconcileFiles gather all files that were collected during a run
// and checks against the current list
func ReconcileFiles(meta *v1alpha1.Metadata, rootDir string) (newFiles []string, err error) {

	foundFiles := make(map[string]struct{}, len(meta.PastFiles))
	for _, pf := range meta.PastFiles {
		foundFiles[pf.Name] = struct{}{}
	}
	// Ignore the current dir.
	foundFiles["."] = struct{}{}

	err = filepath.Walk(rootDir, func(fpath string, info os.FileInfo, err error) error {

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
			// Past files should only be image data, not tool metadata.
			if base := filepath.Base(fpath); base != config.MetadataFile && base != config.AssociationsFile {
				meta.PastFiles = append(meta.PastFiles, file)
			}
			foundFiles[fpath] = struct{}{}
			newFiles = append(newFiles, fpath)
		}

		return nil
	})

	return newFiles, err
}
