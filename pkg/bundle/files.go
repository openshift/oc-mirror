package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/gammazero/workerpool"
	"github.com/sirupsen/logrus"
)

// ReconcileFiles gather all files that were collected during a run
// and checks against the current list
func ReconcileFiles(meta v1alpha1.Metadata, rootDir string) (newFiles []v1alpha1.File, err error) {

	wp := workerpool.New(10)

	mu := new(sync.RWMutex)
	results := make(chan v1alpha1.File)

	foundFiles := make(map[string]struct{}, len(meta.PastFiles))
	for _, pf := range meta.PastFiles {
		foundFiles[pf.Name] = struct{}{}
	}

	// Ignore the current dir.
	foundFiles["."] = struct{}{}

	//Create a work pool
	logrus.Debug("Creating worker pool")

	err = filepath.Walk(rootDir, func(fpath string, info os.FileInfo, err error) error {

		if err != nil {
			return fmt.Errorf("traversing %s: %v", fpath, err)
		}

		if info == nil {

			return fmt.Errorf("traversing %s: %v", fpath, err)
		}

		logrus.Debugf("Creating task for %s", fpath)

		wp.Submit(func() {
			file := v1alpha1.File{
				Name: fpath,
			}

			mu.RLock()
			_, found := foundFiles[fpath]
			mu.RUnlock()

			if !found {

				results <- file

				mu.Lock()
				foundFiles[fpath] = struct{}{}
				mu.Unlock()

			} else {
				logrus.Debugf("File %s exists", fpath)
			}
		})

		return nil
	})

	go func() {
		wp.StopWait()
		close(results)
	}()

	for {
		rst, ok := <-results
		if !ok {
			break
		}
		if base := filepath.Base(fpath); base != config.MetadataFile && base != config.AssociationsFile {
			newFiles = append(newFiles, rst)
		}
	}

	return newFiles, err
}
