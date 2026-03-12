package operator

import (
	"context"

	"github.com/operator-framework/operator-registry/alpha/declcfg"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type CollectorInterface interface {
	OperatorImageCollector(ctx context.Context) (v2alpha1.CollectorSchema, error)
}

type catalogHandlerInterface interface {
	GetDeclarativeConfig(ctx context.Context, filePath string) (*declcfg.DeclarativeConfig, error)
	getRelatedImagesFromCatalog(dc *declcfg.DeclarativeConfig, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) (map[string][]v2alpha1.RelatedImage, error)
	EnsureCatalogInOCIFormat(ctx context.Context, imgSpec image.ImageSpec, catalog, imageIndexDir string, opts mirror.CopyOptions) error
	ExtractOCIConfigLayers(imgSpec image.ImageSpec, imageIndexDir string) (string, error)
}

type imageDispatcher interface {
	dispatch(image v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error)
}
