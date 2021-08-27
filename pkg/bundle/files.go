package bundle

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/sirupsen/logrus"
)

// ReconcileManifest gather all manifests that were collected during a run
// and checks against the current list
func ReconcileManifests(meta *v1alpha1.Metadata, sourceDir string) error {

	foundFiles := make(map[string]struct{}, len(meta.PastManifests))
	for _, pf := range meta.PastManifests {
		foundFiles[pf.Name] = struct{}{}
	}

	// Ignore the current dir.
	foundFiles["."] = struct{}{}

	return filepath.Walk("v2", func(fpath string, info os.FileInfo, err error) error {

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
			meta.PastManifests = append(meta.PastManifests, file)
			foundFiles[fpath] = struct{}{}

			// Make manifest dir in target
			targetPath := filepath.Join(sourceDir, "manifests", filepath.Dir(fpath))
			if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
				return err
			}

			// Move new manifest to manifests directory
			if info.Mode().IsRegular() {

				logrus.Debugf("Adding manifest %s", fpath)

				// Copy blob to blobs directory
				input, err := ioutil.ReadFile(fpath)
				if err != nil {
					return err
				}

				if err := ioutil.WriteFile(filepath.Join(targetPath, info.Name()), input, os.ModePerm); err != nil {
					return err
				}
			}

		} else {
			logrus.Debugf("Manifest %s exists in imageset, skipping...", fpath)
		}

		return nil
	})
}

// ReconcileBlobs gather all blobs that were collected during a run
// and checks against the current list
func ReconcileBlobs(meta *v1alpha1.Metadata, sourceDir string) error {

	foundFiles := make(map[string]struct{}, len(meta.PastBlobs))
	for _, pf := range meta.PastBlobs {
		foundFiles[pf.Name] = struct{}{}
	}

	// Ignore the current dir.
	foundFiles["."] = struct{}{}

	return filepath.Walk("v2", func(fpath string, info os.FileInfo, err error) error {

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
				meta.PastBlobs = append(meta.PastBlobs, file)
				foundFiles[info.Name()] = struct{}{}

				logrus.Debugf("Adding blob %s", info.Name())

				// Copy blob to blobs directory
				input, err := ioutil.ReadFile(fpath)
				if err != nil {
					return err
				}

				if err := ioutil.WriteFile(filepath.Join(sourceDir, "blobs", info.Name()), input, os.ModePerm); err != nil {
					return err
				}

			} else {
				logrus.Debugf("Blob %s exists in imageset, skipping...", info.Name())
			}
		}

		return nil
	})
}
