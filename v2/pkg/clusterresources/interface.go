package clusterresources

import (
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
)

type GeneratorInterface interface {
	IDMS_ITMSGenerator(allRelatedImages []v1alpha3.CopyImageSchema, forceRepositoryScope bool) error
	UpdateServiceGenerator(graphImage, releaseImage string) error
	CatalogSourceGenerator(catalogImage string) error
}
