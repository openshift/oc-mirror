package cli

import (
	"context"

	_ "github.com/distribution/distribution/v3/registry/storage/driver/filesystem"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type MockDelete struct{}

func (o MockDelete) ReadDeleteMetaData() (v2alpha1.DeleteImageList, error) {
	return v2alpha1.DeleteImageList{}, nil
}

func (o MockDelete) WriteDeleteMetaData(context.Context, []v2alpha1.CopyImageSchema) error {
	return nil
}

func (o MockDelete) DeleteCacheBlobs(images v2alpha1.DeleteImageList) error {
	return nil
}

func (o MockDelete) DeleteRegistryImages(images v2alpha1.DeleteImageList) error {
	return nil
}

// nolint: unused
func (o MockDelete) startLocalRegistryGarbageCollect() error {
	return nil
}
