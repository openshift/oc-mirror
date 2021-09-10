package archive

import (
	"archive/tar"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/mholt/archiver/v3"
	"github.com/sirupsen/logrus"
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
func NewPackager(manifests []v1alpha1.Manifest, blobs []v1alpha1.Blob) *packager {
	manifestSetToArchive := make(map[string]struct{}, len(manifests))
	blobSetToArchive := make(map[string]struct{}, len(blobs))

	for _, manifest := range manifests {
		manifestSetToArchive[manifest.Name] = struct{}{}
	}

	for _, blob := range blobs {
		blobSetToArchive[blob.ID] = struct{}{}
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

		logrus.Debugf("File %s added to archive", fpath)

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

func includeFile(fpath string) bool {
	split := strings.Split(filepath.Clean(fpath), string(filepath.Separator))
	return split[0] == config.InternalDir || split[0] == config.PublishDir || split[0] == "catalogs"
}

// Copied from mholt archiver repo. Temporary and can
// add back to repo
// Changes include the additional of the excludePath variable and is
// passed to the untarNext function
func Unarchive(a Archiver, source, destination string, excludePaths []string) error {

	if !fileExists(destination) {
		err := mkdir(destination, 0755)
		if err != nil {
			return fmt.Errorf("preparing destination: %v", err)
		}
	}

	file, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("opening source archive: %v", err)
	}
	defer file.Close()

	err = a.Open(file, 0)
	if err != nil {
		return fmt.Errorf("opening tar archive for reading: %v", err)
	}
	defer a.Close()

	for {
		err := untarNext(a, destination, excludePaths)
		if err == io.EOF {
			break
		}
		if err != nil {
			if archiver.IsIllegalPathError(err) {
				log.Printf("[ERROR] Reading file in tar archive: %v", err)
				continue
			}
			return fmt.Errorf("reading file in tar archive: %v", err)
		}
	}

	return nil
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return !os.IsNotExist(err)
}

func mkdir(dirPath string, dirMode os.FileMode) error {
	err := os.MkdirAll(dirPath, dirMode)
	if err != nil {
		return fmt.Errorf("%s: making directory: %v", dirPath, err)
	}
	return nil
}

func untarNext(a Archiver, destination string, exclude []string) error {
	f, err := a.Read()
	if err != nil {
		return err // don't wrap error; calling loop must break on io.EOF
	}
	defer f.Close()

	header, ok := f.Header.(*tar.Header)
	if !ok {
		return fmt.Errorf("expected header to be *tar.Header but was %T", f.Header)
	}

	errPath := a.CheckPath(destination, header.Name)
	if errPath != nil {
		return fmt.Errorf("checking path traversal attempt: %v", errPath)
	}

	// Added change here to check if
	// current path is in the exclusion
	// list
	for _, path := range exclude {
		if within(path, header.Name) {
			return nil
		}

	}

	return untarFile(f, destination, header)
}

func untarFile(f archiver.File, destination string, hdr *tar.Header) error {
	to := filepath.Join(destination, hdr.Name)

	switch hdr.Typeflag {
	case tar.TypeDir:
		return mkdir(to, f.Mode())
	case tar.TypeReg, tar.TypeRegA, tar.TypeChar, tar.TypeBlock, tar.TypeFifo, tar.TypeGNUSparse:
		return writeNewFile(to, f, f.Mode())
	case tar.TypeSymlink:
		return writeNewSymbolicLink(to, hdr.Linkname)
	case tar.TypeLink:
		return writeNewHardLink(to, filepath.Join(destination, hdr.Linkname))
	case tar.TypeXGlobalHeader:
		return nil // ignore the pax global header from git-generated tarballs
	default:
		return fmt.Errorf("%s: unknown type flag: %c", hdr.Name, hdr.Typeflag)
	}
}

func writeNewFile(fpath string, in io.Reader, fm os.FileMode) error {
	err := os.MkdirAll(filepath.Dir(fpath), 0755)
	if err != nil {
		return fmt.Errorf("%s: making directory for file: %v", fpath, err)
	}

	out, err := os.Create(fpath)
	if err != nil {
		return fmt.Errorf("%s: creating new file: %v", fpath, err)
	}
	defer out.Close()

	err = out.Chmod(fm)
	if err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("%s: changing file mode: %v", fpath, err)
	}

	_, err = io.Copy(out, in)
	if err != nil {
		return fmt.Errorf("%s: writing file: %v", fpath, err)
	}
	return nil
}

func writeNewSymbolicLink(fpath string, target string) error {
	err := os.MkdirAll(filepath.Dir(fpath), 0755)
	if err != nil {
		return fmt.Errorf("%s: making directory for file: %v", fpath, err)
	}

	_, err = os.Lstat(fpath)
	if err == nil {
		err = os.Remove(fpath)
		if err != nil {
			return fmt.Errorf("%s: failed to unlink: %+v", fpath, err)
		}
	}

	err = os.Symlink(target, fpath)
	if err != nil {
		return fmt.Errorf("%s: making symbolic link for: %v", fpath, err)
	}
	return nil
}

func writeNewHardLink(fpath string, target string) error {
	err := os.MkdirAll(filepath.Dir(fpath), 0755)
	if err != nil {
		return fmt.Errorf("%s: making directory for file: %v", fpath, err)
	}

	_, err = os.Lstat(fpath)
	if err == nil {
		err = os.Remove(fpath)
		if err != nil {
			return fmt.Errorf("%s: failed to unlink: %+v", fpath, err)
		}
	}

	err = os.Link(target, fpath)
	if err != nil {
		return fmt.Errorf("%s: making hard link for: %v", fpath, err)
	}
	return nil
}

// within returns true if sub is within or equal to parent.
func within(parent, sub string) bool {
	rel, err := filepath.Rel(parent, sub)
	if err != nil {
		return false
	}
	return !strings.Contains(rel, "..")
}
