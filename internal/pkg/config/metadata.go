package config

import (
	"path/filepath"
)

const (
	// SourceDir is the directory that contains
	// all temporary data in the oc-mirror workspace.
	SourceDir = "src"
	// PublishDir is the directory containing
	// a copy of the current metadata.
	PublishDir = "publish"
	// InternalDir is the directory that
	// contains copies of files for internal
	// oc-mirror operations.
	InternalDir = "internal"
	// HelmDir is the directory that contains all
	// downloaded charts.
	HelmDir = "charts"
	// V2Dir is the directory containing images
	// mirrored to disk.
	V2Dir = "v2"
	// BlobsDir is the directory under each image
	// in the V2 directory that contains layers.
	BlobDir = "blobs"
	// MetadataFile is the filename that contains
	// the metadata.
	MetadataFile = ".metadata.json"
	// ReleaseSignatureDir is the top-level
	// directory where platform release-signature
	// configmaps are stored.
	ReleaseSignatureDir = "release-signatures"
	// GraphDataDir is the top-level directory
	// containing cincinnati graph data.
	GraphDataDir = "cincinnati"
	// CatalogsDir is the top-level directory
	// containing all catalog data.
	CatalogsDir = "catalogs"
	// LayoutsDir is the location of the OCI
	// layout directory that contains a copy of the
	// catalog image.
	LayoutsDir = "layout"
	// IndexDir is the location of the
	// file-based catalog json file.
	IndexDir = "index"
	// IncludeConfigFile is the file where
	// catalog include config data for incorporation
	// into the metadata is located.
	IncludeConfigFile = "include-config.gob"
)

// MetadataBasePath is the local path relative to the oc-mirror workspace
// where metadata is stored.
var MetadataBasePath = filepath.Join(PublishDir, MetadataFile)
