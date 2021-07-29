package v1alpha1

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/operator-framework/operator-registry/pkg/action"
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
	Mirror     Mirror `json:"mirror"`
	PullSecret string `json:"pullSecret,omitempty"`
}

type Mirror struct {
	OCP              OCP                `json:"ocp,omitempty"`
	Operators        []Operator         `json:"operators,omitempty"`
	AdditionalImages []AdditionalImages `json:"additionalImages,omitempty"`
	BlockedImages    []BlockedImages    `json:"blockedImages,omitempty"`
	Samples          []SampleImages     `json:"samples,omitempty"`
}

type OCP struct {
	Graph    bool             `json:"graph,omitempty"`
	Channels []ReleaseChannel `json:"channels,omitempty"`
}

type ReleaseChannel struct {
	Name     string   `json:"name"`
	Versions []string `json:"versions"`
}

type Operator struct {
	action.IncludeCatalog `json:",inline"`
	Catalog               string `json:"catalog"`
	LatestOnly            bool   `json:"latestOnly,omitempty"`
}

type AdditionalImages struct {
	Name string `json:"name"`
}

type BlockedImages struct {
	Name string `json:"name"`
}

type SampleImages struct {
	Name string `json:"name"`
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
