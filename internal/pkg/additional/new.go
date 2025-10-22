package additional

import (
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func New(log clog.PluggableLoggerInterface,
	config v2alpha1.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
) CollectorInterface {
	return &LocalStorageCollector{Log: log, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest, LocalStorageFQDN: opts.LocalStorageFQDN}
}
