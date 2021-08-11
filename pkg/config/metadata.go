package config

import (
	"path/filepath"
)

const (
	SourceDir    = "src"
	PublishDir   = "publish"
	MetadataFile = ".metadata.json"
)

var MetadataBasePath = filepath.Join(SourceDir, PublishDir, MetadataFile)
