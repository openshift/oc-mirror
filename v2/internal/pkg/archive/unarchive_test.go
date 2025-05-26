package archive

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
	"github.com/stretchr/testify/assert"
)

func TestUnArchiver_UnArchive(t *testing.T) {
	t.Run("unarchive with 2 archive: should pass", func(t *testing.T) {
		testFolder := t.TempDir()
		defer os.RemoveAll(testFolder)

		// Create a new tar archive file : for working-dir
		archive1FileName := fmt.Sprintf(archiveFileNameFormat, archiveFilePrefix, 1)
		archive1Path := filepath.Join(testFolder, archive1FileName)
		// to be closed by BuildArchive
		archive1File, err := os.Create(archive1Path)
		if err != nil {
			t.Fatalf("should not fail")
		}
		err = prepareFakeTarWorkingDir(archive1File)
		if err != nil {
			t.Fatalf("should not fail")
		}

		// Create a new tar archive file : for cache-dir
		archive2FileName := fmt.Sprintf(archiveFileNameFormat, archiveFilePrefix, 2)
		archive2Path := filepath.Join(testFolder, archive2FileName)
		// to be closed by BuildArchive
		archive2File, err := os.Create(archive2Path)
		if err != nil {
			t.Fatalf("should not fail")
		}
		err = prepareFakeTarCacheDir(archive2File)
		if err != nil {
			t.Fatalf("should not fail")
		}

		o, err := NewArchiveExtractor(testFolder, filepath.Join(testFolder, "dst", "working-dir"), filepath.Join(testFolder, "dst", "cache-dir"))
		if err != nil {
			t.Fatal(err)
		}
		err = o.Unarchive()
		if err != nil {
			t.Fatal(err)
		}
		assert.DirExists(t, filepath.Join(testFolder, "dst", "working-dir"))
		assert.DirExists(t, filepath.Join(testFolder, "dst", "cache-dir"))
	})

	t.Run("unarchive with 1 archive: should pass", func(t *testing.T) {
		testFolder := t.TempDir()
		defer os.RemoveAll(testFolder)

		// Create a new tar archive file
		archiveFileName := fmt.Sprintf(archiveFileNameFormat, archiveFilePrefix, 1)
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

		o, err := NewArchiveExtractor(testFolder, filepath.Join(testFolder, "dst", "working-dir"), filepath.Join(testFolder, "dst", "cache-dir"))
		if err != nil {
			t.Fatal(err)
		}
		err = o.Unarchive()
		if err != nil {
			t.Fatal(err)
		}

		assert.DirExists(t, filepath.Join(testFolder, "dst", "working-dir"))
		assert.DirExists(t, filepath.Join(testFolder, "dst", "cache-dir"))
	})
}

func TestUnArchiver_NoArchive(t *testing.T) {
	testFolder := t.TempDir()
	defer os.RemoveAll(testFolder)
	workingDir := t.TempDir()
	cacheDir := t.TempDir()
	o, err := NewArchiveExtractor(testFolder, workingDir, cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	err = o.Unarchive()
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnArchiver_WorkingDirError(t *testing.T) {
	testFolder := t.TempDir()
	defer os.RemoveAll(testFolder)

	// Create a new tar archive file
	archiveFileName := fmt.Sprintf(archiveFileNameFormat, archiveFilePrefix, 1)
	archivePath := filepath.Join(testFolder, archiveFileName)
	// to be closed by BuildArchive
	_, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("should not fail")
	}

	o, err := NewArchiveExtractor(testFolder, filepath.Join("/", "dst"), filepath.Join(testFolder, "dst"))
	if err != nil {
		t.Fatal(err)
	}
	err = o.Unarchive()
	assert.Equal(t, "unable to create working dir \"/dst\": mkdir /dst: permission denied", err.Error())
}

func TestUnArchiver_CacheDirError(t *testing.T) {
	testFolder := t.TempDir()
	defer os.RemoveAll(testFolder)

	// Create a new tar archive file
	archiveFileName := fmt.Sprintf(archiveFileNameFormat, archiveFilePrefix, 1)
	archivePath := filepath.Join(testFolder, archiveFileName)
	// to be closed by BuildArchive
	_, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("should not fail")
	}

	o, err := NewArchiveExtractor(testFolder, filepath.Join(testFolder, "dst"), filepath.Join("/", "dst"))
	if err != nil {
		t.Fatal(err)
	}
	err = o.Unarchive()
	assert.Equal(t, "unable to create cache dir \"/dst\": mkdir /dst: permission denied", err.Error())
}

func prepareFakeTarWorkingDir(tarFile *os.File) error {
	tarWriter := tar.NewWriter(tarFile)
	workingDirFake := common.TestFolder + "working-dir-fake"

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
	tarWriter.Close()
	return err
}

func prepareFakeTarCacheDir(tarFile *os.File) error {
	cacheDirFake := common.TestFolder + "cache-fake"
	tarWriter := tar.NewWriter(tarFile)
	err := filepath.Walk(cacheDirFake, func(path string, info os.FileInfo, incomingError error) error {
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
	tarWriter.Close()
	return nil
}

func prepareFakeTar(tarFile *os.File) error {
	err := prepareFakeTarWorkingDir(tarFile)
	if err != nil {
		return err
	}
	err = prepareFakeTarCacheDir(tarFile)
	return err
}
