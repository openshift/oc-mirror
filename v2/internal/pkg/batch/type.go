package batch

import (
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

type ProgressStruct struct {
	Log clog.PluggableLoggerInterface
}

type StringMap map[string]string

type mirrorErrorSchema struct {
	image     v2alpha1.CopyImageSchema
	err       error
	operators map[string]struct{}
	bundles   StringMap
}

func (e mirrorErrorSchema) Error() string {
	return e.err.Error()
}
