package operator

import (
	"context"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type CollectorInterface interface {
	OperatorImageCollector(ctx context.Context) (v2alpha1.CollectorSchema, error)
}
