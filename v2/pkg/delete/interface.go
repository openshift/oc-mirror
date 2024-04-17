package delete

import (
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
)

type DeleteInterface interface {
	WriteDeleteMetaData([]v1alpha3.CopyImageSchema) error
	ReadDeleteMetaData() (v1alpha3.DeleteImageList, error)
	DeleteRegistryImages(images v1alpha3.DeleteImageList) error
	ConvertReleaseImages([]v1alpha3.RelatedImage) ([]v1alpha3.CopyImageSchema, error)
	// Get all relevant releases for delete filter by arch,min and max version
	FilterReleasesForDelete() (map[string][]v1alpha3.RelatedImage, error)
}
