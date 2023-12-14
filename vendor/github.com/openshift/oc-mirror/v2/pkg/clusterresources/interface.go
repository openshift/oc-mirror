package clusterresources

import (
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
)

type GeneratorInterface interface {
	IDMSGenerator(allRelatedImages []v1alpha3.CopyImageSchema) error
	UpdateServiceGenerator(graphImage, releaseImage string) error
}
