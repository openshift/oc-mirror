package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImageSetConfiguration object kind.
const ImageSetConfigurationKind = "ImageSetConfiguration"

// ImageSetConfiguration configures image set creation.
type ImageSetConfiguration struct {
	metav1.TypeMeta `json:",inline"`
	// ImageSetConfigurationSpec defines the global configuration for an imageset.
	ImageSetConfigurationSpec `json:",inline"`
}

// ImageSetConfigurationSpec defines the global configuration for an imageset.
type ImageSetConfigurationSpec struct {
	// Mirror defines the configuration for content types within the imageset.
	Mirror Mirror `json:"mirror"`
	// ArchiveSize is the size of the segmented archive in GB
	ArchiveSize int64 `json:"archiveSize,omitempty"`
	// StorageConfig for reading/writing metadata and files.
	StorageConfig StorageConfig `json:"storageConfig"`
}

// Mirror defines the configuration for content types within the imageset.
type Mirror struct {
	// OCP defines the configuration for OCP and OKD content types.
	OCP OCP `json:"ocp,omitempty"`
	// Operators defines the configuration for Operator content types.
	Operators []Operator `json:"operators,omitempty"`
	// AdditionalImages defines the configuration for a list
	// of individual image content types.
	AdditionalImages []AdditionalImages `json:"additionalImages,omitempty"`
	// Helm define the configuration for Helm content types.
	Helm Helm `json:"helm,omitempty"`
	// BlockedImages define a list of images that will be blocked
	// from the mirroring process if they exist in other content
	// types in the configuration.
	BlockedImages []BlockedImages `json:"blockedImages,omitempty"`
	// Samples defines the configuration for Sample content types.
	// This is currently not implemented.
	Samples []SampleImages `json:"samples,omitempty"`
}

// OCP defines the configuration for OCP and OKD content types.
type OCP struct {
	// Graph defines whether Cincinnati graph data will
	// downloaded and publish
	Graph bool `json:"graph,omitempty"`
	// Channels defines the configuration for individual
	// OCP and OKD channels
	Channels []ReleaseChannel `json:"channels,omitempty"`
}

// ReleaseChannel defines the configuration for individual
// OCP and OKD channels
type ReleaseChannel struct {
	Name string `json:"name"`
	// MinVersion is minimum version in the
	// release channel to mirror
	MinVersion string `json:"minVersion"`
	// MaxVersion is maximum version in the
	// release channel to mirror
	MaxVersion string `json:"maxVersion"`
	// ShortestPath mode calculates the shortest path
	// between the min and mav version
	ShortestPath bool `json:"shortestPath,omitempty"`
	// AllVersions mode set the MinVersion to the
	// first release in the channel and the MaxVersion
	// to the last release in the channel.
	AllVersions bool `json:"allVersions,omitempty"`
}

// IsHeadsOnly determine if the mode set mirrors only channel head.
// Setting MaxVersion will override this setting.
func (r ReleaseChannel) IsHeadsOnly() bool {
	return !r.AllVersions
}

// Operator defines the configuration for operator catalog mirroring.
type Operator struct {
	// Mirror specific operator packages, channels, and versions, and their dependencies.
	// If HeadsOnly is true, these objects are mirrored on top of heads of all channels.
	// Otherwise, only these specific objects are mirrored.
	IncludeConfig `json:",inline"`

	// Catalog image to mirror. This image must be pullable and available for subsequent
	// pulls on later mirrors.
	// This image should be an exact image pin (registry/namespace/name@sha256:<hash>)
	// but is not required to be.
	Catalog string `json:"catalog"`
	// allPackages defines whether all packages within the catalog
	// or specified IncludeConfig will be mirrored or just channel heads.
	AllPackages bool `json:"allPackages,omitempty"`
	// SkipDependencies will not include dependencies
	// of bundles included in the diff if true.
	SkipDependencies bool `json:"skipDeps,omitempty"`
}

// IsHeadsOnly determine if the mode set mirrors only channel heads of all packages in the catalog.
// Channels specified in DiffIncludeConfig will override this setting;
// heads will still be included, but prior versions may also be included.
func (o Operator) IsHeadsOnly() bool {
	return !o.AllPackages
}

// Helm defines the configuration for Helm chart download
// and image mirroring
type Helm struct {
	// Repo is the helm repository containing the charts
	Repos []Repo `json:"repos,omitempty"`
	// Local is the configuration for locally stored helm charts
	Local []Chart `json:"local,omitempty"`
}

// Repo defines the configuration for a Helm repo.
type Repo struct {
	// URL is the url of the Helm repository
	URL string `json:"url"`
	// Name is the name of the Helm repository
	Name string `json:"name"`
	// Charts is a list of charts to pull from the repo
	Charts []Chart `json:"charts"`
}

// Chart is the information an individual Helm chart
type Chart struct {
	// Chart is the chart name as define
	// in the Chart.yaml or in the Helm repo.
	Name string `json:"name"`
	// Version is the chart version as define in the
	// Chart.yaml or in the Helm repo.
	Version string `json:"version,omitempty"`
	// Path defines the path on disk where the
	// chart is stored.
	// This is applicable for a local chart.
	Path string `json:"path,omitempty"`
	// ImagePaths are custom JSON paths for images location
	// in the helm manifest or templates
	ImagePaths []string `json:"imagepaths,omitempty"`
}

// Image contains image pull information.
type Image struct {
	// Name of the image. This should be an exact image pin (registry/namespace/name@sha256:<hash>)
	// but is not required to be.
	Name string `json:"name"`
}

// AdditionalImages defines the configuration
// individual image content types.
type AdditionalImages struct {
	Image `json:",inline"`
}

// BlockedImages the configuration for images that will be blocked
// from the mirroring process if they exist in other content
// types in the configuration.
type BlockedImages struct {
	Image `json:",inline"`
}

// SampleImages define the configuration
// for Sameple content types.
type SampleImages struct {
	Image `json:",inline"`
}
