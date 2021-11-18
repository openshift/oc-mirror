package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archiver/v3"
	"github.com/sirupsen/logrus"

	"github.com/openshift/oc-mirror/pkg/archive"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
)

// ReconcileManifest gather all manifests that were collected during a run
// and checks against the current list
func ReconcileManifests() (manifests []v1alpha1.Manifest, err error) {

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

		manifests = append(manifests, file)

		return nil
	})

	return manifests, err
}

// ReconcileBlobs gather all blobs that were collected during a run
// and checks against the current list
func ReconcileBlobs(meta v1alpha1.Metadata) (newBlobs []v1alpha1.Blob, err error) {

	foundFiles := make(map[string]struct{}, len(meta.PastBlobs))
	for _, pf := range meta.PastBlobs {
		foundFiles[pf.ID] = struct{}{}
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
				ID: info.Name(),
				// QUESTION(estroz): if the image name only had one component,
				// will the publish lookup fail?
				NamespaceName: strings.TrimPrefix(fpath, "v2"+string(filepath.Separator)),
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

// ReadImageSet set will create a map with all the files located in the archives
func ReadImageSet(a archive.Archiver, from string) (map[string]string, error) {

	filesinArchive := make(map[string]string)

	file, err := os.Stat(from)
	if err != nil {
		return nil, err
	}

	if file.IsDir() {

		// Walk the directory and load the files from the archives
		// into the map
		var match int
		err = filepath.Walk(from, func(path string, info os.FileInfo, err error) error {

			if err != nil {
				return fmt.Errorf("traversing %s: %v", path, err)
			}
			if info == nil {
				return fmt.Errorf("no file info")
			}

			extension := filepath.Ext(path)
			extension = strings.TrimPrefix(extension, ".")
			if extension == a.String() {
				logrus.Debugf("Found archive %s", path)
				return a.Walk(path, func(f archiver.File) error {
					filesinArchive[f.Name()] = path
					match++
					return nil
				})
			}

			return nil
		})

		// Make sure the directory is not empty
		if match == 0 {
			return nil, fmt.Errorf("no archives found in directory %s", from)
		}

	} else {
		// Walk the archive and load the file names into the map
		err = a.Walk(from, func(f archiver.File) error {
			filesinArchive[f.Name()] = from
			return nil
		})
	}

	return filesinArchive, err
}
