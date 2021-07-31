package archive

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archiver/v3"
	"github.com/sirupsen/logrus"
)

const (
	bundleBasePath = "bundle"
	srcBasePath    = "src"
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
}

// NewArchiver creates a new archiver for tar archive manipultation
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

// NewCompress create a new compressor far tar archiver compression and decompression
func NewCompressor() *archiver.FileCompressor {

	// Create tar specifically with overrite on false
	return &archiver.FileCompressor{
		Compressor:   &archiver.Gz{},
		Decompressor: &archiver.Gz{},
	}
}

// CreateDiffArchive create an archive by first archive the past mirror data and then
//current data with no-clobber
// FIXME(jpower): This could be more efficient than archiving and reopening.
func CreateDiffArchive(a Archiver, destFile, rootDir, prefix string) error {
	bundleDir := filepath.Join(rootDir, bundleBasePath)
	srcDir := filepath.Join(rootDir, srcBasePath)

	if err := a.Archive([]string{bundleDir}, destFile); err != nil {
		return err
	}

	sourceInfo, err := os.Stat(srcDir)

	if err != nil {
		return fmt.Errorf("%s: stat: %v", srcDir, err)
	}

	file, err := os.Open(destFile)

	if err != nil {
		return err
	}

	a.Open(file, 0)

	return filepath.Walk(srcDir, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("traversing %s: %v", fpath, err)
		}
		if info == nil {
			return fmt.Errorf("no file info")
		}

		// build the name to be used within the archive
		nameInArchive, err := archiver.NameInArchive(sourceInfo, srcDir, fpath)
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

		// Write file to current archive file
		if err = a.Write(f); err != nil {
			return fmt.Errorf("%s: writing: %s", fpath, err)
		}

		// Write empty file to bundle path
		if err := writeBundlePath(info, fpath, bundleDir, srcDir); err != nil {
			return fmt.Errorf("error writing file %s: %v", fpath, err)
		}

		return nil
	})
}

// CreateSplitAchrive will create multiple tar archives from source directory
func CreateSplitArchive(a Archiver, destDir, prefix string, maxSplitSize int64, rootDir string) error {

	base, err := filepath.Abs(rootDir)

	if err != nil {
		return err
	}

	bundleDir := filepath.Join(base, bundleBasePath)

	srcDir := filepath.Join(rootDir, srcBasePath)

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

	sourceInfo, err := os.Stat(srcDir)

	if err != nil {
		return fmt.Errorf("%s: stat: %v", srcDir, err)
	}

	err = filepath.Walk(srcDir, func(fpath string, info os.FileInfo, err error) error {

		if err != nil {
			return fmt.Errorf("traversing %s: %v", fpath, err)
		}
		if info == nil {
			return fmt.Errorf("no file info")
		}

		// build the name to be used within the archive
		nameInArchive, err := archiver.NameInArchive(sourceInfo, srcDir, fpath)
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
			splitFile.Close()

			// Increment split number and reset splitSize
			splitNum += 1
			splitSize = int64(0)
			splitPath = fmt.Sprintf("%s/%s_%06d.%s", destDir, prefix, splitNum, a.String())

			// Create a new tar archive for writing
			splitFile, err = os.Create(splitPath)

			if err != nil {
				return fmt.Errorf("creating %s: %v", splitPath, err)
			}

			logrus.Infof("Creating archive %s", splitPath)

			_, err := os.Stat(splitPath)

			if err != nil {
				return err
			}

			if err := a.Create(splitFile); err != nil {
				return fmt.Errorf("creating archive %s: %v", splitPath, err)
			}

		}

		// Write file to current archive file
		if err = a.Write(f); err != nil {
			return fmt.Errorf("%s: writing: %s", fpath, err)
		}

		// Write empty file to bundle path
		if err := writeBundlePath(info, fpath, bundleDir, srcDir); err != nil {
			return fmt.Errorf("error writing file %s: %v", fpath, err)
		}

		splitSize += info.Size()

		return nil
	})

	// Close final archive
	a.Close()
	splitFile.Close()

	return err
}

// CombineArchives take a list of archives and combines them into one file
// FIXME(jpower): This needs to cleanup up alot
func CombineArchives(name string, paths ...string) error {

	c := NewCompressor()

	var buf bytes.Buffer

	for _, path := range paths {

		outfile := path + "out"

		if err := c.DecompressFile(path, outfile); err != nil {
			return err
		}

		data, err := ioutil.ReadFile(outfile)

		defer os.Remove(outfile)

		if err != nil {
			return fmt.Errorf("error reading file %s: %v", path, err)
		}

		buf.Write(data)
	}

	if err := ioutil.WriteFile(name+".tar", buf.Bytes(), os.ModePerm); err != nil {
		return err
	}

	if err := c.CompressFile(name+".tar", name+".tar.gz"); err != nil {
		return err
	}

	os.Remove(name + ".tar")

	return nil
}

// ExtractArchive will unpack the archive at the specified directory
func ExtractArchive(a Archiver, src, dest string) error {
	return a.Unarchive(src, dest)
}

// writeBundlePAth is a helper function to write an empty file from src to
// the the bundle diretory for history
func writeBundlePath(info os.FileInfo, fpath, bundleDir, srcDir string) error {

	newfile := strings.Split(fpath, srcDir)

	if info.IsDir() {
		os.MkdirAll(filepath.Join(bundleDir, newfile[1]), 0755)
	} else {
		emptyFile, err := os.Create(filepath.Join(bundleDir, newfile[1]))

		if err != nil {
			return fmt.Errorf("could not create file %s: %v", newfile[1], err)
		}

		emptyFile.Close()
	}

	return nil
}
