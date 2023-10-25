package release

import (
	"context"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
)

type CollectorInterface interface {
	ReleaseImageCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error)
}

type GraphBuilderInterface interface {
	CreateGraphImage() (string, error)
}
