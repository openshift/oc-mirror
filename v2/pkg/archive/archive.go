package archive

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	digest "github.com/opencontainers/go-digest"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

type MirrorArchive struct {
	Archiver
	destination string
	iscPath     string
	workingDir  string
	cacheDir    string
	archiveFile *os.File
	tarWriter   *tar.Writer
	//history history.History
	blobGatherer BlobsGatherer
}

// The caller must call Close!
func NewMirrorArchive(ctx context.Context, opts *mirror.CopyOptions, destination, iscPath, workingDir, cacheDir string) (MirrorArchive, error) {
	//TODO handle several chunks
	chunk := 1
	archiveFileName := fmt.Sprintf("%s_%06d.tar", archiveFilePrefix, chunk)
	archivePath := filepath.Join(destination, archiveFileName)
	// Create a new tar archive file
	// to be closed by BuildArchive
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return MirrorArchive{}, err
	}

	// Create a new tar writer
	// to be closed by BuildArchive
	tarWriter := tar.NewWriter(archiveFile)

	bg := NewImageBlobGatherer(ctx, opts)
	// history := history.NewHistory(ctx, options)
	ma := MirrorArchive{
		destination: destination,
		archiveFile: archiveFile,
		tarWriter:   tarWriter,
		//history:history
		blobGatherer: bg,
		workingDir:   workingDir,
		cacheDir:     cacheDir,
		iscPath:      iscPath,
	}
	return ma, nil
}

// BuildArchive creates an archive that contains:
// * docker/v2/repositories : manifests for all mirrored images
// * docker/v2/blobs/sha256 : blobs that haven't been mirrored (diff)
// * working-dir
// * image set config
func (ma MirrorArchive) BuildArchive(collectedImages []v1alpha3.CopyImageSchema) (string, error) {

	// 1 - Add files and directories under the cache's docker/v2/repositories to the archive
	repositoriesDir := filepath.Join(ma.cacheDir, cacheRepositoriesDir)
	err := ma.addAllFolder(repositoriesDir, ma.cacheDir)
	if err != nil {
		return "", fmt.Errorf("unable to add cache repositories to the archive : %v", err)
	}
	// 2- Add working-dir contents to archive
	err = ma.addAllFolder(ma.workingDir, filepath.Dir(ma.workingDir))
	if err != nil {
		return "", fmt.Errorf("unable to add working-dir to the archive : %v", err)
	}
	// 3 - Add imageSetConfig
	iscName := imageSetConfigPrefix + time.Now().Format(time.RFC3339)
	err = ma.addFile(ma.iscPath, iscName)
	if err != nil {
		return "", fmt.Errorf("unable to add image set configuration to the archive : %v", err)
	}
	// 4 - Add blobs
	// TODO Read history file
	blobsInHistory := make(map[string]string, 0)
	blobsInHistory["sha256:e1cb992e7555fa2b8405f96330d856d798e8f9fa2e2b78fdbb7cde084dfb010a"] = ""
	// blobsInHistory, err := ma.history.Read(since?)
	/*addedBlobs*/
	_, err = ma.addImagesDiff(collectedImages, blobsInHistory, ma.cacheDir)
	if err != nil {
		return "", fmt.Errorf("unable to add image blobs to the archive : %v", err)
	}
	//5 - update history file with addedBlobs
	// _, err = ma.history.Append(addedBlobs)
	return ma.archiveFile.Name(), nil
}

func (ma MirrorArchive) addImagesDiff(collectedImages []v1alpha3.CopyImageSchema, historyBlobs map[string]string, cacheDir string) (map[string]string, error) {
	allAddedBlobs := map[string]string{}
	for _, img := range collectedImages {
		imgBlobs, err := ma.blobGatherer.GatherBlobs(img.Destination)
		if err != nil {
			return nil, fmt.Errorf("unable to find blobs corresponding to %s: %v", img.Destination, err)
		}

		addedBlobs, err := ma.addBlobsDiff(imgBlobs, historyBlobs)
		if err != nil {
			return nil, fmt.Errorf("unable to add blobs corresponding to %s: %v", img.Destination, err)
		}

		for hash, value := range addedBlobs {
			allAddedBlobs[hash] = value
		}

	}

	return allAddedBlobs, nil
}

func (ma MirrorArchive) addBlobsDiff(collectedBlobs, historyBlobs map[string]string) (map[string]string, error) {
	blobsInDiff := map[string]string{}
	for hash := range collectedBlobs {
		if _, exists := historyBlobs[hash]; !exists {
			// hash does not exist in historyBlobs
			// Blob not yet mirrored
			// Add to tar
			d, err := digest.Parse(hash)
			if err != nil {
				return nil, err
			}
			blobPath := filepath.Join(ma.cacheDir, cacheBlobsDir, d.Algorithm().String(), d.Hex()[:2], d.Hex())
			err = ma.addAllFolder(blobPath, ma.cacheDir)
			if err != nil {
				return nil, err
			}
			blobsInDiff[hash] = ""
		}
	}
	return blobsInDiff, nil
}

func (ma MirrorArchive) addFile(pathToFile string, pathInTar string) error {
	fi, err := os.Stat(pathToFile)
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(fi, fi.Name())
	if err != nil {
		return err
	}
	header.Name = pathInTar

	if err := ma.tarWriter.WriteHeader(header); err != nil {
		return err
	}
	// Open the file for reading
	file, err := os.Open(pathToFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Copy the file contents to the tar archive
	if _, err := io.Copy(ma.tarWriter, file); err != nil {
		return err
	}
	return nil
}
func (ma MirrorArchive) addAllFolder(folderToAdd string, relativeTo string) error {
	return filepath.Walk(folderToAdd, func(path string, info os.FileInfo, incomingError error) error {
		if incomingError != nil {
			return incomingError
		}
		if info.IsDir() { // skip directories
			return nil
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		// Use full path as name (FileInfoHeader only takes the basename)
		// If we don't do this the directory strucuture would
		// not be preserved
		// https://golang.org/src/archive/tar/common.go?#L626
		header.Name, err = filepath.Rel(relativeTo, path)
		if err != nil {
			return err
		}

		// Write the header to the tar archive
		if err := ma.tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Open the file for reading
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Copy the file contents to the tar archive
		if _, err := io.Copy(ma.tarWriter, file); err != nil {
			return err
		}

		return nil
	})
}

func (ma MirrorArchive) Close() error {

	err1 := ma.archiveFile.Close()
	err2 := ma.tarWriter.Close()

	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}
