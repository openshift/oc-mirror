package v1alpha2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MetadataKind object kind.
const MetadataKind = "Metadata"

// Metadata configures image set creation.
type Metadata struct {
	metav1.TypeMeta `json:",inline"`
	// MetadataSpec defines the global specified for Metadata types.
	MetadataSpec `json:",inline"`
}

// MetadataSpec defines the global configuration specified for Metadata types.
type MetadataSpec struct {
	// Uid uniquely identifies this metadata object.
	Uid uuid.UUID `json:"uid"`
	// SingleUse will ignore the past runs if set to true
	SingleUse bool `json:"singleUse"`
	// PastMirror contains the previous mirrored content
	PastMirror PastMirror `json:"pastMirror"`
	// PastAssociations define the history about the set of mirrored images including
	// child manifest and layer digest information
	PastAssociations []Association `json:"pastAssociations,omitempty"`
}

// PastMirror defines the specification for previously mirrored content.
type PastMirror struct {
	// TimeStamp defines when the mirrored was processed.
	Timestamp int `json:"timestamp"`
	// Sequence defines the serial number
	// assigned to the processed mirror.
	Sequence int `json:"sequence"`
	// Mirror defines the mirror defined
	// in the ImageSetConfigurationSpec provided
	// during the mirror processing.
	Mirror Mirror `json:"mirror"`
	// Operators are metadata about the set of mirrored operators in a mirror operation.
	Operators []OperatorMetadata `json:"operators,omitempty"`
	// Platforms are metadata about the set of mirrored platform release channels in a mirror operation.
	Platforms []PlatformMetadata `json:"platforms,omitempty"`
	// Associations are metadata about the set of mirrored images including
	// child manifest and layer digest information
	Associations []Association `json:"associations,omitempty"`
}

// OperatorMetadata holds an Operator's post-mirror metadata.
type OperatorMetadata struct {
	// Catalog references a catalog name from the mirror spec.
	Catalog string `json:"catalog"`
	// ImagePin is the resolved sha256 image name of Catalog.
	// This image will be pulled using the pull secret
	// in the metadata's Mirror config for this catalog.
	ImagePin string `json:"imagePin"`
	// IncludeConfig in OperatorMetadata holds the starting
	// versions of all heads-only mirrored catalogs. It will
	// be validated against the current catalog during each run
	// and updated.
	IncludeConfig `json:",inline"`
}

// PlatformMetadata holds a Platform's post-mirror metadata.
type PlatformMetadata struct {
	// ReleaseChannel references a channel name from the mirror spec.
	ReleaseChannel string `json:"channel"`
	// MinVersion in PlatformMetadata holds the starting
	// versions of all newly mirrored channels. This will
	// be populated the first time a channel is mirrored
	// and copied the remaining runs.
	MinVersion string `json:"minVersion"`
}

var _ io.Writer = &InlinedIndex{}

type InlinedIndex json.RawMessage

func (index *InlinedIndex) Write(data []byte) (int, error) {
	msg := json.RawMessage{}
	if err := msg.UnmarshalJSON(data); err != nil {
		return 0, err
	}
	*index = InlinedIndex(msg)
	return len(data), nil
}

func (index InlinedIndex) MarshalJSON() ([]byte, error) {
	return json.Marshal(index)
}

// NewMetadata returns an empty
// instance of Metadata with the type metadata defined.
func NewMetadata() Metadata {
	return Metadata{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GroupVersion.String(),
			Kind:       MetadataKind,
		},
	}
}

func (m *Metadata) MarshalJSON() ([]byte, error) {

	gvk := GroupVersion.WithKind(MetadataKind)
	m.SetGroupVersionKind(gvk)

	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	// Use anonymous struct to avoid recursive marshal calls.
	var tmp struct {
		metav1.TypeMeta `json:",inline"`
		MetadataSpec    `json:",inline"`
	}
	tmp.TypeMeta = m.TypeMeta
	tmp.MetadataSpec = m.MetadataSpec
	if err := enc.Encode(tmp); err != nil {
		return nil, fmt.Errorf("encode %s: %v", gvk, err)
	}

	return buf.Bytes(), nil
}
