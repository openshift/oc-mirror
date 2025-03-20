package batch

import (
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func New(workerType string,
	log clog.PluggableLoggerInterface,
	logsDir string,
	mirror mirror.MirrorInterface,
	batchSize uint,
) BatchInterface {
	return &ChannelConcurrentBatch{Log: log, LogsDir: logsDir, Mirror: mirror, MaxGoroutines: int(batchSize)}
}
