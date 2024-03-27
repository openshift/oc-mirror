package cli

import (
	_ "github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
)

type MockDelete struct{}

func (o MockDelete) ReadDeleteMetaData() (v1alpha3.DeleteImageList, error) {
	return v1alpha3.DeleteImageList{}, nil
}

func (o MockDelete) WriteDeleteMetaData([]v1alpha3.CopyImageSchema) error {
	return nil
}

func (o MockDelete) DeleteCacheBlobs(images v1alpha3.DeleteImageList) error {
	return nil
}

func (o MockDelete) DeleteRegistryImages(images v1alpha3.DeleteImageList) error {
	return nil
}

func (o MockDelete) CollectReleaseImages(releaseFolder string) ([]v1alpha3.CopyImageSchema, error) {
	return []v1alpha3.CopyImageSchema{}, nil
}

func (o MockDelete) CollectOperatorImages() ([]v1alpha3.CopyImageSchema, error) {
	return []v1alpha3.CopyImageSchema{}, nil
}

func (o MockDelete) CollectAdditionalImages() ([]v1alpha3.CopyImageSchema, error) {
	return []v1alpha3.CopyImageSchema{}, nil
}
