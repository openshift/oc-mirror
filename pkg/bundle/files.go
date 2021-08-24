package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/openshift/oc/pkg/cli/image/workqueue"
	"github.com/sirupsen/logrus"
)

// ReconcileFiles gather all files that were collected during a run
// and checks against the current list
func ReconcileFiles(meta v1alpha1.Metadata, rootDir string) (newFiles []v1alpha1.File, err error) {

	var wg sync.WaitGroup
	wg.Add(1)

	stopCh := make(chan struct{})
	defer close(stopCh)

	wp := workqueue.New(10, stopCh)

	mu := new(sync.RWMutex)

	if err != nil {
		return nil, err
	}

	results := make(chan v1alpha1.File)

	foundFiles := make(map[string]struct{}, len(meta.PastFiles))
	for _, pf := range meta.PastFiles {
		foundFiles[pf.Name] = struct{}{}
	}

	// Ignore the current dir.
	foundFiles["."] = struct{}{}

	go func() {

		defer wg.Done()
		for {
			rst, ok := <-results
			if !ok {
				break
			}
			if base := filepath.Base(rst.Name); base != config.MetadataFile && base != config.AssociationsFile {
				newFiles = append(newFiles, rst)
				logrus.Debugf("File %s added", rst.Name)
			}
		}
	}()

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

		wp.Queue(func(w workqueue.Work) {

			w.Parallel(func() {

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
		})

		return nil
	})

	go func() {
		wp.Done()
		close(results)
	}()

	wg.Wait()

	return newFiles, nil

}
