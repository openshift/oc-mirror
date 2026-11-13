package archive

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func addFileToWriter(fi fs.FileInfo, pathToFile, pathInTar string, tarWriter *tar.Writer) error {
	header, err := archiveFileHeader(fi, pathInTar)
	if err != nil {
		return err
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}
	// Open the file for reading
	file, err := os.Open(pathToFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Copy the file contents to the tar archive
	if _, err := io.Copy(tarWriter, file); err != nil {
		return err
	}
	return nil
}

// archiveFileHeader is shared by actual archive writers and the size counter.
func archiveFileHeader(info fs.FileInfo, name string) (*tar.Header, error) {
	header, err := tar.FileInfoHeader(info, info.Name())
	if err != nil {
		return nil, fmt.Errorf("create archive entry header: %w", err)
	}
	header.Name = name
	return header, nil
}

func walkArchiveFiles(folder, relativeTo string, add func(fs.FileInfo, string, string) error) error {
	err := filepath.Walk(folder, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name, err := filepath.Rel(relativeTo, path)
		if err != nil {
			return fmt.Errorf("%w", err)
		}
		return add(info, path, name)
	})
	if err != nil {
		return fmt.Errorf("%w", err) // Preserve the existing callback error messages.
	}
	return nil
}

type sizeAdder struct {
	maxSize, payload, total int64
	strict                  bool
	files                   map[string]bool
	pending                 map[string]int64
	pendingPaths            []string // Sorted after all predicted files are collected.
}

// New files inherit the mirroring process's UID/GID. Large IDs require PAX
// headers, just as FileInfoHeader does for existing cache files.
func plannedHeader(name string, size int64) *tar.Header {
	return &tar.Header{Name: name, Size: size, Mode: 0644, Typeflag: tar.TypeReg,
		Uid: os.Getuid(), Gid: os.Getgid(), ModTime: time.Now()}
}

var _ archiveAdder = (*sizeAdder)(nil)

func (a *sizeAdder) addHeader(header *tar.Header) error {
	if a.files[header.Name] {
		return nil
	}
	if header.Size < 0 {
		return fmt.Errorf("unknown size for %s", header.Name)
	}
	oversized := header.Size > a.maxSize
	if oversized && a.strict {
		return fmt.Errorf("file %s exceeds archive size limit", header.Name)
	}
	if oversized || header.Size > a.maxSize-a.payload {
		a.total = AddSize(a.total, 1024)
		if !oversized {
			a.payload = 0
		}
	}
	var headers bytes.Buffer
	writer := tar.NewWriter(&headers)
	if err := writer.WriteHeader(header); err != nil {
		return fmt.Errorf("estimate tar header: %w", err)
	}
	a.total = AddSize(a.total, int64(headers.Len()))
	a.total = AddSize(a.total, header.Size)
	a.total = AddSize(a.total, (512-header.Size%512)%512)
	if !oversized {
		a.payload = AddSize(a.payload, header.Size)
	}
	a.files[header.Name] = true
	return nil
}

func (a *sizeAdder) addFile(path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat archive entry %s: %w", path, err)
	}
	header, err := archiveFileHeader(info, name)
	if err != nil {
		return fmt.Errorf("create archive entry header: %w", err)
	}
	return a.addHeader(header)
}

func (a *sizeAdder) addAllFolder(folder, relativeTo string) error {
	err := walkArchiveFiles(folder, relativeTo, func(info fs.FileInfo, _ string, name string) error {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("cannot estimate non-regular archive entry %s", name)
		}
		header, err := archiveFileHeader(info, name)
		if err != nil {
			return err
		}
		return a.addHeader(header)
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	prefix := folder + string(filepath.Separator)
	start, _ := slices.BinarySearch(a.pendingPaths, prefix)
	for _, path := range a.pendingPaths[start:] {
		if !strings.HasPrefix(path, prefix) {
			break
		}
		name, err := filepath.Rel(relativeTo, path)
		if err != nil {
			return fmt.Errorf("resolve archive entry: %w", err)
		}
		if err := a.addHeader(plannedHeader(name, a.pending[path])); err != nil {
			return err
		}
	}
	return nil
}

func (a *sizeAdder) close() error { return nil }

// AddSize adds nonnegative sizes without overflowing MaxInt64.
func AddSize(a, b int64) int64 {
	a, b = max(a, 0), max(b, 0)
	if b > math.MaxInt64-a {
		return math.MaxInt64
	}
	return a + b
}
