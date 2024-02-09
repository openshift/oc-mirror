package manifest

import (
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
)

type ManifestInterface interface {
	GetImageIndex(dir string) (*v1alpha3.OCISchema, error)
	GetImageManifest(file string) (*v1alpha3.OCISchema, error)
	GetOperatorConfig(file string) (*v1alpha3.OperatorConfigSchema, error)
	GetCatalog(filePath string) (OperatorCatalog, error)
	GetRelatedImagesFromCatalog(operatorCatalog OperatorCatalog, ctlgInIsc v1alpha2.Operator) (map[string][]v1alpha3.RelatedImage, error)
	ExtractLayersOCI(filePath, toPath, label string, oci *v1alpha3.OCISchema) error
	GetReleaseSchema(filePath string) ([]v1alpha3.RelatedImage, error)
}
