package archive

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mholt/archiver/v3"
	"github.com/sirupsen/logrus"
)

type Archiver interface {
	String() string
	Archive([]string, string) error
	Unarchive(string, string) error
	Write(archiver.File) error
	Create(io.Writer) error
	Close() error
	Walk(string, archiver.WalkFunc) error
	Open(io.Reader, int64) error
	Read() (archiver.File, error)
}

// NewArchiver creates a new archiver for tar archive manipultation
func NewArchiver() Archiver {
	return &archiver.Tar{
		OverwriteExisting:      false,
		MkdirAll:               true,
		ImplicitTopLevelFolder: false,
		StripComponents:        0,
		ContinueOnError:        false,
	}
}

// CreateSplitAchrive will create multiple tar archives from source directory
func CreateSplitArchive(a Archiver, maxSplitSize int64, destDir, sourceDir, prefix string) error {

	// Declare split variables
	splitNum := 0
	splitSize := int64(0)
	splitPath := fmt.Sprintf("%s/%s_%06d.%s", destDir, prefix, splitNum, a.String())

	splitFile, err := createArchive(a, splitPath)

	if err != nil {
		return fmt.Errorf("error creating archive %s: %v", splitPath, err)
	}

	sourceInfo, err := os.Stat(sourceDir)

	if err != nil {
		return fmt.Errorf("%s: stat: %v", sourceDir, err)
	}

	walkErr := filepath.Walk(sourceDir, func(fpath string, info os.FileInfo, err error) error {

		if err != nil {
			return fmt.Errorf("traversing %s: %v", fpath, err)
		}
		if info == nil {
			return fmt.Errorf("no file info")
		}

		// skip V2 directory because we have already copied over
		// what we needed during reconcilination
		if info.IsDir() && info.Name() == "v2" && filepath.Dir(fpath) != "manifests" {
			return filepath.SkipDir
		}

		nameInArchive, err := archiver.NameInArchive(sourceInfo, sourceDir, fpath)
		if err != nil {
			return fmt.Errorf("creating %s: %v", nameInArchive, err)
		}

		var file io.ReadCloser
		if info.Mode().IsRegular() {
			file, err = os.Open(fpath)
			if err != nil {
				return fmt.Errorf("%s: opening: %v", fpath, err)
			}
			defer file.Close()
		}

		f := archiver.File{
			FileInfo: archiver.FileInfo{
				FileInfo:   info,
				CustomName: nameInArchive,
			},
			ReadCloser: file,
		}

		// If the file is too large create a new one
		if info.Size()+splitSize > maxSplitSize {

			// Close current tar archive
			a.Close()
			splitFile.Close()

			logrus.Info(splitSize)

			// Increment split number and reset splitSize
			splitNum += 1
			splitSize = int64(0)
			splitPath = fmt.Sprintf("%s/%s_%06d.%s", destDir, prefix, splitNum, a.String())

			// Create a new tar archive for writing
			splitFile, err = createArchive(a, splitPath)

			if err != nil {
				return fmt.Errorf("error creating archive %s: %v", splitPath, err)
			}
		}

		// Write file to current archive file
		if err = a.Write(f); err != nil {
			return fmt.Errorf("%s: writing: %s", fpath, err)
		}

		splitSize += info.Size()

		return nil
	})

	// Close final archive
	if err := a.Close(); err != nil {
		return err
	}

	if err := splitFile.Close(); err != nil {
		return err
	}

	return walkErr
}

// createArchive is a helper function that prepares a new split archive
func createArchive(a Archiver, splitPath string) (splitFile *os.File, err error) {

	// create a new target file
	splitFile, err = os.Create(splitPath)
	if err != nil {
		return nil, fmt.Errorf("creating %s: %v", splitPath, err)
	}

	// Create a new tar archive for writing
	logrus.Infof("Creating archive %s", splitPath)
	if a.Create(splitFile); err != nil {
		return nil, fmt.Errorf("creating archive %s: %v", splitPath, err)
	}

	return splitFile, nil
}
