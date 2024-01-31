package release

import (
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/imagebuilder"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

func New(log clog.PluggableLoggerInterface,
	logsDir string,
	config v1alpha2.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
	cincinnati CincinnatiInterface,
	localStorageFQDN string,
	imageBuilder imagebuilder.ImageBuilderInterface,
) CollectorInterface {
	return &LocalStorageCollector{Log: log, LogsDir: logsDir, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest, Cincinnati: cincinnati, LocalStorageFQDN: localStorageFQDN, ImageBuilder: imageBuilder}
}
