package clusterresources

import (
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

//go:generate mockgen -source=./interface.go -destination=./mock/interface_generated.go -package=mock

type GeneratorInterface interface {
	IDMS_ITMSGenerator(allRelatedImages []v2alpha1.CopyImageSchema, forceRepositoryScope bool) error
	UpdateServiceGenerator(graphImage, releaseImage string) error
	CatalogSourceGenerator(allRelatedImages []v2alpha1.CopyImageSchema) error
	GenerateSignatureConfigMap(allRelatedImages []v2alpha1.CopyImageSchema) error
	ClusterCatalogGenerator(allRelatedImages []v2alpha1.CopyImageSchema) error
}
