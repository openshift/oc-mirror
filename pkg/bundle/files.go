package bundle

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/sirupsen/logrus"
)

// ReconcileManifest gather all manifests that were collected during a run
// and checks against the current list
func ReconcileManifests(meta v1alpha1.Metadata, sourceDir string) (newManifest []v1alpha1.Manifest, err error) {

	foundFiles := make(map[string]struct{}, len(meta.PastManifests))
	for _, pf := range meta.PastManifests {
		foundFiles[pf.Name] = struct{}{}
	}

	// Ignore the current dir.
	foundFiles["."] = struct{}{}

	err = filepath.Walk("v2", func(fpath string, info os.FileInfo, err error) error {

		if err != nil {
			return fmt.Errorf("traversing %s: %v", fpath, err)
		}
		if info == nil {
			return fmt.Errorf("no file info")
		}

		if info.IsDir() && info.Name() == "blobs" {
			return filepath.SkipDir
		}

		// TODO: figure a robust way to get the namespace from the path
		file := v1alpha1.Manifest{
			Name: fpath,
		}

		if _, found := foundFiles[fpath]; !found {

			// Past files should only be image data, not tool metadata.
			newManifest = append(newManifest, file)
			foundFiles[fpath] = struct{}{}

		} else {
			logrus.Debugf("Manifest %s exists in imageset, skipping...", fpath)
		}

		return nil
	})

	return newManifest, err
}

// ReconcileBlobs gather all blobs that were collected during a run
// and checks against the current list
func ReconcileBlobs(meta v1alpha1.Metadata, sourceDir string) (newBlobs []v1alpha1.Blob, err error) {

	foundFiles := make(map[string]struct{}, len(meta.PastBlobs))
	for _, pf := range meta.PastBlobs {
		foundFiles[pf.Name] = struct{}{}
	}

	// Ignore the current dir.
	foundFiles["."] = struct{}{}

	err = filepath.Walk("v2", func(fpath string, info os.FileInfo, err error) error {

		if err != nil {
			return fmt.Errorf("traversing %s: %v", fpath, err)
		}
		if info == nil {
			return fmt.Errorf("no file info")
		}

		if info.IsDir() && info.Name() == "manifests" {
			return filepath.SkipDir
		}

		if info.Mode().IsRegular() {
			file := v1alpha1.Blob{
				Name: info.Name(),
			}

			if _, found := foundFiles[info.Name()]; !found {
				newBlobs = append(newBlobs, file)
				foundFiles[info.Name()] = struct{}{}

				logrus.Debugf("Adding blob %s", info.Name())

			} else {
				logrus.Debugf("Blob %s exists in imageset, skipping...", info.Name())
			}
		}

		return nil
	})

	return newBlobs, err
}
