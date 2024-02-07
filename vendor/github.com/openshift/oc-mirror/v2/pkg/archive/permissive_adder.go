package archive

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	clog "github.com/openshift/oc-mirror/v2/pkg/log"
)

type permissiveAdder struct {
	destination        string
	archiveFile        *os.File
	tarWriter          *tar.Writer
	maxArchiveSize     int64
	currentChunkId     int
	sizeOfCurrentChunk int64
	oversizedFiles     map[string]int64
	logger             clog.PluggableLoggerInterface
}

// `newPermissiveAdder` initializes the permissiveAdder implementation for the `archiveAdder` interface.
// This implementation allows  files to exceed the maxArchiveSize specified in the
// imageSetConfig. It places them in special archive chunks, on their own, and keeps track of the list
// of oversized files.
func newPermissiveAdder(maxSize int64, destination string, logger clog.PluggableLoggerInterface) (*permissiveAdder, error) {
	chunk := 1
	archiveFileName := fmt.Sprintf("%s_%06d.tar", archiveFilePrefix, chunk)
	err := os.MkdirAll(destination, 0755)
	if err != nil {
		return &permissiveAdder{}, err
	}
	archivePath := filepath.Join(destination, archiveFileName)
	// Create a new tar archive file
	// to be closed by BuildArchive
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return &permissiveAdder{}, err
	}
	// Create a new tar writer
	// to be closed by BuildArchive
	tarWriter := tar.NewWriter(archiveFile)
	if maxSize == 0 {
		maxSize = defaultSegSize * segMultiplier
	}
	p := permissiveAdder{
		maxArchiveSize:     maxSize,
		currentChunkId:     chunk,
		sizeOfCurrentChunk: int64(0),
		destination:        destination,
		archiveFile:        archiveFile,
		tarWriter:          tarWriter,
		logger:             logger,
		oversizedFiles:     map[string]int64{},
	}
	return &p, nil
}

func (o *permissiveAdder) close() error {
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
// When encountering a `pathToFile` whose size is greater than maxArchiveSize,
// this file will be placed in an exceptionChunk and marked oversized.
func (o *permissiveAdder) addFile(pathToFile string, pathInTar string) error {
	fi, err := os.Stat(pathToFile)
	if err != nil {
		return err
	}

	// when a file is already bigger than the maxArchiveSize, it will not fit any chunk.
	// It is put on its own in an exceptionChunk and flagged as oversized. The method returns.
	if fi.Size() > o.maxArchiveSize {
		o.logger.Warn("maxArchiveSize %dG is too small compared to sizes of files that need to be included in the archive.\n%s: %dG", o.maxArchiveSize/segMultiplier, pathToFile, fi.Size()/segMultiplier)
		o.oversizedFiles[pathToFile] = fi.Size()

		return o.exceptionChunk(fi, pathToFile, pathInTar)
	}
	// check if we should add this file to the archive without exceeding the maxArchiveSize
	if fi.Size()+o.sizeOfCurrentChunk > o.maxArchiveSize {
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

// addAllFolder copies the contents of the `folderToAdd` from the disk into
// the current chunk archive under `relativeTo` path.
// addAllFolder monitors the size of the current chunk, and creates a new chunk to
// archive files from `folderToAdd` if archiving would make the chunk exceed the maxArchiveSize.
// When encountering a file whose size is greater than maxArchiveSize,
// this file will be placed in an exceptionChunk and marked oversized.
func (o *permissiveAdder) addAllFolder(folderToAdd string, relativeTo string) error {
	return filepath.Walk(folderToAdd, func(path string, info os.FileInfo, incomingError error) error {
		if incomingError != nil {
			return incomingError
		}
		if info.IsDir() { // skip directories
			return nil
		}
		// when a file is already bigger than the maxArchiveSize, it will not fit any chunk.
		// It is put on its own in an exceptionChunk and flagged as oversized. The method returns.
		if info.Size() > o.maxArchiveSize {
			o.logger.Warn("maxArchiveSize %dG is too small compared to sizes of files that need to be included in the archive.\n%s: %dG", o.maxArchiveSize/segMultiplier, info.Name(), info.Size()/segMultiplier)
			o.oversizedFiles[path] = info.Size()
			fileNameInArchive, err := filepath.Rel(relativeTo, path)
			if err != nil {
				return err
			}
			return o.exceptionChunk(info, path, fileNameInArchive)

		}
		// check if we should add this file to the archive without exceeding the maxArchiveSize
		if info.Size()+o.sizeOfCurrentChunk > o.maxArchiveSize {
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

// nextChunk is called in order to close the current chunk archive
// and create the next chunk archive.
// it creates a new file and a new tarWriter, and places them in `o.archiveFile`
// and `o.tarWriter` respectively, for the strictAdder to use.
func (o *permissiveAdder) nextChunk() error {
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

// exceptionChunk handles creating a new archive file to copy the oversized file in it
// then immediately closes that exceptionChunk. It doesn't alter the o.tarWriter, o.sizeOfCurrentChunk.
// It just increments the currentChunkId in order to show that this id has been used.
func (o *permissiveAdder) exceptionChunk(oversizedFileInfo fs.FileInfo, oversizedFilePath, pathInTar string) error {
	// next chunk init
	o.currentChunkId += 1
	// Create a new tar archive file
	exceptionArchiveFileName := fmt.Sprintf("%s_%06d.tar", archiveFilePrefix, o.currentChunkId)
	exceptionArchivePath := filepath.Join(o.destination, exceptionArchiveFileName)

	exceptionArchiveFile, err := os.Create(exceptionArchivePath)
	if err != nil {
		return err
	}
	exceptionTarWriter := tar.NewWriter(exceptionArchiveFile)

	// immediately close the exceptionChunk file when this method is done
	defer func() {
		exceptionTarWriter.Flush()
		exceptionTarWriter.Close()
		exceptionArchiveFile.Close()
	}()

	// create the header for the file
	header, err := tar.FileInfoHeader(oversizedFileInfo, oversizedFileInfo.Name())
	if err != nil {
		return err
	}
	header.Name = pathInTar

	if err := exceptionTarWriter.WriteHeader(header); err != nil {
		return err
	}
	// Open the file for reading
	file, err := os.Open(oversizedFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Copy the file contents to the tar archive
	if _, err := io.Copy(exceptionTarWriter, file); err != nil {
		return err
	}

	return nil
}

func (o *permissiveAdder) getOversizedFiles() map[string]int64 {
	return o.oversizedFiles
}
