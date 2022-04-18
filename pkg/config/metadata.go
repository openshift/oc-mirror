package config

import (
	"path/filepath"
)

const (
	SourceDir           = "src"
	PublishDir          = "publish"
	InternalDir         = "internal"
	HelmDir             = "charts"
	V2Dir               = "v2"
	BlobDir             = "blobs"
	MetadataFile        = ".metadata.json"
	AssociationsFile    = "image-associations.gob"
	ReleaseSignatureDir = "release-signatures"
	GraphDataDir        = "cincinnati"
	CatalogsDir         = "catalogs"
	LayoutsDir          = "layout"
	IndexDir            = "index"
)

var (
	MetadataBasePath = filepath.Join(PublishDir, MetadataFile)

	// AssociationsBasePath stores image association data in opaque binary format.
	AssociationsBasePath = filepath.Join(InternalDir, AssociationsFile)
)
