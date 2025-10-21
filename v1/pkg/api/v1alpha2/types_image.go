package v1alpha2

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ImageType defines the content type for mirrored images
type ImageType int

const (
	TypeInvalid ImageType = iota
	TypeOCPRelease
	TypeOCPReleaseContent
	TypeCincinnatiGraph
	TypeOperatorCatalog
	TypeOperatorBundle
	TypeOperatorRelatedImage
	TypeGeneric
)

// ImageTypeString defines the string
// respresentation of every ImageType.
var imageTypeStrings = map[ImageType]string{
	TypeOCPReleaseContent:    "ocpReleaseContent",
	TypeCincinnatiGraph:      "cincinnatiGraph",
	TypeOCPRelease:           "ocpRelease",
	TypeOperatorCatalog:      "operatorCatalog",
	TypeOperatorBundle:       "operatorBundle",
	TypeOperatorRelatedImage: "operatorRelatedImage",
	TypeGeneric:              "generic",
}

var imageStringsType = map[string]ImageType{
	"ocpReleaseContent":    TypeOCPReleaseContent,
	"cincinnatiGraph":      TypeCincinnatiGraph,
	"ocpRelease":           TypeOCPRelease,
	"operatorCatalog":      TypeOperatorCatalog,
	"operatorBundle":       TypeOperatorBundle,
	"operatorRelatedImage": TypeOperatorRelatedImage,
	"generic":              TypeGeneric,
}

// String returns the string representation
// of an Image Type
func (it ImageType) String() string {
	return imageTypeStrings[it]
}

// MarshalJSON marshals the ImageType as a quoted json string
func (it ImageType) MarshalJSON() ([]byte, error) {
	if err := it.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(it.String())
}

// UnmarshalJSON unmarshals a quoted json string to the ImageType
func (it *ImageType) UnmarshalJSON(b []byte) error {
	var j string
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}

	*it = imageStringsType[j]
	return nil
}

func (it ImageType) validate() error {
	if _, found := imageTypeStrings[it]; found {
		return nil
	}
	switch it {
	case TypeInvalid:
		// TypeInvalid is the default value for the concrete type, which means the field was not set.
		return errors.New("must set image type")
	default:
		return fmt.Errorf("unknown image type %v", it)
	}
}

// Association between an image and its children, either image layers or child manifests.
type Association struct {
	// Name of the image.
	Name string `json:"name"`
	// Path to image in new location (archive or registry)
	Path string `json:"path"`
	// ID of the image. Joining this value with "manifests" and Path
	// will produce a path to the image's manifest.
	ID string `json:"id"`
	// TagSymlink of the blob specified by ID.
	// This value must be a filename on disk in the "blobs" dir
	TagSymlink string `json:"tagSymlink"`
	// Type of the image in the context of this tool.
	// See the ImageType enum for options.
	Type ImageType `json:"type"`
	// ManifestDigests of images if the image is a docker manifest list or OCI index.
	// These manifests refer to image manifests by content SHA256 digest.
	// LayerDigests and Manifests are mutually exclusive.
	ManifestDigests []string `json:"manifestDigests,omitempty"`
	// LayerDigests of a single manifest if the image is not a docker manifest list
	// or OCI index. These digests refer to image layer blobs by content SHA256 digest.
	// LayerDigests and Manifests are mutually exclusive.
	LayerDigests []string `json:"layerDigests,omitempty"`
}

// Validate checks that the Association fields are set as expected
func (a Association) Validate() error {

	if len(a.ManifestDigests) != 0 && len(a.LayerDigests) != 0 {
		return fmt.Errorf("image %q: child descriptors cannot contain both manifests and image layers", a.Name)
	}
	if len(a.ManifestDigests) == 0 && len(a.LayerDigests) == 0 {
		return fmt.Errorf("image %q: child descriptors must contain at least one manifest or image layer", a.Name)
	}

	if a.ID == "" && a.TagSymlink == "" {
		return fmt.Errorf("image %q: tag or ID must be set", a.Name)
	}

	return a.Type.validate()
}
