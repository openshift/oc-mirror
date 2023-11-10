package release

import (
	"context"

	"github.com/google/uuid"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
)

type CollectorInterface interface {
	ReleaseImageCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error)
}

type GraphBuilderInterface interface {
	CreateGraphImage(ctx context.Context) error
}

type CincinnatiInterface interface {
	GetReleaseReferenceImages(context.Context) []v1alpha3.CopyImageSchema
	NewOCPClient(uuid.UUID) (Client, error)
	NewOKDClient(uuid.UUID) (Client, error)
}

type SignatureInterface interface {
	GenerateReleaseSignatures(context.Context, []v1alpha3.CopyImageSchema) ([]v1alpha3.CopyImageSchema, error)
}
