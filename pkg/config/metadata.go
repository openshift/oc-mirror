package config

import (
	"path/filepath"
)

const (
	SourceDir        = "src"
	PublishDir       = "publish"
	InternalDir      = "internal"
	HelmDir          = "charts"
	MetadataFile     = ".metadata.json"
	AssociationsFile = "image-associations.gob"
)

var (
	MetadataBasePath = filepath.Join(PublishDir, MetadataFile)

	// AssociationsBasePath stores image association data in opaque binary format.
	AssociationsBasePath = filepath.Join(InternalDir, AssociationsFile)
)
