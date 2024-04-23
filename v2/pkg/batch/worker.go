package batch

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/openshift/oc-mirror/v2/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

type BatchInterface interface {
	Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) error
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
	CopyImages []v2alpha1.RelatedImage
	Items      int
	Count      int
	BatchSize  int
	BatchIndex int
	Remainder  int
}

// Worker - the main batch processor
func (o *Batch) Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) error {
	startTime := time.Now()

	var mirrorMsg string
	if opts.Function == string(mirror.CopyMode) {
		mirrorMsg = "copying"
	} else if opts.Function == string(mirror.DeleteMode) {
		mirrorMsg = "deleting"
	}

	var errArray []error

	totalImages := len(collectorSchema.AllImages)
	countTotal := 0
	countReleaseImages := 0
	countOperatorsImages := 0
	countAdditionalImages := 0

	o.Log.Info("🚀 Start " + mirrorMsg + " the images...")

	for _, img := range collectorSchema.AllImages {
		switch img.Type {
		case v2alpha1.TypeOCPRelease, v2alpha1.TypeOCPReleaseContent, v2alpha1.TypeCincinnatiGraph:
			countReleaseImages++
		case v2alpha1.TypeOperatorCatalog, v2alpha1.TypeOperatorBundle, v2alpha1.TypeOperatorRelatedImage:
			countOperatorsImages++
		case v2alpha1.TypeGeneric:
			countAdditionalImages++
		}

		countTotal++

		overalProgress := fmt.Sprintf("=== Overall Progress - "+mirrorMsg+" image %d / %d ===", countTotal, totalImages)
		o.Log.Info(overalProgress)
		if opts.Function == string(mirror.CopyMode) {
			o.Log.Info(mirrorMsg+" release image %d / %d ", countReleaseImages, collectorSchema.TotalReleaseImages)
			o.Log.Info(mirrorMsg+" operator image %d / %d ", countOperatorsImages, collectorSchema.TotalOperatorImages)
			o.Log.Info(mirrorMsg+" additional image %d / %d ", countAdditionalImages, collectorSchema.TotalAdditionalImages)
			o.Log.Info(strings.Repeat("=", len(overalProgress)))
		}
		o.Log.Info(mirrorMsg+" image: %s", img.Origin)

		err := o.Mirror.Run(ctx, img.Source, img.Destination, mirror.Mode(opts.Function), &opts)

		if err != nil {
			errArray = append(errArray, err)
		}
	}

	if opts.Function == string(mirror.CopyMode) {
		o.Log.Info("=== Results ===")
		if countReleaseImages == collectorSchema.TotalReleaseImages {
			o.Log.Info("All release images mirrored successfully %d / %d ✅", countReleaseImages, collectorSchema.TotalReleaseImages)
		} else {
			o.Log.Info("Some release images failed to mirror %d / %d ❌ - please check the logs", countReleaseImages, collectorSchema.TotalReleaseImages)
		}

		if countOperatorsImages == collectorSchema.TotalOperatorImages {
			o.Log.Info("All operator images mirrored successfully %d / %d ✅", countOperatorsImages, collectorSchema.TotalOperatorImages)
		} else {
			o.Log.Info("Some operator images failed to mirror %d / %d ❌ - please check the logs", countOperatorsImages, collectorSchema.TotalOperatorImages)
		}

		if countAdditionalImages == collectorSchema.TotalAdditionalImages {
			o.Log.Info("All additional images mirrored successfully %d / %d ✅", countAdditionalImages, collectorSchema.TotalAdditionalImages)
		} else {
			o.Log.Info("Some additional images failed to mirror %d / %d ❌ - please check the logs", countAdditionalImages, collectorSchema.TotalAdditionalImages)
		}
	} else {
		o.Log.Info("=== Results ===")
		if countTotal == totalImages {
			o.Log.Info("All images deleted successfully %d / %d ✅", countTotal, totalImages)
		} else {
			o.Log.Info("Some images failed to delete %d / %d ❌ - please check the logs", countTotal, totalImages)
		}
	}

	if len(errArray) > 0 {
		for _, err := range errArray {
			o.Log.Error(workerPrefix+"err: %s", err.Error())
		}
		return fmt.Errorf(workerPrefix + "error in batch - refer to console logs")
	}

	endTime := time.Now()
	execTime := endTime.Sub(startTime)
	o.Log.Debug("batch time     : %v", execTime)
	return nil
}
