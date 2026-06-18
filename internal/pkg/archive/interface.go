package archive

import (
	"context"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type BlobsGatherer interface {
	GatherBlobs(ctx context.Context, imgRef string) (sets.Set[string], error)
}

type Archiver interface {
	BuildArchive(ctx context.Context, collectedImages []v2alpha1.CopyImageSchema) error
}

type UnArchiver interface {
	Unarchive() error
}

type archiveAdder interface {
	addFile(pathToFile string, pathInTar string) error
	addAllFolder(folderToAdd string, relativeTo string) error
	close() error
}
