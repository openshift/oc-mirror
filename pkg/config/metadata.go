package config

import (
	"path/filepath"
)

const (
	SourceDir        = "src"
	PublishDir       = "publish"
	InternalDir      = "internal"
	MetadataFile     = ".metadata.json"
	AssociationsFile = "image-associations.gob"
)

var (
	MetadataBasePath = filepath.Join(SourceDir, PublishDir, MetadataFile)

	// AssociationsBasePath stores image association data in opaque binary format.
	AssociationsBasePath = filepath.Join(SourceDir, InternalDir, AssociationsFile)
)
