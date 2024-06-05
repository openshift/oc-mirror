package batch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type BatchInterface interface {
	Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.CollectorSchema, error)
}

func New(log clog.PluggableLoggerInterface,
	logsDir string,
	mirror mirror.MirrorInterface,
) BatchInterface {
	copiedImages := v2alpha1.CollectorSchema{
		AllImages: []v2alpha1.CopyImageSchema{},
	}
	return &Batch{Log: log, LogsDir: logsDir, Mirror: mirror, CopiedImages: copiedImages, Progress: &ProgressStruct{}}
}

type Batch struct {
	Log          clog.PluggableLoggerInterface
	LogsDir      string
	Mirror       mirror.MirrorInterface
	CopiedImages v2alpha1.CollectorSchema
	Progress     *ProgressStruct
}

type ProgressStruct struct {
	countTotal            int
	countReleaseImages    int
	countOperatorsImages  int
	countAdditionalImages int

	countErrorTotal                 int
	countReleaseImagesErrorTotal    int
	countOperatorsImagesErrorTotal  int
	countAdditionalImagesErrorTotal int

	mirrorMessage string
	Log           clog.PluggableLoggerInterface
}
type mirrorErrorSchema struct {
	image v2alpha1.CopyImageSchema
	err   error
}

// Worker - the main batch processor
func (o *Batch) Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.CollectorSchema, error) {
	startTime := time.Now()

	var mirrorMsg string
	if opts.Function == string(mirror.CopyMode) {
		mirrorMsg = "copying"
	} else if opts.Function == string(mirror.DeleteMode) {
		mirrorMsg = "deleting"
	}

	var errArray []mirrorErrorSchema

	totalImages := len(collectorSchema.AllImages)

	o.Log.Info("🚀 Start " + mirrorMsg + " the images...")

	for _, img := range collectorSchema.AllImages {

		if img.Type == v2alpha1.TypeCincinnatiGraph && (opts.Mode == mirror.MirrorToDisk || opts.Mode == mirror.MirrorToMirror) {
			continue
		}

		err := o.Mirror.Run(ctx, img.Source, img.Destination, mirror.Mode(opts.Function), &opts)
		isSafe := isFailSafe(err)
		switch {
		case err == nil:
			o.CopiedImages.AllImages = append(o.CopiedImages.AllImages, img)
			// switch img.Type {
			// case v2alpha1.TypeCincinnatiGraph, v2alpha1.TypeOCPRelease, v2alpha1.TypeOCPReleaseContent:
			// 	o.CopiedImages.TotalReleaseImages++
			// case v2alpha1.TypeGeneric:
			// 	o.CopiedImages.TotalAdditionalImages++
			// case v2alpha1.TypeOperatorBundle, v2alpha1.TypeOperatorCatalog, v2alpha1.TypeOperatorRelatedImage:
			// 	o.CopiedImages.TotalOperatorImages++
			// }
		case err != nil && isSafe && img.Type != v2alpha1.TypeOCPRelease && img.Type != v2alpha1.TypeOCPReleaseContent:
			// this error is fail safe, we're continuing to mirror other images.
			errArray = append(errArray, mirrorErrorSchema{image: img, err: err})

		case err != nil && isSafe && (img.Type == v2alpha1.TypeOCPRelease || img.Type == v2alpha1.TypeOCPReleaseContent):
			fallthrough
		case err != nil && !isSafe:
			// this error is fail fast, we save the errArray and immediately return `UnsafeError` to caller
			currentMirrorError := mirrorErrorSchema{image: img, err: err}
			errArray = append(errArray, currentMirrorError)
			filename, saveError := o.saveErrors(errArray)
			if saveError != nil {
				o.Log.Error("unable to log these errors in %s: %v", o.LogsDir+"/"+filename, saveError)
			}
			return o.CopiedImages, NewUnsafeError(currentMirrorError)
		}

		o.logProgress(img, collectorSchema, err)

	}

	if opts.Function == string(mirror.CopyMode) {
		o.Log.Info("=== Results ===")
		if collectorSchema.TotalReleaseImages != 0 {
			if o.Progress.countReleaseImages == collectorSchema.TotalReleaseImages && o.Progress.countReleaseImagesErrorTotal == 0 {
				o.Log.Info("All release images mirrored successfully %d / %d ✅", o.Progress.countReleaseImages, collectorSchema.TotalReleaseImages)
			} else {
				o.Log.Info("Images mirrored %d / %d: Some release images failed to mirror ❌ - please check the logs", o.Progress.countReleaseImages-o.Progress.countReleaseImagesErrorTotal, collectorSchema.TotalReleaseImages)
			}
		}
		if collectorSchema.TotalOperatorImages != 0 {
			if o.Progress.countOperatorsImages == collectorSchema.TotalOperatorImages && o.Progress.countOperatorsImagesErrorTotal == 0 {
				o.Log.Info("All operator images mirrored successfully %d / %d ✅", o.Progress.countOperatorsImages, collectorSchema.TotalOperatorImages)
			} else {
				o.Log.Info("Images mirrored %d / %d: Some operator images failed to mirror ❌ - please check the logs", o.Progress.countOperatorsImages-o.Progress.countOperatorsImagesErrorTotal, collectorSchema.TotalOperatorImages)
			}
		}
		if collectorSchema.TotalAdditionalImages != 0 {
			if o.Progress.countAdditionalImages == collectorSchema.TotalAdditionalImages && o.Progress.countAdditionalImagesErrorTotal == 0 {
				o.Log.Info("All additional images mirrored successfully %d / %d ✅", o.Progress.countAdditionalImages, collectorSchema.TotalAdditionalImages)
			} else {
				o.Log.Info("Images mirrored %d / %d: Some additional images failed to mirror ❌ - please check the logs", o.Progress.countAdditionalImages-o.Progress.countAdditionalImagesErrorTotal, collectorSchema.TotalAdditionalImages)
			}
		}
	} else {
		o.Log.Info("=== Results ===")
		if o.Progress.countTotal == totalImages && o.Progress.countErrorTotal == 0 {
			o.Log.Info("All images deleted successfully %d / %d ✅", o.Progress.countTotal, totalImages)
		} else {
			o.Log.Info("Images deleted %d / %d: Some images failed to delete ❌ - please check the logs", o.Progress.countTotal-o.Progress.countErrorTotal, totalImages)
		}
	}

	if len(errArray) > 0 {
		filename, err := o.saveErrors(errArray)
		if err != nil {
			return o.CopiedImages, NewSafeError(workerPrefix+"some errors occurred during the mirroring - unable to log these errors in %s: %v", o.LogsDir+"/"+filename, err)
		} else {
			return o.CopiedImages, NewSafeError(workerPrefix+"some errors occurred during the mirroring - refer to %s for more details", o.LogsDir+"/"+filename)
		}
	}

	endTime := time.Now()
	execTime := endTime.Sub(startTime)
	o.Log.Debug("batch time     : %v", execTime)
	return o.CopiedImages, nil
}

