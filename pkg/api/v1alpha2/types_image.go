package v1alpha2

import "fmt"

// ImageType defines the content type for mirrored images
type ImageType int

// String returns the string representation
// of an Image Type
func (it ImageType) String() string {
	return ImageTypeStrings[it]
}

const (
	TypeInvalid ImageType = iota
	TypeOCPRelease
	TypeOperatorCatalog
	TypeOperatorBundle
	TypeOperatorRelatedImage
	TypeGeneric
)

// ImageTypeString defines the string
// respresentation of every ImageType.
var ImageTypeStrings = map[ImageType]string{
	TypeOCPRelease:           "ocpRelease",
	TypeOperatorCatalog:      "operatorCatalog",
	TypeOperatorBundle:       "operatorBundle",
	TypeOperatorRelatedImage: "operatorRelatedImage",
	TypeGeneric:              "generic",
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

// Validates checks that the Association fields are set as expected
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

	if s, ok := ImageTypeStrings[a.Type]; ok && s != "" {
		return nil
	}
	switch a.Type {
	case TypeInvalid:
		// TypeInvalid is the default value for the concrete type, which means the field was not set.
		return fmt.Errorf("image %q: must set image type", a.Name)
	default:
		return fmt.Errorf("image %q: unknown image type %v", a.Name, a.Type)
	}
}
