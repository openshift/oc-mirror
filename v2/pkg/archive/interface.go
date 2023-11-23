package archive

import "github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"

type BlobsGatherer interface {
	GatherBlobs(imgRef string) (map[string]string, error)
}

type Archiver interface {
	BuildArchive(collectedImages []v1alpha3.CopyImageSchema) (string, error)
	Close() error
}

type UnArchiver interface {
	Close() error
	Unarchive() error
}
