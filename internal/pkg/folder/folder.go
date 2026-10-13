// Package folder contains utilities for working with folders.
package folder

import (
	"errors"
	"os"
)

// CreateFolders creates all the folders in the argument list.
func CreateFolders(paths ...string) error {
	var errs []error
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// RemoveFolders recursive delete all the folders in the argument list.
func RemoveFolders(paths ...string) error {
	var errs []error
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
