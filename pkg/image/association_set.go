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
	as.Add(newKey, values...)
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
	if err := as.Validate(); err != nil {
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

// Validate AssociationSet and all contained Associations
func (as AssociationSet) Validate() error {
	var errs []error
	for imageName, assocs := range as {
		for _, assoc := range assocs {
			if len(assoc.ManifestDigests) != 0 {
				for _, digest := range assoc.ManifestDigests {
					if _, found := assocs[digest]; !found {
						errs = append(errs, fmt.Errorf("image %q: digest %s not found", imageName, digest))
						continue
					}
				}
			}
			if err := assoc.Validate(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return utilerrors.NewAggregate(errs)
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

// ReposForBlobs returns a map with the first repos found
// for each layer digest in the imageset. This can be used
// to pull layers to reform images.
func ReposForBlobs(as AssociationSet) map[string]string {
	reposByBlob := map[string]string{}
	for _, assocs := range as {
		for _, assoc := range assocs {
			for _, dgst := range assoc.LayerDigests {
				if _, found := reposByBlob[dgst]; found {
					continue
				}
				reposByBlob[dgst] = assoc.Path
			}
		}
	}
	return reposByBlob
}

// Prune will return a pruned AssociationSet containing provided keys
func Prune(in AssociationSet, keepKey []string) (AssociationSet, error) {
	// return a new map with the pruned mapping
	pruned := AssociationSet{}
	for _, key := range keepKey {
		assocs, ok := in[key]
		if !ok {
			return pruned, fmt.Errorf("key %s does not exist in provided associations", key)
		}
		pruned[key] = assocs
	}
	return pruned, nil
}
