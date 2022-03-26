package image

import (
	"fmt"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

// COnvertToAssociationSet will return an AssociationSet from a slice of Associations
func ConvertToAssociationSet(assoc []Association) (AssociationSet, error) {
	assocSet := AssociationSet{}

	assocMapping := make(map[string]Association, len(assoc))
	var errs []error
	for _, a := range assoc {
		if err := a.validate(); err != nil {
			errs = append(errs, err)
		}
		assocMapping[a.Name] = a
	}
	if len(errs) != 0 {
		return assocSet, utilerrors.NewAggregate(errs)
	}

	// Process and remove all index manifest and the child image manifests
	for key, value := range assocMapping {
		if len(value.ManifestDigests) != 0 {
			if ok := assocSet.SetContainsKey(value.Name); !ok {
				assocSet[value.Name] = make(Associations)
			}
			assocSet.Add(value.Name, value)
			for _, digest := range value.ManifestDigests {
				child, ok := assocMapping[digest]
				if !ok {
					return assocSet, fmt.Errorf("invalid associations: association for %q is missing", digest)
				}
				assocSet.Add(value.Name, child)
				delete(assocMapping, digest)
			}
			delete(assocMapping, key)
		}
	}

	// Process image manifests with no parent
	for _, value := range assocMapping {
		assocSet.Add(value.Name, value)
	}

	return assocSet, nil
}

// COnvertFromAssociationSet will return a slice of Association from an AssociationSet
func ConvertFromAssociationSet(assocSet AssociationSet) ([]Association, error) {
	assocs := []Association{}
	var errs []error
	for _, as := range assocSet {
		for _, a := range as {
			if err := a.validate(); err != nil {
				errs = append(errs, err)
			}
			assocs = append(assocs, a)
		}
	}
	return assocs, utilerrors.NewAggregate(errs)
}
