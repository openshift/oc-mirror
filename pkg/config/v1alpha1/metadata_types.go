package v1alpha1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Metadata object kind.
const MetadataKind = "Metadata"

// Metadata configures image set creation.
type Metadata struct {
	metav1.TypeMeta `json:",inline"`

	MetadataSpec `json:",inline"`
}

type MetadataSpec struct {
	// Uid uniquely identifies this metadata object.
	Uid uuid.UUID `json:"uid"`
	// SingleUse will ignore the past runs if set to true
	SingleUse bool `json:"singleUse"`
	// PastMirror contains the previous mirrored content
	PastMirror PastMirror `json:"pastMirror"`
	// PastBlobs is a slice containing information for
	// all files created for an imageset
	PastBlobs Blobs `json:"pastBlobs"`
}

type PastMirror struct {
	Timestamp int        `json:"timestamp"`
	Sequence  int        `json:"sequence"`
	Manifests []Manifest `json:"manifests"`
	Blobs     Blobs      `json:"blobs"`
	Mirror    Mirror     `json:"mirror"`
	// Operators are metadata about the set of mirrored operators in a mirror operation.
	Operators []OperatorMetadata `json:"operators,omitempty"`
}

type Blob struct {
	ID string `json:"id"`
	// NamespaceName of image that owns this blob.
	// Required for blob lookups during the publish step.
	NamespaceName string `json:"namespaceName"`
	TimeStamp     int    `json:"timestamp"`
}

var _ sort.Interface = Blobs{}

// Blobs is a sortable slice of Blob.
type Blobs []Blob

func (b Blobs) Len() int           { return len(b) }
func (b Blobs) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b Blobs) Less(i, j int) bool { return b[i].TimeStamp < b[j].TimeStamp }

type Manifest struct {
	Name string `json:"name"`
	Tag  string `json:"tag"`
	// NamespaceName of image that owns this manifest.
	NamespaceName string `json:"namespaceName"`
}

// OperatorMetadata holds an Operator's post-mirror metadata.
type OperatorMetadata struct {
	// Catalog references a catalog name from the mirror spec.
	Catalog string `json:"catalog"`
	// ImagePin is the resolved sha256 image name of Catalog.
	// This image will be pulled using the pull secret
	// in the metadata's Mirror config for this catalog.
	ImagePin string `json:"imagePin"`
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

func NewMetadata() Metadata {
	return Metadata{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GroupVersion.String(),
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

	// Make sure blobs are sorted by timestamp
	sort.Sort(sort.Reverse(m.PastMirror.Blobs))

	return m, nil
}

func (m *Metadata) MarshalJSON() ([]byte, error) {

	gvk := GroupVersion.WithKind(MetadataKind)
	m.SetGroupVersionKind(gvk)

	// Make sure blobs are sorted by timestamp
	sort.Sort(sort.Reverse(m.PastMirror.Blobs))

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
