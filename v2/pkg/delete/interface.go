package delete

import (
	"context"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
)

type DeleteInterface interface {
	ReadDeleteMetaData() ([]v1alpha3.CopyImageSchema, error)
	DeleteCacheBlobs(ctx context.Context, images []v1alpha3.CopyImageSchema) error
	DeleteRegistryImages(ctx context.Context, images []v1alpha3.CopyImageSchema) error
	CollectReleaseImages(releaseFolder string) ([]v1alpha3.CopyImageSchema, error)
	CollectOperatorImages() ([]v1alpha3.CopyImageSchema, error)
	CollectAdditionalImages() ([]v1alpha3.CopyImageSchema, error)
}
