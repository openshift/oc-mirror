package archive

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type MirrorUnArchiver struct {
	UnArchiver
	workingDir   string
	cacheDir     string
	archiveFiles []string
}

func NewArchiveExtractor(archivePath, workingDir, cacheDir string) (MirrorUnArchiver, error) {
	ae := MirrorUnArchiver{
		workingDir: workingDir,
		cacheDir:   cacheDir,
	}
	files, err := os.ReadDir(archivePath)
	if err != nil {
		return MirrorUnArchiver{}, err
	}

	rxp, err := regexp.Compile(archiveFilePrefix + "_[0-9]{6}\\.tar")
	if err != nil {
		return MirrorUnArchiver{}, err
	}
	for _, chunk := range files {
		if rxp.MatchString(chunk.Name()) {
			ae.archiveFiles = append(ae.archiveFiles, filepath.Join(archivePath, chunk.Name()))
		}
	}
	return ae, nil
}

// Unarchive extracts:
// * docker/v2* to cacheDir
// * working-dir to workingDir
func (o MirrorUnArchiver) Unarchive() error {
	// make sure workingDir exists
	if err := os.MkdirAll(o.workingDir, 0755); err != nil {
		return fmt.Errorf("unable to create working dir %q: %w", o.workingDir, err)
	}
	// make sure cacheDir exists
	if err := os.MkdirAll(o.cacheDir, 0755); err != nil {
		return fmt.Errorf("unable to create cache dir %q: %w", o.cacheDir, err)
	}

	for _, chunkPath := range o.archiveFiles {
		if err := o.unarchiveChunkTarFile(chunkPath); err != nil {
			return err
		}
	}

	return nil
}

func (o MirrorUnArchiver) unarchiveChunkTarFile(chunkPath string) error {
	chunkFile, err := os.Open(chunkPath)
	if err != nil {
		return fmt.Errorf("unable to open chunk tar file: %w", err)
	}
	defer chunkFile.Close()
	workingDirParent := filepath.Dir(o.workingDir)
	reader := tar.NewReader(chunkFile)
	for {
		header, err := reader.Next()

		// break the infinite loop when EOF
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("error reading archive %s: %w", chunkFile.Name(), err)
		}

		if header == nil {
			continue
		}

		// taking only files into account because we are considering that all
		// parent folders will be created recursively, and that, to the best of
		// our knowledge the archive doesn't include any symbolic links
		if header.Typeflag != tar.TypeReg {
			continue
		}

		// for the moment we ignore imageSetConfig that is included in the tar
		// as well as any other files that are not working-dir or cache

		descriptor := ""
		// case file belongs to working-dir
		if strings.Contains(header.Name, workingDirectory) {
			workingDirParent := filepath.Dir(o.workingDir)
			descriptor = filepath.Join(workingDirParent, header.Name)
		} else if strings.Contains(header.Name, cacheFilePrefix) {
			// case file belongs to the cache
			descriptor = filepath.Join(o.cacheDir, header.Name)
		} else {
			continue
		}
		// if it's a file create it
		// make sure it's at least writable and executable by the user
		// since with every UnArchive, we should be able to rewrite the file
		if err := writeFile(descriptor, reader, header.FileInfo().Mode()|0755); err != nil {
			return err
		}
	}

	return nil
}

func writeFile(filePath string, reader *tar.Reader, perm os.FileMode) error {
	// make sure all the parent directories exist
	descriptorParent := filepath.Dir(filePath)
	if err := os.MkdirAll(descriptorParent, 0755); err != nil {
		return fmt.Errorf("unable to create parent directory for %s: %w", filePath, err)
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, perm)
	if err != nil {
		return fmt.Errorf("unable to create file %s: %w", filePath, err)
	}
	defer f.Close()

	// copy  contents
	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("error copying file %s: %w", filePath, err)
	}

	return nil
}
