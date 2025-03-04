package operator

import (
	"context"

	"github.com/operator-framework/operator-registry/alpha/declcfg"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

//go:generate mockgen -source=./interface.go -destination=./mock/interface_generated.go -package=mock

type CollectorInterface interface {
	OperatorImageCollector(ctx context.Context) (v2alpha1.CollectorSchema, error)
}

type catalogHandlerInterface interface {
	getDeclarativeConfig(filePath string) (*declcfg.DeclarativeConfig, error)
	getCatalog(filePath string) (OperatorCatalog, error)
	filterRelatedImagesFromCatalog(operatorCatalog OperatorCatalog, ctlgInIsc v2alpha1.Operator, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) (map[string][]v2alpha1.RelatedImage, error)
	getRelatedImagesFromCatalog(dc *declcfg.DeclarativeConfig, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) (map[string][]v2alpha1.RelatedImage, error)
}

type imageDispatcher interface {
	dispatch(image v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error)
}
