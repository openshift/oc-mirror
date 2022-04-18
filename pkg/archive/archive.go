package archive

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archiver/v3"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

type Archiver interface {
	String() string
	Archive([]string, string) error
	Extract(string, string, string) error
	Unarchive(string, string) error
	Write(archiver.File) error
	Create(io.Writer) error
	Close() error
	Walk(string, archiver.WalkFunc) error
	Open(io.Reader, int64) error
	Read() (archiver.File, error)
	CheckPath(string, string) error
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
		OverwriteExisting:      true,
		MkdirAll:               true,
		ImplicitTopLevelFolder: false,
		StripComponents:        0,
		ContinueOnError:        false,
	}
}

// NewPackager create a new packager for build ImageSets
func NewPackager(manifests []string, blobs []string) *packager {
	manifestSetToArchive := make(map[string]struct{}, len(manifests))
	blobSetToArchive := make(map[string]struct{}, len(blobs))

	for _, manifest := range manifests {
		manifestSetToArchive[manifest] = struct{}{}
	}

	for _, blob := range blobs {
		blobSetToArchive[blob] = struct{}{}
	}

	return &packager{
		manifest:    manifestSetToArchive,
		blobs:       blobSetToArchive,
		packedBlobs: make(map[string]struct{}, len(blobs)),
		Archiver:    NewArchiver(),
	}
}

// CreateSplitArchive will create multiple tar archives from source directory
func (p *packager) CreateSplitArchive(ctx context.Context, backend storage.Backend, maxSplitSize int64, destDir, sourceDir, prefix string, skipCleanup bool) error {

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

	// write metadata to first archive
	if err := packMetadata(ctx, p, backend); err != nil {
		return fmt.Errorf("writing metadata to archive %s failed: %v", splitPath, err)
	}

	walkErr := filepath.Walk(sourceDir, func(fpath string, info os.FileInfo, err error) error {

		if err != nil {
			return fmt.Errorf("traversing %s: %v", fpath, err)
		}
		if info == nil {
			return fmt.Errorf("no file info")
		}

		// pack the image associations and the metadata
		if includeFile(fpath) {
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
			klog.V(4).Info("File %s will not be archived, skipping...", fpath)
			return nil
		}

		var file io.ReadCloser
		if info.Mode().IsRegular() {
			file, err = os.Open(filepath.Clean(fpath))
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
			if err := p.Close(); err != nil {
				return err
			}
			if err := splitFile.Close(); err != nil {
				return err
			}

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

		// Delete file after written to archive
		if shouldRemove(fpath, info) && !skipCleanup {
			if err := os.Remove(fpath); err != nil {
				return err
			}
		}

		klog.V(4).Info("File %s added to archive", fpath)

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

// Unarchive will extract files unless excluded to destination directory
func Unarchive(a Archiver, source, destination string, excludePaths []string) error {
	// Reconcile files to be unarchived
	var files []string
	err := a.Walk(source, func(f archiver.File) error {
		header, ok := f.Header.(*tar.Header)
		if !ok {
			return fmt.Errorf("expected header to be *tar.Header but was %T", f.Header)
		}
		// Only extract files that are not in the exclude paths
		if !shouldExclude(excludePaths, header.Name) && !f.IsDir() {
			files = append(files, header.Name)
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Extract files that have not been excluded
	for _, f := range files {
		if err := a.Extract(source, f, destination); err != nil {
			return err
		}
	}

	return nil
}

// createArchive is a helper function that prepares a new split archive
func (p *packager) createArchive(splitPath string) (splitFile *os.File, err error) {

	// create a new target file
	splitFile, err = os.Create(filepath.Clean(splitPath))
	if err != nil {
		return nil, fmt.Errorf("creating %s: %v", splitPath, err)
	}

	// Create a new tar archive for writing
	klog.Infof("Creating archive %s", splitPath)
	if err = p.Create(splitFile); err != nil {
		return nil, fmt.Errorf("creating archive %s: %v", splitPath, err)
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

func includeFile(fpath string) bool {
	includeFiles := map[string]struct{}{
		config.InternalDir:         {},
		config.CatalogsDir:         {},
		config.HelmDir:             {},
		config.ReleaseSignatureDir: {},
		config.GraphDataDir:        {},
	}
	split := strings.Split(filepath.Clean(fpath), string(filepath.Separator))
	_, found := includeFiles[split[0]]
	return found
}

func shouldRemove(fpath string, info fs.FileInfo) bool {
	return !includeFile(fpath) && !info.IsDir()
}

// within returns true if sub is within or equal to parent.
func within(parent, sub string) bool {
	rel, err := filepath.Rel(parent, sub)
	if err != nil {
		return false
	}
	return !strings.Contains(rel, "..")
}

// shouldExclude will check whether the files should
// be excluded from the extracting processing
func shouldExclude(exclude []string, file string) bool {
	for _, path := range exclude {
		if within(path, file) {
			return true
		}
	}
	return false
}

func packMetadata(ctx context.Context, arc Archiver, backend storage.Backend) error {

	info, err := backend.Stat(ctx, config.MetadataBasePath)
	if err != nil {
		return err
	}
	file, err := backend.Open(ctx, config.MetadataBasePath)
	if err != nil {
		return err
	}
	defer file.Close()
	f := archiver.File{
		FileInfo: archiver.FileInfo{
			FileInfo:   info,
			CustomName: config.MetadataBasePath,
		},
		ReadCloser: file,
	}
	return arc.Write(f)
}
