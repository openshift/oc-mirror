package cli

import (
	_ "github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type MockDelete struct{}

func (o MockDelete) ReadDeleteMetaData() (v2alpha1.DeleteImageList, error) {
	return v2alpha1.DeleteImageList{}, nil
}

func (o MockDelete) WriteDeleteMetaData([]v2alpha1.CopyImageSchema) error {
	return nil
}

func (o MockDelete) DeleteCacheBlobs(images v2alpha1.DeleteImageList) error {
	return nil
}

func (o MockDelete) DeleteRegistryImages(images v2alpha1.DeleteImageList) error {
	return nil
}

func (o MockDelete) ConvertReleaseImages([]v2alpha1.RelatedImage) ([]v2alpha1.CopyImageSchema, error) {
	return nil, nil
}

func (o MockDelete) FilterReleasesForDelete() (map[string][]v2alpha1.RelatedImage, error) {
	res := make(map[string][]v2alpha1.RelatedImage)
	res["test"] = []v2alpha1.RelatedImage{
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
