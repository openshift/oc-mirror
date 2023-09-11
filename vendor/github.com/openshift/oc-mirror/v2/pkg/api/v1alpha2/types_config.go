package v1alpha2

import (
	"fmt"
	"strings"

	"github.com/openshift/library-go/pkg/image/reference"
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
	// Platform defines the configuration for OpenShift and OKD platform types.
	Platform Platform `json:"platform,omitempty"`
	// Operators defines the configuration for Operator content types.
	Operators []Operator `json:"operators,omitempty"`
	// AdditionalImages defines the configuration for a list
	// of individual image content types.
	AdditionalImages []Image `json:"additionalImages,omitempty"`
	// Helm define the configuration for Helm content types.
	Helm Helm `json:"helm,omitempty"`
	// BlockedImages define a list of images that will be blocked
	// from the mirroring process if they exist in other content
	// types in the configuration.
	BlockedImages []Image `json:"blockedImages,omitempty"`
	// Samples defines the configuration for Sample content types.
	// This is currently not implemented.
	Samples []SampleImages `json:"samples,omitempty"`
}

// Platform defines the configuration for OpenShift and OKD platform types.
type Platform struct {
	// Graph defines whether Cincinnati graph data will
	// downloaded and publish
	Graph bool `json:"graph,omitempty"`
	// Channels defines the configuration for individual
	// OCP and OKD channels
	Channels []ReleaseChannel `json:"channels,omitempty"`
	// Architectures defines one or more architectures
	// to mirror for the release image. This is defined at the
	// platform level to enable cross-channel upgrades.
	Architectures []string `json:"architectures,omitempty"`
	// This new field will allow the diskToMirror functionality
	// to copy from a release location on disk
	Release string `json:"release,omitempty"`
}

// ReleaseChannel defines the configuration for individual
// OCP and OKD channels
type ReleaseChannel struct {
	Name string `json:"name"`
	// Type of the platform in the context of this tool.
	// See the PlatformType enum for options. OCP is the default.
	Type PlatformType `json:"type"`
	// MinVersion is minimum version in the
	// release channel to mirror
	MinVersion string `json:"minVersion,omitempty"`
	// MaxVersion is maximum version in the
	// release channel to mirror
	MaxVersion string `json:"maxVersion,omitempty"`
	// ShortestPath mode calculates the shortest path
	// between the min and mav version
	ShortestPath bool `json:"shortestPath,omitempty"`
	// Full mode set the MinVersion to the
	// first release in the channel and the MaxVersion
	// to the last release in the channel.
	Full bool `json:"full,omitempty"`
}

// IsHeadsOnly determine if the mode set mirrors only channel head.
// Setting MaxVersion will override this setting.
func (r ReleaseChannel) IsHeadsOnly() bool {
	return !r.Full
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
	// TargetName is the target image name the catalog will be built with. If unset,
	// the catalog will be published with the provided name in the Catalog
	// field.
	TargetName string `json:"targetName,omitempty"`
	// TargetTag is the tag the catalog image will be built with. If unset,
	// the catalog will be publish with the provided tag in the Catalog
	// field or a tag calculated from the partial digest.
	TargetTag string `json:"targetTag,omitempty"`
	// Full defines whether all packages within the catalog
	// or specified IncludeConfig will be mirrored or just channel heads.
	Full bool `json:"full,omitempty"`
	// SkipDependencies will not include dependencies
	// of bundles included in the diff if true.
	SkipDependencies bool `json:"skipDependencies,omitempty"`
	// OriginalRef is used when the Catalog is an OCI FBC (File Based Catalog) location.
	// It contains the reference to the original repo on a remote registry
	// Deprecated in oc-mirror 4.13, and will no longer be used.
	OriginalRef string `json:"originalRef,omitempty"`
}

// GetUniqueName determines the catalog name that will
// be tracked in the metadata and built. This depends on what fields
// are set between Catalog, TargetName, and TargetTag.
func (o Operator) GetUniqueName() (string, error) {
	ctlgRef := o.Catalog
	if o.IsFBCOCI() {
		ctlgRef = strings.TrimPrefix(ctlgRef, "oci:")
		ctlgRef = strings.TrimPrefix(ctlgRef, "//") //it could be that there is none
		ctlgRef = strings.TrimPrefix(ctlgRef, "/")  // case of full path
	}
	if o.TargetName == "" && o.TargetTag == "" {
		return ctlgRef, nil
	}

	catalogRef, err := reference.Parse(ctlgRef)
	if err != nil {
		return "", fmt.Errorf("error parsing source catalog %s: %v", catalogRef, err)
	}
	if o.TargetName != "" {
		catalogRef.Name = o.TargetName
	}
	if o.TargetTag != "" {
		catalogRef.ID = ""
		catalogRef.Tag = o.TargetTag
	}

	return catalogRef.Exact(), nil
}

// IsHeadsOnly determine if the mode set mirrors only channel heads of all packages in the catalog.
// Channels specified in DiffIncludeConfig will override this setting;
// heads will still be included, but prior versions may also be included.
func (o Operator) IsHeadsOnly() bool {
	return !o.Full
}

func (o Operator) IsFBCOCI() bool {
	return strings.HasPrefix(o.Catalog, "oci:")
}

// Helm defines the configuration for Helm chart download
// and image mirroring
type Helm struct {
	// Repositories are the Helm repositories containing the charts
	Repositories []Repository `json:"repositories,omitempty"`
	// Local is the configuration for locally stored helm charts
	Local []Chart `json:"local,omitempty"`
}

// Repository defines the configuration for a Helm repository.
type Repository struct {
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
	ImagePaths []string `json:"imagePaths,omitempty"`
}

// Image contains image pull information.
type Image struct {
	// Name of the image. This should be an exact image pin (registry/namespace/name@sha256:<hash>)
	// but is not required to be.
	Name string `json:"name"`
}

// SampleImages define the configuration
// for Sameple content types.
// Not implemented.
type SampleImages struct {
	Image `json:",inline"`
}
