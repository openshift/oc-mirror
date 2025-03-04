package additional

import (
	"context"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

//go:generate mockgen -source=./interface.go -destination=./mock/interface_generated.go -package=mock

type CollectorInterface interface {
	AdditionalImagesCollector(ctx context.Context) ([]v2alpha1.CopyImageSchema, error)
}
