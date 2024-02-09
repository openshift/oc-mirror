package batch

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

const (
	BATCH_SIZE int    = 8
	logFile    string = "worker-{batch}.log"
)

type BatchInterface interface {
	Worker(ctx context.Context, images []v1alpha3.CopyImageSchema, opts mirror.CopyOptions) error
}

func New(log clog.PluggableLoggerInterface,
	logsDir string,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
) BatchInterface {
	return &Batch{Log: log, LogsDir: logsDir, Mirror: mirror, Manifest: manifest}
}

type Batch struct {
	Log      clog.PluggableLoggerInterface
	LogsDir  string
	Mirror   mirror.MirrorInterface
	Manifest manifest.ManifestInterface
}

type BatchSchema struct {
	Writer     io.Writer
	CopyImages []v1alpha3.RelatedImage
	Items      int
	Count      int
	BatchSize  int
	BatchIndex int
	Remainder  int
}

// Worker - the main batch processor
func (o *Batch) Worker(ctx context.Context, images []v1alpha3.CopyImageSchema, opts mirror.CopyOptions) error {

	var errArray []error
	var mu sync.Mutex
	var wg sync.WaitGroup

	var b *BatchSchema
	imgs := len(images)
	if imgs < BATCH_SIZE {
		b = &BatchSchema{Items: imgs, Count: 1, BatchSize: imgs, BatchIndex: 0, Remainder: 0}
	} else {
		b = &BatchSchema{Items: imgs, Count: (imgs / BATCH_SIZE), BatchSize: BATCH_SIZE, Remainder: (imgs % BATCH_SIZE)}
	}

	o.Log.Info("images to mirror %d ", b.Items)
	o.Log.Info("batch count %d ", b.Count)
	o.Log.Info("batch index %d ", b.BatchIndex)
	o.Log.Info("batch size %d ", b.BatchSize)
	o.Log.Info("remainder size %d ", b.Remainder)

	// prepare batching
	wg.Add(b.BatchSize)
	for i := 0; i < b.Count; i++ {

		o.Log.Info(fmt.Sprintf("starting batch %d ", i))
		for x := 0; x < b.BatchSize; x++ {
			index := (i * b.BatchSize) + x
			o.Log.Debug("source %s ", images[index].Source)
			o.Log.Debug("destination %s ", images[index].Destination)
			go func(ctx context.Context, src, dest string, opts *mirror.CopyOptions) {
				defer wg.Done()
				err := o.Mirror.Run(ctx, src, dest, "copy", opts)
				if err != nil {
					mu.Lock()
					errArray = append(errArray, err)
					defer mu.Unlock()
				}
			}(ctx, images[index].Source, images[index].Destination, &opts)
		}
		wg.Wait()

		o.Log.Info("completed batch %d", i)
		if b.Count > 1 {
			wg.Add(BATCH_SIZE)
		}
		if len(errArray) > 0 {
			for _, err := range errArray {
				o.Log.Error("[Worker] errArray %v", err)
			}
			return fmt.Errorf("[Worker] error in batch - refer to console logs")
		}
	}
	if b.Remainder > 0 {
		// one level of simple recursion
		i := b.Count * BATCH_SIZE
		o.Log.Info("executing remainder [batch size of 1]")
		err := o.Worker(ctx, images[i:], opts)
		if err != nil {
			return err
		}
		// output the logs to console
		if !opts.Global.Quiet {
			consoleLogFromFile(o.Log, o.LogsDir)
		}
		o.Log.Info("[Worker] successfully completed all batches")
	}
	return nil
}

// consoleLogFromFile
func consoleLogFromFile(log clog.PluggableLoggerInterface, path string) {
	dir, _ := os.ReadDir(path)
	for _, f := range dir {
		if strings.Contains(f.Name(), "worker") {
			log.Debug("[batch] %s ", f.Name())
			data, _ := os.ReadFile("logs/" + f.Name())
			lines := strings.Split(string(data), "\n")
			for _, s := range lines {
				if len(s) > 0 {
					// clean the line
					log.Debug("%s ", strings.ToLower(s))
				}
			}
		}
	}
}
