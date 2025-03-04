package manifest

import (
	"context"

	"github.com/containers/image/v5/types"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

//go:generate mockgen -source=./interface.go -destination=./mock/interface_generated.go -package=mock

type ManifestInterface interface {
	GetImageIndex(dir string) (*v2alpha1.OCISchema, error)
	GetImageManifest(file string) (*v2alpha1.OCISchema, error)
	GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error)
	ExtractLayersOCI(filePath, toPath, label string, oci *v2alpha1.OCISchema) error
	GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error)
	ConvertIndexToSingleManifest(dir string, oci *v2alpha1.OCISchema) error
	GetDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error)
}
