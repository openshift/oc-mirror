package config

import (
	"path/filepath"
)

/*
Constants defined here refer to this workspace layout:

	/tmp/cwd/oc-mirror-workspace
	├── results-1675904745
	└── src
		├── catalogs
		│   └── icr.io                                                                              ─┐
		│       └── cpopen                                                                           ├─ catalog path (one per catalog)
		│           └── ibm-zcon-zosconnect-catalog                                                  │
		│               └── sha256:6f02ecef46020bcd21bdd24a01f435023d5fc3943972ef0d9769d5276e178e76 ─┘
		│                   ├── include-config.gob         <—— this represents v1alpha2.IncludeConfig for "single architecture image" ─┐
		│                   │                                  (i.e. no multi arch use case)                                           │
		│                   ├── index                      <—— this represents a decl config for "single architecture image" image     ├─ single arch
		│                   │   │                              (i.e. no multi arch use case)                                           │
		│                   │   └── index.json             <—— Declarative Config                                                     ─┘
		│                   ├── multi                      <—— if present, then the above "single architecture image" ─┐
		│                   │   │                              files/folders won't be present                          │
		│                   │   ├── linux-amd64            <—— this represents a "amd64" image                         │
		│                   │   │   ├── include-config.gob <—— this represents v1alpha2.IncludeConfig for amd64        │
		│                   │   │   └── index              <—— this represents a decl config for "amd64" image         │
		│                   │   │       └── index.json     <—— Declarative Config                                      │
		│                   │   ├── linux-ppc64le          <—— this represents a "ppc64le" image                       ├─ multi arch
		│                   │   │   ├── include-config.gob <—— this represents v1alpha2.IncludeConfig for ppc64le      │
		│                   │   │   └── index              <—— this represents a decl config for "ppc64le" image       │
		│                   │   │       └── index.json     <—— Declarative Config                                      │
		│                   │   └── linux-s390x            <—— this represents a "s390x" image                         │
		│                   │       ├── include-config.gob <—— this represents v1alpha2.IncludeConfig for s390x        │
		│                   │       └── index              <—— this represents a decl config for "s390x" image         │
		│                   │           └── index.json     <—— Declarative Config                                     ─┘
		│                   └── layout                     <—— OCI layout is capable of holding “manifest list” and “single architecture" images
		│                       ├── blobs
		│                       │   └── sha256
		│                       │       ├── 01c6d5dcde3e9f2de758d794a77974600fe5e0b7a8c2ce2833eede2e8b25e7e5
		│                       │       ├── 1cd0595314a53d179ddaf68761c9f40c4d9d1bcd3f692d1c005938dac2993db6
		│                       │       ├── 1ff4ea896d6b958aa060e04eb090f70c563ad0650e6b362c1a1d67582acb3b8e
		│                       │       ├── 25d123725cf91c20b497ca9dae8e0a6e8dedd8fe64f83757f3b41f6ac447eac0
		│                       │       ├── 2d55550cefe39606cc933a2ed1d242b3fd9a72d85e92a2c6b52b9623a6f4fe6a
		│                       │       ├── 34e12aa195bcd8366fbab95a242e8214ae959259bd0a1119c28d124f5799f502
		│                       │       ├── 49a32e2e950732d5a638d5486968dcc3096f940a94811cfec99bd5a4f9e1ad49
		│                       │       ├── 561bc8bee264b124ba5673e73ad36589abbecf0b15fb845ed9aab4e640989fbc
		│                       │       ├── 6672e188b9c3f7274b7ebf4498b34f951bc20ea86a8d72367eab363f1722d2ed
		│                       │       ├── 7062267a99d0149e4129843a9e3257b882920fb8554ec2068a264b37539768bc
		│                       │       ├── ae475359a3fb8fe5e3dff0626f0c788b94340416eb5c453339abc884ac86b671
		│                       │       ├── b2dd6105dc025aa5c5e8b75e0b2d8a390951369801d049fc0de2586917a42772
		│                       │       ├── c03c8f94bb495320bbe862bc69349dbdf9f2a29b83d5b344b3930890aaf89d7d
		│                       │       ├── dc1b9846d7994450e74b9cde2e621f8c1d98cdf1debd591db291785dd3fc6446
		│                       │       └── f89d6e2463fc5fff8bba9e568ec28a6030076dbc412bd52dff6dbf2b5897a59d
		│                       ├── index.json
		│                       └── oci-layout
		├── charts
		├── publish
		│   └── .metadata.json
		├── release-signatures
		└── v2
*/
const (
	// DefaultWorkspaceName defines the default value for the workspace if not provided by the user
	DefaultWorkspaceName = "oc-mirror-workspace"
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
	/*
		IndexDir is the location of the file-based catalog json file.
		This folder can be located at:

		• the root of the <catalog path> for single architecture catalogs

		• one or more architecture specific path(s) <catalog path>/multi/<architecture> for multi architecture catalogs

		Single and multi architecture index locations are mutually exclusive
		(i.e. you get one or the other and not both)
	*/
	IndexDir = "index"
	// MultiDir is present when the catalog is a multi architecture catalog.
	// This will not be present if the catalog is a single architecture catalog.
	MultiDir = "multi"
	// IncludeConfigFile is the file where
	// catalog include config data for incorporation
	// into the metadata is located.
	IncludeConfigFile = "include-config.gob"
)

// MetadataBasePath is the local path relative to the oc-mirror workspace
// where metadata is stored.
var MetadataBasePath = filepath.Join(PublishDir, MetadataFile)
