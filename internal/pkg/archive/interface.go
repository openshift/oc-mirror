package archive

import (
	"context"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type BlobsGatherer interface {
	// GatherBlobs returns all blobs for the given image.
	// allowedPlatforms is the set of os/arch pairs that were intentionally mirrored
	// (from CollectorSchema.PlatformFilters). Missing platforms outside this set are
	// skipped; missing platforms inside it are real errors.
	GatherBlobs(ctx context.Context, imgRef string, allowedPlatforms []string) (sets.Set[string], error)
}

type Archiver interface {
	BuildArchive(ctx context.Context, schema v2alpha1.CollectorSchema) error
}

type UnArchiver interface {
	Unarchive() error
}

type archiveAdder interface {
	addFile(pathToFile string, pathInTar string) error
	addAllFolder(folderToAdd string, relativeTo string) error
	close() error
}
