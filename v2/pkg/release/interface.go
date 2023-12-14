package release

import (
	"context"

	"github.com/google/uuid"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
)

type CollectorInterface interface {
	ReleaseImageCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error)
	// Returns the graphImage if generated, especially with the reference to the image
	// on the destination repository
	// Returns an error if the graph image was not generated (graph : false)
	// This is especially useful for generating UpdateService during
	// DiskToMirror workflow
	GraphImage() (string, error)
	// Returns "a" release image that was mirrored
	// This is especially useful for generating UpdateService during
	// DiskToMirror workflow when `graph : true`.
	// The reason this method returns any release image is because
	// the updateServiceGenerator is only interrested in the repository
	// of the image, rather than the exact digest or tag.
	// This works because oc-mirror doesn't know how to mix OKD and OCP
	// release mirroring.
	ReleaseImage() (string, error)
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
