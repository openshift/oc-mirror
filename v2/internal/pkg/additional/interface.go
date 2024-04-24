package additional

import (
	"context"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type CollectorInterface interface {
	AdditionalImagesCollector(ctx context.Context) ([]v2alpha1.CopyImageSchema, error)
}
