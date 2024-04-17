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

func (o MockDelete) ConvertReleaseImages([]v1alpha3.RelatedImage) ([]v1alpha3.CopyImageSchema, error) {
	return nil, nil
}

func (o MockDelete) FilterReleasesForDelete() (map[string][]v1alpha3.RelatedImage, error) {
	res := make(map[string][]v1alpha3.RelatedImage)
	res["test"] = []v1alpha3.RelatedImage{
		{
			Name:  "test",
			Image: "test.registry.com/test",
		},
	}
	return res, nil
}

// nolint: unused
func (o MockDelete) startLocalRegistryGarbageCollect() error {
	return nil
}
