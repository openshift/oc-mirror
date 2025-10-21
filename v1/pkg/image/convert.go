package image

import (
	"fmt"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
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

		// The association name itself
		// is not unique because images can
		// share child manifest so using the
		// name and path/image as the key for
		// unique combination.
		assocMapping[a.Name+a.Path] = a
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
				klog.V(4).Infof("image %s: processing child manifest %s", value.Name, digest)
				child, ok := assocMapping[digest+value.Path]
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

// ConvertFromAssociationSet will return a slice of Association from an AssociationSet
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

// ConvertToTypedMapping will return a TypedMappingFrom an AssociationSet
func ConvertToTypedMapping(assocs []v1alpha2.Association) (TypedImageMapping, error) {
	mapping := TypedImageMapping{}
	var errs []error
	childManifest := make(map[string]struct{})
	for _, a := range assocs {
		if err := a.Validate(); err != nil {
			errs = append(errs, err)
			continue
		}
		for _, digest := range a.ManifestDigests {
			childManifest[digest] = struct{}{}
		}
	}

	for _, a := range assocs {
		if _, ok := childManifest[a.Name]; ok {
			continue
		}
		typedImg, err := ParseReference(a.Name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		mapping.Add(typedImg, typedImg, a.Type)
	}
	return mapping, utilerrors.NewAggregate(errs)
}
