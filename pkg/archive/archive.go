package archive

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
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

// NewArchiver create a new archiver for tar archive manipultation
func NewArchiver() Archiver {
	// Create tar specifically with overrite on false
	return &archiver.TarGz{

		Tar: &archiver.Tar{
			OverwriteExisting:      true,
			MkdirAll:               true,
			ImplicitTopLevelFolder: false,
			StripComponents:        0,
			ContinueOnError:        false,
		},
	}
}

// CreateSplitAchrive will create multiple tar archives from source directory
func CreateSplitArchive(a Archiver, maxSplitSize int64, destDir, sourceDir, prefix string, newFiles []v1alpha1.File) error {

	logrus.Debugf("Found new files %v", newFiles)

	// Declare split variables
	splitNum := 0
	splitSize := int64(0)
	splitPath := fmt.Sprintf("%s/%s_%06d.%s", destDir, prefix, splitNum, a.String())

	splitFile, err := os.Create(splitPath)

	if err != nil {
		return fmt.Errorf("creating %s: %v", splitPath, err)
	}

	// Create a new tar archive for writing
	logrus.Infof("Creating archive %s", splitPath)
	if a.Create(splitFile); err != nil {
		return fmt.Errorf("creating archive %s: %v", splitPath, err)
	}

	sourceInfo, err := os.Stat(sourceDir)

	if err != nil {
		return fmt.Errorf("%s: stat: %v", sourceDir, err)
	}

	fileSetToArchive := make(map[string]struct{}, len(newFiles))
	for _, fpath := range newFiles {
		fileSetToArchive[fpath] = struct{}{}
	}
	// Ignore the current dir.
	fileSetToArchive["."] = struct{}{}

	walkErr := filepath.Walk(sourceDir, func(fpath string, info os.FileInfo, err error) error {

		if err != nil {
			return fmt.Errorf("traversing %s: %v", fpath, err)
		}
		if info == nil {
			return fmt.Errorf("no file info")
		}

		// Make sure the metadata file is always packed
		if strings.Contains(fpath, config.MetadataFile) {
			logrus.Debugf("Packing metadata file %s", fpath)
			fileSetToArchive[fpath] = struct{}{}
		}

		if _, archive := fileSetToArchive[fpath]; !archive {
			logrus.Debugf("File %s should not be archived, skipping...", fpath)
			return nil
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

			// Close current tar archive
			a.Close()
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
	if err := a.Close(); err != nil {
		return err
	}

	if err := splitFile.Close(); err != nil {
		return err
	}

	return walkErr
}

// CombineArchives take a list of archives and combines them into one file
func CombineArchives(in Archiver, out Archiver, destDir, name string, paths ...string) error {

	outPath := filepath.Join(destDir, name)

	// Open new archive
	outFile, err := os.Create(outPath)

	if err != nil {
		return err
	}

	// Create a new tar archive for writing
	if out.Create(outFile); err != nil {
		return err
	}

	for _, p := range paths {

		// Read in the source tar archive
		inFile, err := os.Open(p)

		if err != nil {
			return fmt.Errorf("could not open current file %s: %v", p, err)
		}

		// Open the tar archive for reading
		if err := in.Open(inFile, 0); err != nil {
			return fmt.Errorf("error opening archive %s: %v", p, err)
		}

		// Loop through the files in the source tar
		readErr := func() error {
			for {
				// Read current byte and check if we are at the end of the the file
				f, err := in.Read()
				header := f.Header
				switch {
				case err == io.EOF:
					return nil
				case err != nil:
					return err
				case header == nil:
					continue
				}

				// Write file to current archive
				if err := out.Write(f); err != nil {
					return err
				}
			}
		}()

		if readErr != nil {
			return readErr
		}

		if err := in.Close(); err != nil {
			return err
		}
	}

	if err := out.Close(); err != nil {
		return err
	}

	return nil
}
