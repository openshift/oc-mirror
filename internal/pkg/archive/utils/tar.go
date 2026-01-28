// Package utils contains utilities for dealing with archive files.
package utils

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SanitizeArchivePath checks for filepath traversal attacks when extracting archives.
// see https://github.com/securego/gosec/issues/324#issuecomment-935927967
func SanitizeArchivePath(dir, filePath string) (string, error) {
	v := filepath.Join(dir, filePath)
	// OCPBUGS-57387: use absolute paths otherwise the `.` needs special
	// treatment because of the way Golang handles it after `Clean`
	absV, err := filepath.Abs(v)
	if err != nil {
		return "", fmt.Errorf("get absolute path for %q: %w", v, err)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("get absolute path for %q: %w", dir, err)
	}
	if strings.HasPrefix(absV, absDir+string(os.PathSeparator)) {
		return v, nil
	}
	return "", fmt.Errorf("content filepath is tainted: %s", v)
}

// WriteFile copies bytes from a reader to a file.
func WriteFile(filePath string, reader io.Reader, perm os.FileMode, size int64) error {
	// make sure all the parent directories exist
	descriptorParent := filepath.Dir(filePath)
	if err := os.MkdirAll(descriptorParent, 0o755); err != nil {
		return fmt.Errorf("unable to create parent directory for %s: %w", filePath, err)
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return fmt.Errorf("unable to create file %s: %w", filePath, err)
	}
	defer f.Close()

	// copy contents in chunks to avoid gosec:G110 decompression bomb.
	// https://stackoverflow.com/questions/67327323/g110-potential-dos-vulnerability-via-decompression-bomb-gosec
	const maxChunkSize = 2048
	for size > 0 {
		n, err := io.CopyN(f, reader, maxChunkSize)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("error copying file %s: %w", filePath, err)
		}
		size -= n
	}

	return nil
}

// FilterFn is a function used to filter which files to extract from an archive.
type FilterFn func(header *tar.Header) bool

// UntarWithFilter untars the io reader archive and extracts only files that match `filterFn`.
func UntarWithFilter(stream io.Reader, destDir string, filter FilterFn) error { //nolint:cyclop // FIXME: refactor further
	tarReader := tar.NewReader(stream)
	for {
		header, err := tarReader.Next()

		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("untar: Next() failed: %w", err)
		}

		if !filter(header) {
			continue
		}

		filePath, err := SanitizeArchivePath(destDir, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if header.Name != "./" {
				if err := os.MkdirAll(filePath, 0o755); err != nil {
					return fmt.Errorf("untar: Mkdir() failed: %w", err)
				}
			}
		case tar.TypeReg:
			if err := WriteFile(filePath, tarReader, 0o666, header.Size); err != nil {
				return err
			}
		default:
			// just ignore errors as we are only interested in the FB configs layer
		}
	}
	return nil
}
