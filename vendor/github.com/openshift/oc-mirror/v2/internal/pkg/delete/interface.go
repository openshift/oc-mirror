package delete

import (
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type DeleteInterface interface {
	WriteDeleteMetaData([]v2alpha1.CopyImageSchema) error
	ReadDeleteMetaData() (v2alpha1.DeleteImageList, error)
	DeleteRegistryImages(images v2alpha1.DeleteImageList) error
}
