package v1alpha2

import (
	"bytes"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// ImageSetConfiguration object kind.
const ImageSetConfigurationKind = "ImageSetConfiguration"

// ImageSetConfiguration configures image set creation.
type ImageSetConfiguration struct {
	metav1.TypeMeta `json:",inline"`

	ImageSetConfigurationSpec `json:",inline"`
}

type ImageSetConfigurationSpec struct {
	Mirror Mirror `json:"mirror"`
	// ArchiveSize is the size of the segmented archive in GB
	ArchiveSize int64 `json:"archiveSize,omitempty"`
	// StorageConfig for reading/writing metadata and files.
	StorageConfig StorageConfig `json:"storageConfig"`
}

type Mirror struct {
	OCP              OCP                `json:"ocp,omitempty"`
	Operators        []Operator         `json:"operators,omitempty"`
	AdditionalImages []AdditionalImages `json:"additionalImages,omitempty"`
	Helm             Helm               `json:"helm,omitempty"`
	BlockedImages    []BlockedImages    `json:"blockedImages,omitempty"`
	Samples          []SampleImages     `json:"samples,omitempty"`
}

type OCP struct {
	Graph    bool             `json:"graph,omitempty"`
	Channels []ReleaseChannel `json:"channels,omitempty"`
}

type ReleaseChannel struct {
	Name string `json:"name"`
	// MinVersion is minimum version in the
	// release channel to mirror
	MinVersion string `json:"minVersion"`
	// MaxVersion is maximum version in the
	// release channel to mirror
	MaxVersion string `json:"maxVersion"`
}

// Operator configures operator catalog mirroring.
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
	// HeadsOnly mode mirrors only channel heads of all packages in the catalog.
	// Channels specified in DiffIncludeConfig will override this setting;
	// heads will still be included, but prior versions may also be included.
	// The default is true.
	HeadsOnly *bool `json:"headsOnly,omitempty"`
	// SkipDependencies will not include dependencies
	// of bundles included in the diff if true.
	SkipDependencies bool `json:"skipDeps,omitempty"`
}

func (o Operator) IsHeadsOnly() bool {
	if o.HeadsOnly == nil {
		return true
	}
	return *o.HeadsOnly
}

type Helm struct {
	// Repo is the helm repository containing the charts
	Repos []Repo `json:"repos,omitempty"`
	// Local is the configuration for locally stored helm charts
	Local []Chart `json:"local,omitempty"`
}

// Repo is the configuration for a Helm Repo
type Repo struct {
	// URL is the url of the helm repository
	URL string `json:"url"`
	// Name is the name of the helm repository
	Name string `json:"name"`
	// Charts is a list of charts to pull from the repo
	Charts []Chart `json:"charts"`
}

// Chart is the information an individual Helm chart
type Chart struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`
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

type AdditionalImages struct {
	Image `json:",inline"`
}

type BlockedImages struct {
	Image `json:",inline"`
}

type SampleImages struct {
	Image `json:",inline"`
}

func LoadConfig(data []byte) (c ImageSetConfiguration, err error) {

	gvk := GroupVersion.WithKind(ImageSetConfigurationKind)

	if data, err = yaml.YAMLToJSON(data); err != nil {
		return c, fmt.Errorf("yaml to json %s: %v", gvk, err)
	}

	dec := json.NewDecoder(bytes.NewBuffer(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("decode %s: %v", gvk, err)
	}

	c.SetGroupVersionKind(gvk)

	return c, nil
}