func (o *Batch) saveErrors(errArray []mirrorErrorSchema) (string, error) {
	if len(errArray) > 0 {
		timestamp := time.Now().Format("20060102_150405")
		filename := fmt.Sprintf("mirroring_errors_%s.txt", timestamp)
		file, err := os.Create(filepath.Join(o.LogsDir, filename))
		if err != nil {
			o.Log.Error(workerPrefix+"failed to create file: %s", err.Error())
			return filename, err
		}
		defer file.Close()

		for _, err := range errArray {
			errorMsg := fmt.Sprintf("error mirroring image %s error: %s", err.image.Origin, err.err.Error())
			o.Log.Error(workerPrefix + errorMsg)
			fmt.Fprintln(file, errorMsg)
		}
		return filename, nil
	}
	return "", nil
}

func (o Batch) logProgress(img v2alpha1.CopyImageSchema, collectorSchema v2alpha1.CollectorSchema, err error) {
	switch img.Type {
	case v2alpha1.TypeOCPRelease, v2alpha1.TypeOCPReleaseContent, v2alpha1.TypeCincinnatiGraph:
		o.Progress.countReleaseImages++
	case v2alpha1.TypeOperatorCatalog, v2alpha1.TypeOperatorBundle, v2alpha1.TypeOperatorRelatedImage:
		o.Progress.countOperatorsImages++
	case v2alpha1.TypeGeneric:
		o.Progress.countAdditionalImages++
	}

	o.Progress.countTotal++
	isSafe := isFailSafe(err)
	//isSafe && img.Type != v2alpha1.TypeOCPRelease && img.Type != v2alpha1.TypeOCPReleaseContent:
	if err != nil && !isSafe { // normally logProgress should never be called for fail fast errors
		return
	} else if err != nil { // it is a fail safe error
		if img.Type == v2alpha1.TypeOCPRelease || img.Type == v2alpha1.TypeOCPReleaseContent { // normally logProgress should never be called for fail fast errors
			return
		}
		o.Progress.countErrorTotal++
		switch img.Type {
		case v2alpha1.TypeOCPRelease, v2alpha1.TypeOCPReleaseContent, v2alpha1.TypeCincinnatiGraph:
			o.Progress.countReleaseImagesErrorTotal++
		case v2alpha1.TypeOperatorCatalog, v2alpha1.TypeOperatorBundle, v2alpha1.TypeOperatorRelatedImage:
			o.Progress.countOperatorsImagesErrorTotal++
		case v2alpha1.TypeGeneric:
			o.Progress.countAdditionalImagesErrorTotal++
		}
	}

	var overalProgress string
	if o.Progress.countErrorTotal > 0 {
		overalProgress = fmt.Sprintf("=== Overall Progress - "+o.Progress.mirrorMessage+" image %d / %d (%d errors)===", o.Progress.countTotal, len(collectorSchema.AllImages), o.Progress.countErrorTotal)
	} else {
		overalProgress = fmt.Sprintf("=== Overall Progress - "+o.Progress.mirrorMessage+" image %d / %d ===", o.Progress.countTotal, len(collectorSchema.AllImages))
	}
	o.Log.Info(overalProgress)

	if collectorSchema.TotalReleaseImages > 0 {
		if o.Progress.countReleaseImagesErrorTotal > 0 {
			o.Log.Info(o.Progress.mirrorMessage+" release image %d / %d (%d errors)", o.Progress.countReleaseImages, collectorSchema.TotalReleaseImages, o.Progress.countReleaseImagesErrorTotal)
		} else {
			o.Log.Info(o.Progress.mirrorMessage+" release image %d / %d", o.Progress.countReleaseImages, collectorSchema.TotalReleaseImages)
		}
	}
	if collectorSchema.TotalOperatorImages > 0 {
		if o.Progress.countOperatorsImagesErrorTotal > 0 {
			o.Log.Info(o.Progress.mirrorMessage+" operator image %d / %d (%d errors)", o.Progress.countOperatorsImages, collectorSchema.TotalOperatorImages, o.Progress.countOperatorsImagesErrorTotal)
		} else {
			o.Log.Info(o.Progress.mirrorMessage+" operator image %d / %d", o.Progress.countOperatorsImages, collectorSchema.TotalOperatorImages)
		}
	}
	if collectorSchema.TotalAdditionalImages > 0 {
		if o.Progress.countAdditionalImagesErrorTotal > 0 {
			o.Log.Info(o.Progress.mirrorMessage+" additional image %d / %d (%d errors)", o.Progress.countAdditionalImages, collectorSchema.TotalAdditionalImages, o.Progress.countAdditionalImagesErrorTotal)
		} else {
			o.Log.Info(o.Progress.mirrorMessage+" additional image %d / %d", o.Progress.countAdditionalImages, collectorSchema.TotalAdditionalImages)
		}
	}
	o.Log.Info(" image: %s", img.Origin)

	o.Log.Info(strings.Repeat("=", len(overalProgress)))

}
