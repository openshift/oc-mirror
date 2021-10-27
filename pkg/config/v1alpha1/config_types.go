package v1alpha1

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/operator-framework/operator-registry/alpha/action"
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
	// Backend is the URI or intended URI where metadata will
	// be stored and retrieved
	// example URI prefixes: "git://" "file://" "s3://"
	Backend string `json:"backend"`
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
	// PullSecret for the release image. Must be a file ref,
	// as this value may be stored unencrypted.
	PullSecret string `json:"pullSecret,omitempty"`
}

type ReleaseChannel struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

// Operator configures operator catalog mirroring.
type Operator struct {
	action.DiffIncludeConfig `json:",inline"`

	// Catalog image to mirror. This image must be pullable and available for subsequent
	// pulls on later mirrors unless WriteIndex or InlineIndex are set.
	// This image should be an exact image pin (registry/namespace/name@sha256:<hash>)
	// but is not required to be.
	Catalog string `json:"catalog"`
	// PullSecret for the image. Must be a file ref, as this value may be stored unencrypted.
	PullSecret string `json:"pullSecret,omitempty"`
	// HeadsOnly mode mirrors only channel heads of all packages in the catalog.
	// Channels specified in DiffIncludeConfig will override this setting;
	// heads will still be included, but prior versions may also be included.
	HeadsOnly bool `json:"headsOnly,omitempty"`
	// InlineIndex directs the mirrorer to store the catalog's declarative config
	// index representation as a file.
	// Only set this option if not using git as a backend, which experiences
	// degraded performance with file sizes of 100MB.
	// InlineIndex and WriteIndex cannot both be set.
	WriteIndex bool `json:"writeIndex,omitempty"`
	// InlineIndex directs the mirrorer to store the catalog's declarative config
	// index representation within the metadata file itself.
	// Only set this option if the index is small or not using git as a backend,
	// which experiences degraded performance with file sizes of 100MB.
	// InlineIndex and WriteIndex cannot both be set.
	InlineIndex bool `json:"inlineIndex,omitempty"`
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
	// PullSecret for the image. Must be a file ref, as this value may be stored unencrypted.
	PullSecret string `json:"pullSecret,omitempty"`
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
