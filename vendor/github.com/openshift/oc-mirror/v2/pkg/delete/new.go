package delete

import (
	"github.com/openshift/oc-mirror/v2/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/pkg/archive"
	"github.com/openshift/oc-mirror/v2/pkg/batch"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

func New(log clog.PluggableLoggerInterface,
	opts mirror.CopyOptions,
	batch batch.BatchInterface,
	blobs archive.BlobsGatherer,
	config v2alpha1.ImageSetConfiguration,
	manifest manifest.ManifestInterface,
	localStorageDisk string,
	localStorageFQDN string,
) DeleteInterface {
	return &DeleteImages{
		Log:              log,
		Opts:             opts,
		Batch:            batch,
		Blobs:            blobs,
		Config:           config,
		Manifest:         manifest,
		LocalStorageDisk: localStorageDisk,
		LocalStorageFQDN: localStorageFQDN,
	}
}
