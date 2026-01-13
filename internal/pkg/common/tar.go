package common

import (
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

// WriteFile copies bytes from reader to a file.
func WriteFile(filePath string, reader io.Reader, perm os.FileMode) error {
	// make sure all the parent directories exist
	descriptorParent := filepath.Dir(filePath)
	if err := os.MkdirAll(descriptorParent, 0o755); err != nil {
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
