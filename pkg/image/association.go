package image

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

// Associations is a map for Association
// searching
type Associations map[string]Association

// AssociationSet is a set of image Associations
// mapped to their images
type AssociationSet map[string]Associations

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

// Search will return all Associations for the specificed key
func (as AssociationSet) Search(key string) (values []Association, found bool) {
	assocs, found := as[key]
	values = make([]Association, len(assocs))
	count := 0
	for _, value := range assocs {
		values[count] = value
		count++
	}
	return
}

// UpdateKey will move values under oldKey to newKey in Assocations.
// Old entries will be deleted.
func (as AssociationSet) UpdateKey(oldKey, newKey string) error {

	// make sure we don't delete the
	// same key we just set
	if newKey == oldKey {
		return nil
	}
	values, found := as.Search(oldKey)
	if !found {
		return errors.New("key does not exist in map")
	}
	for _, value := range values {
		as.Add(newKey, value)
	}
	delete(as, oldKey)

	return nil
}

// UpdateValue will update the Association values for a given key
func (as AssociationSet) UpdateValue(key string, value Association) error {

	assocs, found := as[key]
	if !found {
		return errors.New("key does not exist in map")
	}
	assocs[value.Name] = value

	return nil
}

// Add stores a key-value pair in this multimap.
func (as AssociationSet) Add(key string, value Association) {
	assocs, found := as[key]
	if found {
		assocs[value.Name] = value
	} else {
		assocs = make(Associations)
		assocs[value.Name] = value
		as[key] = assocs
	}
}

// Keys returns all unique keys contained in map
func (as AssociationSet) Keys() []string {
	keys := make([]string, len(as))
	count := 0
	for key := range as {
		keys[count] = key
		count++
	}
	return keys
}

// SetContainsKey checks if the AssociationSet map contains a key
func (as AssociationSet) SetContainsKey(key string) (found bool) {
	_, found = as[key]
	return
}

// ContainsKey checks if the Associations map contains the specified key
func (as AssociationSet) ContainsKey(setKey, key string) (found bool) {
	asSet, found := as[setKey]
	if !found {
		return false
	}
	_, found = asSet[key]
	return
}

// Merge Associations into the receiver.
func (as AssociationSet) Merge(in AssociationSet) {
	for imageName, assocs := range in {
		for _, value := range assocs {
			as.Add(imageName, value)
		}
	}
}

// Encode Associations in an efficient, opaque format.
func (as AssociationSet) Encode(w io.Writer) error {
	if err := as.validate(); err != nil {
		return fmt.Errorf("invalid image associations: %v", err)
	}
	enc := gob.NewEncoder(w)
	if err := enc.Encode(as); err != nil {
		return fmt.Errorf("error encoding image associations: %v", err)
	}
	return nil
}

// Decode Associations from an opaque format. Only useable if Associations
// was encoded with Encode().
func (as *AssociationSet) Decode(r io.Reader) error {
	dec := gob.NewDecoder(r)
	if err := dec.Decode(as); err != nil {
		return fmt.Errorf("error decoding image associations: %v", err)
	}
	// Update paths for local usage.
	for imageName, assocs := range *as {
		for _, assoc := range assocs {
			assoc.Path = filepath.FromSlash(assoc.Path)
			if err := as.UpdateValue(imageName, assoc); err != nil {
				return err
			}
		}
	}
	return nil
}

// UpdatePath path will update path values for local
// AssociationSet use
func (as *AssociationSet) UpdatePath() error {
	// Update paths for local usage.
	for imageName, assocs := range *as {
		for _, assoc := range assocs {
			assoc.Path = filepath.FromSlash(assoc.Path)
			if err := as.UpdateValue(imageName, assoc); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetDigests will return all layer and manifest digests in the association set
func (as *AssociationSet) GetDigests() []string {
	var digests []string
	for _, assocs := range *as {
		for _, assoc := range assocs {
			digests = append(digests, assoc.LayerDigests...)
			digests = append(digests, assoc.ManifestDigests...)
			digests = append(digests, assoc.ID)
		}
	}
	return digests
}

func (as AssociationSet) validate() error {
	var errs []error
	for _, imageName := range as.Keys() {
		assocs, found := as.Search(imageName)
		if !found {
			return fmt.Errorf("image %q does not exist in association set", imageName)
		}
		for _, a := range assocs {

			if s, ok := imageTypeStrings[a.Type]; ok && s != "" {
				continue
			}
			switch a.Type {
			case TypeInvalid:
				// TypeInvalid is the default value for the concrete type, which means the field was not set.
				errs = append(errs, fmt.Errorf("image %q: must set image type", a.Name))
			default:
				errs = append(errs, fmt.Errorf("image %q: unknown image type %v", a.Name, a.Type))
			}

			if len(a.ManifestDigests) != 0 && len(a.LayerDigests) != 0 {
				errs = append(errs, fmt.Errorf("image %q: child descriptors cannot contain both manifests and image layers", a.Name))
			}
			if len(a.ManifestDigests) == 0 && len(a.LayerDigests) == 0 {
				errs = append(errs, fmt.Errorf("image %q: child descriptors must contain at least one manifest or image layer", a.Name))
			}

			if a.ID == "" {
				errs = append(errs, fmt.Errorf("image %q: tag or ID must be set", a.Name))
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}

func GetImageFromBlob(as AssociationSet, digest string) string {
	for imageName, assocs := range as {
		for _, assoc := range assocs {
			for _, dgst := range assoc.LayerDigests {
				if dgst == digest {
					return imageName
				}

			}
		}
	}
	return ""
}
