package archive

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestUnArchiver_UnArchive(t *testing.T) {
	testFolder := t.TempDir()
	defer os.RemoveAll(testFolder)

	// Create a new tar archive file
	archiveFileName := fmt.Sprintf("%s_%06d.tar", archiveFilePrefix, 1)
	archivePath := filepath.Join(testFolder, archiveFileName)
	// to be closed by BuildArchive
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("should not fail")
	}
	err = prepareFakeTar(archiveFile)
	if err != nil {
		t.Fatalf("should not fail")
	}

	o, err := NewArchiveExtractor(testFolder, filepath.Join(testFolder, "dst"), filepath.Join(testFolder, "dst"))
	if err != nil {
		t.Fatal(err)
	}
	err = o.Unarchive()
	if err != nil {
		t.Fatal(err)
	}
}

func prepareFakeTar(tarFile *os.File) error {
	workingDirFake := "../../tests/working-dir-fake"
	cacheDirFake := "../../tests/cache-fake"
	tarWriter := tar.NewWriter(tarFile)

	err := filepath.Walk(workingDirFake, func(path string, info os.FileInfo, incomingError error) error {
		if incomingError != nil {
			return incomingError
		}
		if info.IsDir() { // skip directories
			return nil
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		// Use full path as name (FileInfoHeader only takes the basename)
		// If we don't do this the directory strucuture would
		// not be preserved
		// https://golang.org/src/archive/tar/common.go?#L626
		relativePathToAdd, err := filepath.Rel(workingDirFake, path)
		if err != nil {
			return err
		}
		header.Name = filepath.Join("working-dir", relativePathToAdd)

		// Write the header to the tar archive
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Open the file for reading
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Copy the file contents to the tar archive
		if _, err := io.Copy(tarWriter, file); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}
	err = filepath.Walk(cacheDirFake, func(path string, info os.FileInfo, incomingError error) error {
		if incomingError != nil {
			return incomingError
		}
		if info.IsDir() { // skip directories
			return nil
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		// Use full path as name (FileInfoHeader only takes the basename)
		// If we don't do this the directory strucuture would
		// not be preserved
		// https://golang.org/src/archive/tar/common.go?#L626
		header.Name, err = filepath.Rel(cacheDirFake, path)
		if err != nil {
			return err
		}

		// Write the header to the tar archive
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Open the file for reading
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Copy the file contents to the tar archive
		if _, err := io.Copy(tarWriter, file); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
