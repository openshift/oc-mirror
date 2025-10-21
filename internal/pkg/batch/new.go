package batch

import (
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// We want to return an interface here since `New` is a convenience function to
// easily swap out batch worker implementations in the callee.
//
//nolint:ireturn
func New(workerType string,
	log clog.PluggableLoggerInterface,
	logsDir string,
	mirror mirror.MirrorInterface,
	batchSize uint,
	timestamp string,
) BatchInterface {
	return &ChannelConcurrentBatch{Log: log, LogsDir: logsDir, Mirror: mirror, MaxGoroutines: batchSize, SynchedTimeStamp: timestamp}
}
