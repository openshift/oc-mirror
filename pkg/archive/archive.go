package archive

import (
	"compress/flate"
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
}

// NewArchiver create a new archiver for tar archive manipultation
func NewArchiver(ext string) (Archiver, error) {

	// Check extestion of target file
	f, err := archiver.ByExtension(ext)

	if err != nil {
		return nil, fmt.Errorf("error parsing type %s for format: %v", ext, err)
	}

	// Create tar
	mytar := &archiver.Tar{
		OverwriteExisting:      true,
		MkdirAll:               true,
		ImplicitTopLevelFolder: false,
		StripComponents:        0,
		ContinueOnError:        false,
	}

	// Check compression type (if using)
	// TODO(jpower): Allow user to specify compression level
	switch v := f.(type) {
	case *archiver.Tar:
		return mytar, nil
	case *archiver.TarGz:
		v.Tar = mytar
		v.CompressionLevel = flate.DefaultCompression
		return v, nil
	default:
		return nil, fmt.Errorf("format does not support customization: %s", f)
	}
}

// CreateSplitAchrive will create multiple tar archives from source directory
func CreateSplitArchive(a Archiver, destDir, prefix string, maxSplitSize int64, sourceDir string) error {

	// Declare split variables
	splitNum := 0
	splitSize := int64(0)
	splitPath := fmt.Sprintf("%s/%s_%06d.%s", destDir, prefix, splitNum, a.String())
	shaPath := fmt.Sprintf("%s/sha256sum.txt", destDir)

	splitFile, err := os.Create(splitPath)

	if err != nil {
		return fmt.Errorf("creating %s: %v", splitPath, err)
	}

	// Create fsha256sum.txt file
	shaFile, err := os.Create(shaPath)

	if err != nil {
		return fmt.Errorf("creating %s: %v", shaPath, err)
	}

	defer shaFile.Close()

	// Create a new tar archive for writing
	logrus.Infof("Creating archive %s", splitPath)
	if a.Create(splitFile); err != nil {
		return fmt.Errorf("creating archive %s: %v", splitPath, err)
	}

	sourceInfo, err := os.Stat(sourceDir)

	if err != nil {
		return fmt.Errorf("%s: stat: %v", sourceDir, err)
	}

	filepath.Walk(sourceDir, func(fpath string, info os.FileInfo, err error) error {

		if err != nil {
			return fmt.Errorf("traversing %s: %v", fpath, err)
		}
		if info == nil {
			return fmt.Errorf("no file info")
		}

		// build the name to be used within the archive
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

			// Current current tar archive
			a.Close()

			// Calculate checksum and append to checksum file
			if err := AppendChecksum(shaFile, splitPath); err != nil {
				return fmt.Errorf("error appending checksum for %s: %v", splitPath, err)
			}

			splitFile.Close()

			// Increment split number and reset splitSize
			splitNum += 1
			splitSize = int64(0)
			splitPath = fmt.Sprintf("%s/%s_%06d.%s", destDir, prefix, splitNum, a.String())

			// Create a new tar archive for writing
			logrus.Infof("Creating archive %s", splitPath)

			splitFile, err = os.Create(splitPath)

			if err != nil {
				return fmt.Errorf("creating %s: %v", splitPath, err)
			}

			if err := a.Create(splitFile); err != nil {
				return fmt.Errorf("creating archive %s: %v", splitPath, err)
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
	a.Close()

	// Calculate checksum and append to checksum file
	if err := AppendChecksum(shaFile, splitPath); err != nil {
		return fmt.Errorf("error appending checksum for %s: %v", splitPath, err)
	}

	splitFile.Close()

	return nil
}

// ExtractArchive will unpack the archive at the specified directory
func ExtractArchive(a Archiver, src, dest string) error {
	return a.Unarchive(src, dest)
}
