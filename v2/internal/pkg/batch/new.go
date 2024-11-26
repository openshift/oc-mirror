package batch

import (
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func New(workerType string,
	log clog.PluggableLoggerInterface,
	logsDir string,
	mirror mirror.MirrorInterface,
	batchSize uint,
) BatchInterface {
	copiedImages := v2alpha1.CollectorSchema{
		AllImages: []v2alpha1.CopyImageSchema{},
	}

	switch workerType {
	case ConcurrentWorker:
		return &ConcurrentBatch{Log: log, LogsDir: logsDir, Mirror: mirror, CopiedImages: copiedImages, BatchSize: batchSize}
	case ChannelConcurrentWorker:
		return &ChannelConcurrentBatch{Log: log, LogsDir: logsDir, Mirror: mirror, MaxGoroutines: int(batchSize)}
	default:
		return &ChannelConcurrentBatch{Log: log, LogsDir: logsDir, Mirror: mirror, MaxGoroutines: int(batchSize)}
	}
}
