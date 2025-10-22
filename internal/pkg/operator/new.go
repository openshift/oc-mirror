package operator

import (
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func NewWithFilter(log clog.PluggableLoggerInterface,
	logsDir string,
	config v2alpha1.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
) CollectorInterface {
	return &FilterCollector{OperatorCollector{Log: log, LogsDir: logsDir, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest, LocalStorageFQDN: opts.LocalStorageFQDN, ctlgHandler: catalogHandler{Log: log}}}
}
