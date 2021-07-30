package config

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

func WriteMetadata(metadata *v1alpha1.Metadata, rootDir string) error {

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
