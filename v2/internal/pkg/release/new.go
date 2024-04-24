package release

import (
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/imagebuilder"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func New(log clog.PluggableLoggerInterface,
	logsDir string,
	config v2alpha1.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
	cincinnati CincinnatiInterface,
	localStorageFQDN string,
	imageBuilder imagebuilder.ImageBuilderInterface,
) CollectorInterface {
	return &LocalStorageCollector{Log: log, LogsDir: logsDir, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest, Cincinnati: cincinnati, LocalStorageFQDN: localStorageFQDN, ImageBuilder: imageBuilder}
}
