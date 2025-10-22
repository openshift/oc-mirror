package bundle

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archiver/v3"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/archive"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

// ReconcileV2Dir gathers all manifests and blobs that were collected during a run
// and checks against the current list.
// This function is used to prepare a list of files that need to added to the Imageset.
func ReconcileV2Dir(assocs image.AssociationSet, filenames map[string]string) (manifests []string, blobs []string, err error) {

	foundFiles := map[string]struct{}{}

	// Checking against all digest because mirroring
	// by digest can cause the manifest to pop up in the blobs
	// directory
	// TODO(jpower432): Investigate why this happens.
	// Happens with oc image mirror as well.
	for _, digest := range assocs.GetDigests() {
		foundFiles[digest] = struct{}{}
	}

	for rootOnDisk, rootInArchive := range filenames {

		if rootInArchive == "" {
			rootInArchive = filepath.Base(rootInArchive)
		}

		if filepath.Base(rootOnDisk) != config.V2Dir {
			return manifests, blobs, fmt.Errorf("path %q is not a v2 directory", rootOnDisk)
		}

		err = filepath.WalkDir(rootOnDisk, func(filename string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			info, err := d.Info()
			if err != nil {
				return err
			}

			// Rename manifests to ensure the match the files processed
			// during archiving
			nameInArchive := filepath.Join(rootInArchive, strings.TrimPrefix(filename, rootOnDisk))

			dir := filepath.Dir(filename)
			switch filepath.Base(dir) {
			case config.BlobDir:
				if info.Mode().IsRegular() {
					if _, found := foundFiles[info.Name()]; found {
						klog.V(1).Infof("Blob %s exists in imageset, skipping...", info.Name())
						return nil
					}
					blobs = append(blobs, info.Name())
					foundFiles[info.Name()] = struct{}{}
					klog.V(1).Infof("Adding blob %s", info.Name())
				}
			default:
				// Skips the blob dir which
				// does not come up as its own base dir
				if info.Name() == config.BlobDir {
					return nil
				}
				m := info.Mode()
				if m.IsRegular() || m&fs.ModeSymlink != 0 {
					manifests = append(manifests, nameInArchive)
				}
			}

			return nil
		})
	}

	return manifests, blobs, err
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
				klog.V(1).Infof("Found archive %s", path)
				return a.Walk(path, func(f archiver.File) error {
					switch t := f.Header.(type) {
					case *tar.Header:
						name := filepath.Clean(t.Name)
						filesinArchive[name] = path
						match++
						return nil
					default:
						return fmt.Errorf("file type not currently implemented %v", t)
					}
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
			switch t := f.Header.(type) {
			case *tar.Header:
				name := filepath.Clean(t.Name)
				filesinArchive[name] = from
				return nil
			default:
				return fmt.Errorf("file type not currently implemented %v", t)
			}
		})
	}

	return filesinArchive, err
}

// ReadMetadataFromFile will return the metadata from a given imageset
func ReadMetadataFromFile(ctx context.Context, archivePath string) (v1alpha2.Metadata, error) {
	a := archive.NewArchiver()
	meta := v1alpha2.NewMetadata()

	// Get archive with metadata
	filesInArchive, err := ReadImageSet(a, archivePath)
	if err != nil {
		return meta, err
	}

	// Create workspace to work from
	tmpdir, err := os.MkdirTemp(".", "metadata")
	if err != nil {
		return meta, err
	}
	defer os.RemoveAll(tmpdir)

	archive, ok := filesInArchive[config.MetadataBasePath]
	if !ok {
		return meta, errors.New("metadata is not in archive")
	}

	klog.V(2).Infof("Extracting incoming metadata")
	if err := a.Extract(archive, config.MetadataBasePath, tmpdir); err != nil {
		return meta, err
	}

	workspace, err := storage.NewLocalBackend(tmpdir)

	if err != nil {
		return meta, err
	}

	if err := workspace.ReadMetadata(ctx, &meta, config.MetadataBasePath); err != nil {
		return meta, err
	}

	return meta, nil
}
