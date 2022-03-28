package image

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

// Associations is a map for Association
// searching
type Associations map[string]v1alpha2.Association

// AssociationSet is a set of image Associations
// mapped to their images
type AssociationSet map[string]Associations

// Search will return all Associations for the specificed key
func (as AssociationSet) Search(key string) (values []v1alpha2.Association, found bool) {
	assocs, found := as[key]
	values = make([]v1alpha2.Association, len(assocs))
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
func (as AssociationSet) UpdateValue(key string, value v1alpha2.Association) error {

	assocs, found := as[key]
	if !found {
		return errors.New("key does not exist in map")
	}
	assocs[value.Name] = value

	return nil
}

// Add stores a key-value pair in this multimap.
func (as AssociationSet) Add(key string, values ...v1alpha2.Association) {
	assocs, found := as[key]
	for _, value := range values {
		if found {
			assocs[value.Name] = value
		} else {
			assocs = make(Associations)
			assocs[value.Name] = value
			as[key] = assocs
		}
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

// GetDigests will return all layer and manifest digests in the AssociationSet
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

// GetImageFromBlob will search the AssociationSet for a blob and return the first
// found image it is associated to
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

func (as AssociationSet) validate() error {
	var errs []error
	for _, imageName := range as.Keys() {
		assocs, found := as.Search(imageName)
		if !found {
			return fmt.Errorf("image %q does not exist in association set", imageName)
		}
		for _, a := range assocs {

			if err := a.Validate(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}
