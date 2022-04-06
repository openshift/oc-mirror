package image

import (
	"fmt"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

// ConvertToAssociationSet will return an AssociationSet from a slice of Associations
func ConvertToAssociationSet(assocs []v1alpha2.Association) (AssociationSet, error) {
	assocSet := AssociationSet{}

	assocMapping := make(map[string]v1alpha2.Association, len(assocs))
	var errs []error
	for _, a := range assocs {
		if err := a.Validate(); err != nil {
			errs = append(errs, err)
			continue
		}
		assocMapping[a.Name] = a
	}
	if len(errs) != 0 {
		return assocSet, utilerrors.NewAggregate(errs)
	}

	// Process and remove all index manifest and the child image manifests
	visited := make(map[string]struct{}, len(assocMapping))
	for _, value := range assocMapping {
		if len(value.ManifestDigests) != 0 {
			if ok := assocSet.SetContainsKey(value.Name); !ok {
				assocSet[value.Name] = make(Associations)
			}
			assocSet.Add(value.Name, value)
			for _, digest := range value.ManifestDigests {
				logrus.Debugf("image %q: processing child manifest %s", value.Name, digest)
				child, ok := assocMapping[digest]
				if !ok {
					return assocSet, fmt.Errorf("invalid associations: association for %q is missing", digest)
				}
				assocSet.Add(value.Name, child)
				visited[child.Name] = struct{}{}
			}
			visited[value.Name] = struct{}{}
		}
	}

	// Process image manifests with no parent
	for _, value := range assocMapping {
		if _, found := visited[value.Name]; !found {
			assocSet.Add(value.Name, value)
		}
	}

	return assocSet, nil
}

// COnvertFromAssociationSet will return a slice of Association from an AssociationSet
func ConvertFromAssociationSet(assocSet AssociationSet) ([]v1alpha2.Association, error) {
	assocs := []v1alpha2.Association{}
	var errs []error
	for _, as := range assocSet {
		for _, a := range as {
			if err := a.Validate(); err != nil {
				errs = append(errs, err)
				continue
			}
			assocs = append(assocs, a)
		}
	}
	return assocs, utilerrors.NewAggregate(errs)
}
