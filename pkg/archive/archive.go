package archive

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

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

type packager struct {
	manifest    map[string]struct{}
	blobs       map[string]struct{}
	packedBlobs map[string]struct{}
	Archiver
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

// NewPackager create a new packager for build ImageSets
func NewPackager(manifests []v1alpha1.Manifest, blobs []v1alpha1.Blob) *packager {
	manifestSetToArchive := make(map[string]struct{}, len(manifests))
	blobSetToArchive := make(map[string]struct{}, len(blobs))

	for _, manifest := range manifests {
		manifestSetToArchive[manifest.Name] = struct{}{}
	}

	for _, blob := range blobs {
		blobSetToArchive[blob.Name] = struct{}{}
	}

	return &packager{
		manifest:    manifestSetToArchive,
		blobs:       blobSetToArchive,
		packedBlobs: make(map[string]struct{}, len(blobs)),
		Archiver:    NewArchiver(),
	}
}

// CreateSplitAchrive will create multiple tar archives from source directory
func (p *packager) CreateSplitArchive(maxSplitSize int64, destDir, sourceDir, prefix string) error {

	// Declare split variables
	splitNum := 0
	splitSize := int64(0)
	splitPath := filepath.Join(destDir, fmt.Sprintf("%s_%06d.%s", prefix, splitNum, p.String()))

	splitFile, err := p.createArchive(splitPath)

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

		// pack the image associations and the metadata
		if filepath.Base(fpath) == config.MetadataFile || filepath.Base(fpath) == config.AssociationsFile {
			p.manifest[fpath] = struct{}{}
		}

		var nameInArchive string

		switch {
		case pack(p.manifest, fpath):
			nameInArchive, err = archiver.NameInArchive(sourceInfo, sourceDir, fpath)
			if err != nil {
				return fmt.Errorf("creating %s: %v", nameInArchive, err)
			}
		case pack(p.blobs, info.Name()) && !pack(p.packedBlobs, info.Name()):
			nameInArchive = blobInArchive(info.Name())
			p.packedBlobs[info.Name()] = struct{}{}

		default:
			logrus.Debugf("File %s will not be archived, skipping...", fpath)
			return nil
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
			p.Close()
			splitFile.Close()

			logrus.Info(splitSize)

			// Increment split number and reset splitSize
			splitNum += 1
			splitSize = int64(0)
			splitPath = filepath.Join(destDir, fmt.Sprintf("%s_%06d.%s", prefix, splitNum, p.String()))

			// Create a new tar archive for writing
			splitFile, err = p.createArchive(splitPath)

			if err != nil {
				return fmt.Errorf("error creating archive %s: %v", splitPath, err)
			}
		}

		// Write file to current archive file
		if err = p.Write(f); err != nil {
			return fmt.Errorf("%s: writing: %s", fpath, err)
		}

		logrus.Debugf("File %s added to archive %s", fpath, splitPath)

		splitSize += info.Size()

		return nil
	})

	// Close final archive
	if err := p.Close(); err != nil {
		return err
	}

	if err := splitFile.Close(); err != nil {
		return err
	}

	return walkErr
}

// createArchive is a helper function that prepares a new split archive
func (p *packager) createArchive(splitPath string) (splitFile *os.File, err error) {

	// create a new target file
	splitFile, err = os.Create(splitPath)
	if err != nil {
		return nil, fmt.Errorf("creating %s: %v", splitPath, err)
	}

	// Create a new tar archive for writing
	logrus.Infof("Creating archive %s", splitPath)
	if p.Create(splitFile); err != nil {
		return nil, fmt.Errorf("creating archive %s: %v", splitPath, err)
	}

	if err != nil {
		return nil, err
	}

	return splitFile, nil
}

func pack(search map[string]struct{}, file string) bool {
	_, archive := search[file]
	return archive
}

func blobInArchive(file string) string {
	return filepath.Join("blobs", file)
}
