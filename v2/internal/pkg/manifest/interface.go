package manifest

import (
	"context"

	digest "github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type ManifestInterface interface {
	GetOCIImageIndex(dir string) (*v2alpha1.OCISchema, error)
	GetOCIImageManifest(file string) (*v2alpha1.OCISchema, error)
	ExtractOCILayers(filePath, toPath, label string, oci *v2alpha1.OCISchema) error
	ConvertOCIIndexToSingleManifest(dir string, oci *v2alpha1.OCISchema) error
	GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error)
	GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error)
	ImageDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error)
	ImageManifest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, instanceDigest *digest.Digest) ([]byte, string, error)
}
