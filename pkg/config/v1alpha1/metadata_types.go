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
	// Past is a slice containing information for
	// all mirrors created for an imageset
	PastMirrors PastMirrors `json:"pastMirrors"`
	// PastFiles is a slice containing information for
	// all files created for an imageset
	PastBlobs []Blob `json:"pastBlobs"`
}

type PastMirror struct {
	Timestamp int        `json:"timestamp"`
	Sequence  int        `json:"sequence"`
	Manifests []Manifest `json:"manifests"`
	Blobs     []Blob     `json:"blobs"`
	Mirror    Mirror     `json:"mirror"`
	// Operators are metadata about the set of mirrored operators in a mirror operation.
	Operators []OperatorMetadata `json:"operators,omitempty"`
}

var _ sort.Interface = PastMirrors{}

// PastMirrors is a sortable slice of PastMirrors.
type PastMirrors []PastMirror

func (pms PastMirrors) Len() int           { return len(pms) }
func (pms PastMirrors) Swap(i, j int)      { pms[i], pms[j] = pms[j], pms[i] }
func (pms PastMirrors) Less(i, j int) bool { return pms[i].Sequence < pms[j].Sequence }

type Blob struct {
	ID string `json:"id"`
	// NamespaceName of image that owns this blob.
	// Required for blob lookups during the publish step.
	NamespaceName string `json:"namespaceName"`
}

type Manifest struct {
	Name  string `json:"name"`
	Image string `json:"image"`
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

	// Make sure sequences are sorted in ascending order before returning m.
	sort.Sort(m.PastMirrors)

	return m, nil
}

func (m *Metadata) MarshalJSON() ([]byte, error) {

	gvk := GroupVersion.WithKind(MetadataKind)
	m.SetGroupVersionKind(gvk)

	// Make sure sequences are sorted in ascending order before writing m.
	sort.Sort(m.PastMirrors)

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
