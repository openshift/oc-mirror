package manifest

import (
	"context"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	digest "github.com/opencontainers/go-digest"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type ManifestInterface interface {
	GetOCIImageIndex(file string) (*specv1.Index, error)
	GetOCIImageManifest(file string) (*specv1.Manifest, error)
	GetOCIImageFromIndex(dir string) (gcrv1.Image, error)
	ExtractOCILayers(img gcrv1.Image, toPath, label string) error
	ConvertOCIIndexToSingleManifest(dir string, oci *specv1.Index) error
	GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error)
	GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error)
	ImageDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error)
	ImageManifest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, instanceDigest *digest.Digest) ([]byte, string, error)
}
