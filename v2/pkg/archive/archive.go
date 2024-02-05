package archive

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	digest "github.com/opencontainers/go-digest"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/history"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

type MirrorArchive struct {
	Archiver
	destination        string
	iscPath            string
	workingDir         string
	cacheDir           string
	archiveFile        *os.File
	tarWriter          *tar.Writer
	history            history.History
	blobGatherer       BlobsGatherer
	maxArchiveSize     int64
	currentChunkId     int
	sizeOfCurrentChunk int64
	oversizedFiles     map[string]int64
	logger             clog.PluggableLoggerInterface
}

const (
	segMultiplier  int64 = 1024 * 1024 * 1024
	defaultSegSize int64 = 500
)

// The caller must call Close!
func NewMirrorArchive(opts *mirror.CopyOptions, destination, iscPath, workingDir, cacheDir string, maxSize int64, logg clog.PluggableLoggerInterface) (*MirrorArchive, error) {
	chunk := 1
	archiveFileName := fmt.Sprintf("%s_%06d.tar", archiveFilePrefix, chunk)
	err := os.MkdirAll(destination, 0755)
	if err != nil {
		return &MirrorArchive{}, err
	}
	archivePath := filepath.Join(destination, archiveFileName)
	// Create a new tar archive file
	// to be closed by BuildArchive
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return &MirrorArchive{}, err
	}

	// create the history interface
	history, err := history.NewHistory(workingDir, time.Time{}, logg, history.OSFileCreator{})
	if err != nil {
		return &MirrorArchive{}, err
	}

	// Create a new tar writer
	// to be closed by BuildArchive
	tarWriter := tar.NewWriter(archiveFile)

	bg := NewImageBlobGatherer(opts)

	if maxSize == 0 {
		maxSize = defaultSegSize
	}
	maxSize *= segMultiplier

	ma := MirrorArchive{
		destination:        destination,
		archiveFile:        archiveFile,
		tarWriter:          tarWriter,
		history:            history,
		blobGatherer:       bg,
		workingDir:         workingDir,
		cacheDir:           cacheDir,
		iscPath:            iscPath,
		maxArchiveSize:     maxSize,
		currentChunkId:     chunk,
		sizeOfCurrentChunk: int64(0),
		oversizedFiles:     make(map[string]int64),
		logger:             logg,
	}
	return &ma, nil
}

// BuildArchive creates an archive that contains:
// * docker/v2/repositories : manifests for all mirrored images
// * docker/v2/blobs/sha256 : blobs that haven't been mirrored (diff)
// * working-dir
// * image set config
func (o *MirrorArchive) BuildArchive(ctx context.Context, collectedImages []v1alpha3.CopyImageSchema) (string, error) {

	// 1 - Add files and directories under the cache's docker/v2/repositories to the archive
	repositoriesDir := filepath.Join(o.cacheDir, cacheRepositoriesDir)
	err := o.addAllFolder(repositoriesDir, o.cacheDir)
	if err != nil {
		return "", fmt.Errorf("unable to add cache repositories to the archive : %v", err)
	}
	// 2- Add working-dir contents to archive
	err = o.addAllFolder(o.workingDir, filepath.Dir(o.workingDir))
	if err != nil {
		return "", fmt.Errorf("unable to add working-dir to the archive : %v", err)
	}
	// 3 - Add imageSetConfig
	iscName := imageSetConfigPrefix + time.Now().UTC().Format(time.RFC3339)
	err = o.addFile(o.iscPath, iscName)
	if err != nil {
		return "", fmt.Errorf("unable to add image set configuration to the archive : %v", err)
	}
	// 4 - Add blobs
	blobsInHistory, err := o.history.Read()
	if err != nil && !errors.Is(err, &history.EmptyHistoryError{}) {
		return "", fmt.Errorf("unable to read history metadata from working-dir : %v", err)
	}
	// ignoring the error otherwise: continuing with an empty map in blobsInHistory

	addedBlobs, err := o.addImagesDiff(ctx, collectedImages, blobsInHistory, o.cacheDir)
	if err != nil {
		return "", fmt.Errorf("unable to add image blobs to the archive : %v", err)
	}
	//5 - update history file with addedBlobs
	_, err = o.history.Append(addedBlobs)
	if err != nil {
		return "", fmt.Errorf("unable to update history metadata: %v", err)
	}
	o.tarWriter.Flush()
	o.tarWriter.Close()
	// create a warning with the archiveSize that should be set, and the list of files that
	// were exceeding the max
	if len(o.oversizedFiles) > 0 {
		recommendedSize := int64(0)
		o.logger.Warn("The following files exceed the archiveSize configured: ")
		for f, s := range o.oversizedFiles {
			o.logger.Warn("%s: %d", f, s/segMultiplier)
			if s > recommendedSize {
				recommendedSize = s
			}
		}
		recommendedSize /= segMultiplier
		o.logger.Warn("Please consider updating archiveSize to at least %d", recommendedSize)
	}
	return o.archiveFile.Name(), nil
}

