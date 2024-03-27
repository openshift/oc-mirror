package delete

import (
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
)

type DeleteInterface interface {
	WriteDeleteMetaData([]v1alpha3.CopyImageSchema) error
	ReadDeleteMetaData() (v1alpha3.DeleteImageList, error)
	DeleteCacheBlobs(images v1alpha3.DeleteImageList) error
	DeleteRegistryImages(images v1alpha3.DeleteImageList) error
	CollectReleaseImages(releaseFolder string) ([]v1alpha3.CopyImageSchema, error)
	CollectOperatorImages() ([]v1alpha3.CopyImageSchema, error)
	CollectAdditionalImages() ([]v1alpha3.CopyImageSchema, error)
}
