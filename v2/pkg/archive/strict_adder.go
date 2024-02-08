package archive

import (
	"archive/tar"
	"fmt"
	"os"
	"path/filepath"

	clog "github.com/openshift/oc-mirror/v2/pkg/log"
)

type strictAdder struct {
	destination        string
	archiveFile        *os.File
	tarWriter          *tar.Writer
	maxArchiveSize     int64
	currentChunkId     int
	sizeOfCurrentChunk int64
	logger             clog.PluggableLoggerInterface
}

// `newStrictAdder` initializes the strictAdder implementation for the `archiveAdder` interface.
// This implementation doesn't allow for any files to exceed the maxArchiveSize specified in the
// imageSetConfig. It stops adding to the archive chunks if a file exceeds maxArchiveSize
// and returns in error.
func newStrictAdder(maxSize int64, destination string, logger clog.PluggableLoggerInterface) (*strictAdder, error) {
	chunk := 1
	archiveFileName := fmt.Sprintf(archiveFileNameFormat, archiveFilePrefix, chunk)
	err := os.MkdirAll(destination, 0755)
	if err != nil {
		return &strictAdder{}, err
	}
	archivePath := filepath.Join(destination, archiveFileName)
	// Create a new tar archive file
	// to be closed by BuildArchive
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return &strictAdder{}, err
	}
	// Create a new tar writer
	// to be closed by the call to close method
	tarWriter := tar.NewWriter(archiveFile)
	if maxSize == 0 {
		maxSize = defaultSegSize * segMultiplier
	}
	p := strictAdder{
		maxArchiveSize:     maxSize,
		currentChunkId:     chunk,
		sizeOfCurrentChunk: int64(0),
		destination:        destination,
		archiveFile:        archiveFile,
		tarWriter:          tarWriter,
		logger:             logger,
	}
	return &p, nil
}

func (o *strictAdder) close() error {
	err := o.tarWriter.Flush()
	if err != nil {
		o.logger.Warn("error flushing archive writer : %v", err)
	}
	err = o.tarWriter.Close()
	if err != nil {
		o.logger.Warn("error closing archive writer : %v", err)
	}
	return o.archiveFile.Close()
}

// addFile copies the contents of the `pathToFile` file from the disk into
// the current chunk archive at `pathInTar` location.
// addFile monitors the size of the current chunk, and creates a new chunk to
// archive `pathToFile` if archiving would make the chunk exceed the maxArchiveSize.
// addFile stops the archiving and returns an error if `pathToFile`'s size is
// greater than maxArchiveSize.
func (o *strictAdder) addFile(pathToFile string, pathInTar string) error {
	fi, err := os.Stat(pathToFile)
	if err != nil {
		return err
	}

	// when a file is already bigger than the maxArchiveSize, it will not fit any chunk
	// therefore we should stop
	if fi.Size() > o.maxArchiveSize {
		return fmt.Errorf("maxArchiveSize %dG is too small compared to sizes of files that need to be included in the archive.\n%s: %dG\n Aborting archive generation", o.maxArchiveSize/segMultiplier, fi.Name(), fi.Size()/segMultiplier)
	}
	// check if we should add this file to the archive without exceeding the maxArchiveSize
	if fi.Size()+o.sizeOfCurrentChunk > o.maxArchiveSize {
		err = o.nextChunk()
		if err != nil {
			return err
		}
	}
	err = addFileToWriter(fi, pathToFile, pathInTar, o.tarWriter)
	if err != nil {
		return err
	}

	o.sizeOfCurrentChunk += fi.Size()
	return nil
}

// addAllFolder copies the contents of the `folderToAdd` from the disk into
// the current chunk archive under `relativeTo` path.
// addAllFolder monitors the size of the current chunk, and creates a new chunk to
// archive files from `folderToAdd` if archiving would make the chunk exceed the maxArchiveSize.
// addAllFolder stops the archiving and returns an error if the size of any file within the folder is
// greater than maxArchiveSize.
func (o *strictAdder) addAllFolder(folderToAdd string, relativeTo string) error {
	return filepath.Walk(folderToAdd, func(path string, info os.FileInfo, incomingError error) error {
		if incomingError != nil {
			return incomingError
		}
		if info.IsDir() { // skip directories
			return nil
		}
		// when a file is already bigger than the maxArchiveSize, it will not fit any chunk
		// therefore we should stop
		if info.Size() > o.maxArchiveSize {
			return fmt.Errorf("maxArchiveSize %dG is too small compared to sizes of files that need to be included in the archive.\n%s: %dG\n Aborting archive generation", o.maxArchiveSize/segMultiplier, info.Name(), info.Size()/segMultiplier)
		}
		// check if we should add this file to the archive without exceeding the maxArchiveSize
		if info.Size()+o.sizeOfCurrentChunk > o.maxArchiveSize {
			err := o.nextChunk()
			if err != nil {
				return err
			}
		}
		// Use full path as name (FileInfoHeader only takes the basename)
		// If we don't do this the directory strucuture would
		// not be preserved
		// https://golang.org/src/archive/tar/common.go?#L626
		pathInTar, err := filepath.Rel(relativeTo, path)
		if err != nil {
			return err
		}

		err = addFileToWriter(info, path, pathInTar, o.tarWriter)
		if err != nil {
			return err
		}

		o.sizeOfCurrentChunk += info.Size()
		return nil
	})
}

// nextChunk is called in order to close the current chunk archive
// and create the next chunk archive.
// it creates a new file and a new tarWriter, and places them in `o.archiveFile`
// and `o.tarWriter` respectively, for the strictAdder to use.
func (o *strictAdder) nextChunk() error {
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
	archiveFileName := fmt.Sprintf(archiveFileNameFormat, archiveFilePrefix, o.currentChunkId)
	archivePath := filepath.Join(o.destination, archiveFileName)

	o.archiveFile, err = os.Create(archivePath)
	if err != nil {
		return err
	}
	o.tarWriter = tar.NewWriter(o.archiveFile)
	return nil
}
