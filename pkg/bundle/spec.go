package bundle

import (
	"github.com/google/uuid"
)

// The bundlespec and the directory tree structs live here.

// Directory structure
// CreateDir has one more level then the publish direcotory.
type CreateDir struct {
	Bundle `json:"bundle"`
	Src    `json:"src"`
}

type Bundle struct {
	Publish string `json:"publish"`
	V2      string `json:"v2"`
}

type Src struct {
	Publish string `json:"publish"`
	V2      string `json:"v2"`
}

// BundleSpec
const (
	ApiVersion = "v1alpha1"
)

type BundleSpec struct {
	ApiVersion string `json:"apiVersion"`
	Mirror     `json:"mirror"`
	PullSecret string `json:"pullescret,omitempty"`
}

type Mirror struct {
	AdditionalImages []string `json:"additionalImages,omitempty"`
	BlockedImages    []string `json:"blockedImages,omitempty"`
	Ocp              `json:"ocp,omitempty"`
	Operators        []Operator `json:"operators,omitempty"`
	Samples          []string   `json:"samples,omitempty"`
}

type AdditionalImages struct {
	Name string `json:"name"`
}

type Ocp struct {
	Graph    bool      `json:"graph,omitempty"`
	Channels []channel `json:"channels,omitempty"`
}

type Version struct {
	Version string `json:"version"`
}

type Operator struct {
	Catalog    string    `json:"catalog"`
	Packages   []Package `json:"packages,omitempty"`
	LatestOnly bool      `json:"latestOnly,omitempty"`
}

type Package struct {
	Name       string `json:"name"`
	MinVersion string `json:"minVersion,omitempty"`
}

type channel struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

// Metadata is the data format used to track generated imagesets

type Imageset struct {
	Timestamp int `json:"timestamp"`
	Sequence  int `json:"sequence"`
	Mirror    `json:"mirror"`
	Files     []string  `json:"files"`
	Uid       uuid.UUID `json:"uid"`
}

type File struct {
	Name     string `json:"name"`
	Filename string `json:"filename"`
}

type Imagesets struct {
	Imagesets []Imageset `json:"imagesets"`
}

type RootDir string
