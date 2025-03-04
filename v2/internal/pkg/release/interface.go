package release

import (
	"context"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

//go:generate mockgen -source=./interface.go -destination=./mock/interface_generated.go -package=mock

type CollectorInterface interface {
	ReleaseImageCollector(ctx context.Context) ([]v2alpha1.CopyImageSchema, error)
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
	ReleaseImage(context.Context) (string, error)
}

type GraphBuilderInterface interface {
	CreateGraphImage(ctx context.Context) error
}

type CincinnatiInterface interface {
	GetReleaseReferenceImages(context.Context) ([]v2alpha1.CopyImageSchema, error)
}

type SignatureInterface interface {
	GenerateReleaseSignatures(context.Context, []v2alpha1.CopyImageSchema) ([]v2alpha1.CopyImageSchema, error)
}
