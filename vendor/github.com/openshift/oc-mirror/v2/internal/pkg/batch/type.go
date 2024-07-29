package batch

import (
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

type ProgressStruct struct {
	countTotal            int
	countReleaseImages    int
	countOperatorsImages  int
	countAdditionalImages int

	countErrorTotal                 int
	countReleaseImagesErrorTotal    int
	countOperatorsImagesErrorTotal  int
	countAdditionalImagesErrorTotal int

	mirrorMessage string
	Log           clog.PluggableLoggerInterface
}

type StringMap map[string]string

type mirrorErrorSchema struct {
	image     v2alpha1.CopyImageSchema
	err       error
	operators map[string]struct{}
	bundles   StringMap
}
