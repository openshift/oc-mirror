package customsort

import (
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type ByTypePriority []v2alpha1.CopyImageSchema

func (a ByTypePriority) Len() int      { return len(a) }
func (a ByTypePriority) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByTypePriority) Less(i, j int) bool {
	priority := map[string]int{
		v2alpha1.TypeOCPReleaseContent.String():    1,
		v2alpha1.TypeKubeVirtContainer.String():    2,
		v2alpha1.TypeOCPRelease.String():           3,
		v2alpha1.TypeCincinnatiGraph.String():      4,
		v2alpha1.TypeOperatorRelatedImage.String(): 5,
		v2alpha1.TypeGeneric.String():              6,
		v2alpha1.TypeHelmImage.String():            7,
		v2alpha1.TypeOperatorBundle.String():       8,
		v2alpha1.TypeOperatorCatalog.String():      9,
	}

	defaultPriority := 0

	priorityI, ok := priority[a[i].Type.String()]
	if !ok {
		priorityI = defaultPriority
	}

	priorityJ, ok := priority[a[j].Type.String()]
	if !ok {
		priorityJ = defaultPriority
	}

	return priorityI < priorityJ
}