func (o *MirrorArchive) addImagesDiff(ctx context.Context, collectedImages []v1alpha3.CopyImageSchema, historyBlobs map[string]string, cacheDir string) (map[string]string, error) {
	allAddedBlobs := map[string]string{}
	for _, img := range collectedImages {
		imgBlobs, err := o.blobGatherer.GatherBlobs(ctx, img.Destination)
		if err != nil {
			return nil, fmt.Errorf("unable to find blobs corresponding to %s: %v", img.Destination, err)
		}

		addedBlobs, err := o.addBlobsDiff(imgBlobs, historyBlobs, allAddedBlobs)
		if err != nil {
			return nil, fmt.Errorf("unable to add blobs corresponding to %s: %v", img.Destination, err)
		}

		for hash, value := range addedBlobs {
			allAddedBlobs[hash] = value
		}

	}

	return allAddedBlobs, nil
}

func (o *MirrorArchive) addBlobsDiff(collectedBlobs, historyBlobs map[string]string, alreadyAddedBlobs map[string]string) (map[string]string, error) {
	blobsInDiff := map[string]string{}
	for hash := range collectedBlobs {
		_, alreadyMirrored := historyBlobs[hash]
		_, previouslyAdded := alreadyAddedBlobs[hash]
		skip := alreadyMirrored || previouslyAdded
		if !skip {
			// Add to tar
			d, err := digest.Parse(hash)
			if err != nil {
				return nil, err
			}
			blobPath := filepath.Join(o.cacheDir, cacheBlobsDir, d.Algorithm().String(), d.Encoded()[:2], d.Encoded())
			err = o.addAllFolder(blobPath, o.cacheDir)
			if err != nil {
				return nil, err
			}
			blobsInDiff[hash] = ""
		}
	}
	return blobsInDiff, nil
}

func (o *MirrorArchive) addFile(pathToFile string, pathInTar string) error {
	fi, err := os.Stat(pathToFile)
	if err != nil {
		return err
	}

	// when a file is already bigger than the maxArchiveSize, it will not fit any chunk
	// therefore we create a warning, and make an exception for a bigger chunk
	if fi.Size() > o.maxArchiveSize {
		o.logger.Warn("maxArchiveSize %dG is too small compared to sizes of files that need to be included in the archive.\n%s: %dG\n Aborting archive generation", o.maxArchiveSize/segMultiplier, fi.Name(), fi.Size()/segMultiplier)
		o.oversizedFiles[pathToFile] = fi.Size()
		err = o.nextChunk()
		if err != nil {
			return err
		}
	} else if fi.Size()+o.sizeOfCurrentChunk > o.maxArchiveSize { // check if we should add this file to the archive without exceeding the maxArchiveSize
		// we only check this if the file size isn't exceeding the max, so that we don't create 2 new chunks
		err = o.nextChunk()
		if err != nil {
			return err
		}
	}
	header, err := tar.FileInfoHeader(fi, fi.Name())
	if err != nil {
		return err
	}
	header.Name = pathInTar

	if err := o.tarWriter.WriteHeader(header); err != nil {
		return err
	}
	// Open the file for reading
	file, err := os.Open(pathToFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Copy the file contents to the tar archive
	if _, err := io.Copy(o.tarWriter, file); err != nil {
		return err
	}
	o.sizeOfCurrentChunk += fi.Size()
	return nil
}

func (o *MirrorArchive) addAllFolder(folderToAdd string, relativeTo string) error {
	return filepath.Walk(folderToAdd, func(path string, info os.FileInfo, incomingError error) error {
		if incomingError != nil {
			return incomingError
		}
		if info.IsDir() { // skip directories
			return nil
		}
		// when a file is already bigger than the maxArchiveSize, it will not fit any chunk
		// therefore we create a warning, and make an exception for a bigger chunk
		if info.Size() > o.maxArchiveSize && o.sizeOfCurrentChunk != 0 {
			o.logger.Warn("maxArchiveSize %dG is too small compared to sizes of files that need to be included in the archive.\n%s: %dG\n Aborting archive generation", o.maxArchiveSize/segMultiplier, info.Name(), info.Size()/segMultiplier)
			o.oversizedFiles[path] = info.Size()
			err := o.nextChunk()
			if err != nil {
				return err
			}
		} else if info.Size()+o.sizeOfCurrentChunk > o.maxArchiveSize { // check if we should add this file to the archive without exceeding the maxArchiveSize
			// we only check this if the file size isn't exceeding the max, so that we don't create 2 new chunks
			err := o.nextChunk()
			if err != nil {
				return err
			}
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
		if err := o.tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Open the file for reading
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Copy the file contents to the tar archive
		if _, err := io.Copy(o.tarWriter, file); err != nil {
			return err
		}

		o.sizeOfCurrentChunk += info.Size()
		return nil
	})
}

func (o *MirrorArchive) nextChunk() error {
	// close the current archive
	err := o.tarWriter.Close()
	if err != nil {
		return err
	}
	err = o.archiveFile.Close()
	if err != nil {
		return err
	}

	// next chunk init
	o.currentChunkId += 1
	o.sizeOfCurrentChunk = 0

	// Create a new tar archive file
	// to be closed by BuildArchive
	archiveFileName := fmt.Sprintf("%s_%06d.tar", archiveFilePrefix, o.currentChunkId)
	archivePath := filepath.Join(o.destination, archiveFileName)

	o.archiveFile, err = os.Create(archivePath)
	if err != nil {
		return err
	}
	o.tarWriter = tar.NewWriter(o.archiveFile)
	return nil
}

func (o *MirrorArchive) Close() error {

	err1 := o.archiveFile.Close()
	err2 := o.tarWriter.Close()

	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}
