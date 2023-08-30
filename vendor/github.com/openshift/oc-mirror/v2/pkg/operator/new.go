package operator

import (
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

func New(log clog.PluggableLoggerInterface,
	config v1alpha2.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
	localStorageFQDN string,
) CollectorInterface {
	if localStorageFQDN != "" {
		return &LocalStorageCollector{Log: log, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest, LocalStorageFQDN: localStorageFQDN}
	} else {
		return &Collector{Log: log, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest}
	}
}
