package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

// Define error for metadata managment
type MetadataError struct {
	Path string
	Type string
}

func (e *MetadataError) Error() string {
	if e.Type == "diff" {
		return fmt.Sprintf("no metadata found at %s, please run a create full", e.Path)
	} else {
		return fmt.Sprintf("metadata found at %s, please run a create diff", e.Path)
	}
}

// WriteMetadata write provided metadata to disk
func WriteMetadata(metadata v1alpha1.Metadata, rootDir string) error {

	metadataPath := filepath.Join(rootDir, metadataBasePath)

	data, err := json.Marshal(metadata)

	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(metadataPath, data, 0640); err != nil {
		return err
	}

	return nil
}

// NewFullMetadtaError is a helper function for returning a
// MetadataError for full bundles
func NewFullMetadataError(fpath string) error {
	return &MetadataError{Path: fpath, Type: "full"}
}

// NewDiffMetadtaError is a helper function for returning a
// MetadataError for diff bundles
func NewDiffMetadataError(fpath string) error {
	return &MetadataError{Path: fpath, Type: "diff"}
}
