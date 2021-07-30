package v1alpha1

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Metadata object kind.
const MetadataKind = "Metadata"

const MetadataAPIVersion = "tmp-redhatgov.com/v1alpha1"

// Metadata configures image set creation.
type Metadata struct {
	metav1.TypeMeta `json:",inline"`

	MetadataSpec `json:",inline"`
}

type MetadataSpec struct {
	PastMirrors []PastMirror `json:"pastMirrors"`
}

type PastMirror struct {
	Timestamp int       `json:"timestamp"`
	Sequence  int       `json:"sequence"`
	Uid       uuid.UUID `json:"uid"`
	Files     []File    `json:"files"`
	Mirror    Mirror    `json:"mirror"`
}

type File struct {
	Name string `json:"name"`
}

func NewMetadata() *Metadata {
	return &Metadata{
		TypeMeta: metav1.TypeMeta{
			APIVersion: MetadataAPIVersion,
			Kind:       MetadataKind,
		},
	}

}
func LoadMetadata(data []byte) (m Metadata, err error) {

	gvk := GroupVersion.WithKind(MetadataKind)

	dec := json.NewDecoder(bytes.NewBuffer(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, fmt.Errorf("decode %s: %v", gvk, err)
	}

	m.SetGroupVersionKind(gvk)

	return m, nil
}
