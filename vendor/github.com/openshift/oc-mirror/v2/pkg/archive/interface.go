package archive

import (
	"context"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
)

type BlobsGatherer interface {
	GatherBlobs(ctx context.Context, imgRef string) (map[string]string, error)
}

type Archiver interface {
	BuildArchive(ctx context.Context, collectedImages []v1alpha3.CopyImageSchema) (string, error)
	Close() error
}

type UnArchiver interface {
	Close() error
	Unarchive() error
}
