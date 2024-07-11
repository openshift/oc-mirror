package operator

import (
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type ByTypePriority []v2alpha1.CopyImageSchema

func (a ByTypePriority) Len() int      { return len(a) }
func (a ByTypePriority) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByTypePriority) Less(i, j int) bool {
	priority := map[string]int{
		v2alpha1.TypeOperatorRelatedImage.String(): 1,
		v2alpha1.TypeOperatorBundle.String():       2,
		v2alpha1.TypeOperatorCatalog.String():      3,
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
