package operator

import (
	"context"
	"github.com/operator-framework/operator-registry/alpha/declcfg"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type CollectorInterface interface {
	OperatorImageCollector(ctx context.Context) (v2alpha1.CollectorSchema, error)
}

type MetadataPopulator interface {
	PopulateMetadata(ctx context.Context, result v2alpha1.CatalogFilterResult) error
}

type catalogHandlerInterface interface {
	getDeclarativeConfig(filePath string) (*declcfg.DeclarativeConfig, error)
	getCatalog(filePath string) (OperatorCatalog, error)
	filterRelatedImagesFromCatalog(operatorCatalog OperatorCatalog, ctlgInIsc v2alpha1.Operator, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) (map[string][]v2alpha1.RelatedImage, error)
	getRelatedImagesFromCatalog(dc *declcfg.DeclarativeConfig, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) (map[string][]v2alpha1.RelatedImage, error)
	getMetadataPropertyType(dc *declcfg.DeclarativeConfig) string
}

type imageDispatcher interface {
	dispatch(image v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error)
}
