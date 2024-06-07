package batch

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

func NewConcurrentBatch(log clog.PluggableLoggerInterface,
	logsDir string,
	mirror mirror.MirrorInterface,
	batchSize uint,
) BatchInterface {
	copiedImages := v2alpha1.CollectorSchema{
		AllImages: []v2alpha1.CopyImageSchema{},
	}
	return &ConcurrentBatch{Log: log, LogsDir: logsDir, Mirror: mirror, CopiedImages: copiedImages, BatchSize: batchSize}
}

type ConcurrentBatch struct {
	Log          clog.PluggableLoggerInterface
	LogsDir      string
	Mirror       mirror.MirrorInterface
	Manifest     manifest.ManifestInterface
	CopiedImages v2alpha1.CollectorSchema
	Progress     *ProgressStruct
	BatchSize    uint
}

type BatchSchema struct {
	Images v2alpha1.CollectorSchema
}

// Worker - the main batch processor
func (o *ConcurrentBatch) Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.CollectorSchema, error) {

	var errArray []mirrorErrorSchema
	var mu sync.Mutex
	var wg errgroup.Group

	var mirrorMsg string
	if opts.Function == string(mirror.CopyMode) {
		mirrorMsg = "copying"
	} else if opts.Function == string(mirror.DeleteMode) {
		mirrorMsg = "deleting"
	}

	startTime := time.Now()

	batches := splitImagesToBatches(collectorSchema, int(o.BatchSize))

	o.Log.Info("🚀 Start " + mirrorMsg + " the images...")

	o.Log.Info("images to %s %d ", opts.Function, len(collectorSchema.AllImages))
	o.Log.Debug("batch count %d ", len(batches))

	imgOverallIndex := 0
	for batchIndex, batch := range batches {

		p := mpb.New()
		total := len(collectorSchema.AllImages)
		for _, img := range batch.Images.AllImages {
			imgOverallIndex++
			counterText := fmt.Sprintf("%d/%d : (", imgOverallIndex, total)
			spinner := p.AddSpinner(
				1, mpb.BarFillerMiddleware(positionSpinnerLeft),
				mpb.BarWidth(3),
				mpb.PrependDecorators(
					decor.OnComplete(emptyDecorator(), "\x1b[1;92m ✓ \x1b[0m"),
					decor.OnAbort(emptyDecorator(), "\x1b[1;91m ✗ \x1b[0m"),
				),
				mpb.AppendDecorators(
					decor.Name(counterText),
					decor.Elapsed(decor.ET_STYLE_GO),
					decor.Name(") "+img.Origin+" "),
				),
				mpb.BarFillerClearOnComplete(),
				barFillerClearOnAbort(),
			)
			wg.Go(func() error {
				skip, reason := shouldSkipImage(img, opts.Mode)
				if skip {
					mu.Lock()
					o.CopiedImages.TotalReleaseImages++
					o.CopiedImages.AllImages = append(o.CopiedImages.AllImages, img)
					if reason != nil {
						errArray = append(errArray, mirrorErrorSchema{image: img, err: reason})
					}
					spinner.Increment()
					mu.Unlock()
					return nil
				}

				err := o.Mirror.Run(ctx, img.Source, img.Destination, mirror.Mode(opts.Function), &opts)
				mu.Lock()
				switch {
				case err == nil:
					o.CopiedImages.AllImages = append(o.CopiedImages.AllImages, img)
					spinner.Increment()

					switch img.Type {
					case v2alpha1.TypeCincinnatiGraph, v2alpha1.TypeOCPRelease, v2alpha1.TypeOCPReleaseContent:
						o.CopiedImages.TotalReleaseImages++
					case v2alpha1.TypeGeneric:
						o.CopiedImages.TotalAdditionalImages++
					case v2alpha1.TypeOperatorBundle, v2alpha1.TypeOperatorCatalog, v2alpha1.TypeOperatorRelatedImage:
						o.CopiedImages.TotalOperatorImages++
					}
				case img.Type != v2alpha1.TypeOCPRelease && img.Type != v2alpha1.TypeOCPReleaseContent:
					_, errArray = handleError(err, errArray, img, collectorSchema.AllImages)
					spinner.Abort(false)
				default:
					// error on release image, save the errArray and immediately return `UnsafeError` to caller
					currentMirrorError := mirrorErrorSchema{image: img, err: err}
					errArray = append(errArray, currentMirrorError)
					spinner.Abort(false)
					mu.Unlock()
					return NewUnsafeError(currentMirrorError)
				}
				mu.Unlock()
				return nil
			})

		}
		if err := wg.Wait(); err != nil {
			return o.CopiedImages, err
		}
		p.Wait()
		if batchIndex < len(batches)-1 { // for all batches except last one
			if len(errArray) > 0 {
				o.Log.Warn("Mirroring is ongoing. Total errors: %d.", len(errArray))
			} else {
				o.Log.Info("Mirroring is ongoing. No errors.")
			}
		}
	}

	if opts.Function == string(mirror.CopyMode) {
		o.Log.Info("=== Results ===")
		if collectorSchema.TotalReleaseImages != 0 {
			if o.CopiedImages.TotalReleaseImages == collectorSchema.TotalReleaseImages {
				o.Log.Info("✅ %d / %d release images mirrored successfully", o.CopiedImages.TotalReleaseImages, collectorSchema.TotalReleaseImages)
			} else {
				o.Log.Info("❌ %d / %d release images mirrored: Some release images failed to mirror - please check the logs", o.CopiedImages.TotalReleaseImages, collectorSchema.TotalReleaseImages)
			}
		}
		if collectorSchema.TotalOperatorImages != 0 {
			if o.CopiedImages.TotalOperatorImages == collectorSchema.TotalOperatorImages {
				o.Log.Info("✅ %d / %d operator images mirrored successfully", o.CopiedImages.TotalOperatorImages, collectorSchema.TotalOperatorImages)
			} else {
				o.Log.Info("❌ %d / %d operator images mirrored: Some operator images failed to mirror - please check the logs", o.CopiedImages.TotalOperatorImages, collectorSchema.TotalOperatorImages)
			}
		}
		if collectorSchema.TotalAdditionalImages != 0 {
			if o.CopiedImages.TotalAdditionalImages == collectorSchema.TotalAdditionalImages {
				o.Log.Info("✅ %d / %dadditional images mirrored successfully", o.CopiedImages.TotalAdditionalImages, collectorSchema.TotalAdditionalImages)
			} else {
				o.Log.Info("❌ %d / %d addtional images mirrored: Some additional images failed to mirror - please check the logs", o.CopiedImages.TotalAdditionalImages, collectorSchema.TotalAdditionalImages)
			}
		}
	} else {
		o.Log.Info("=== Results ===")
		totalImages := len(collectorSchema.AllImages)
		totalImagesMirrored := o.CopiedImages.TotalAdditionalImages + o.CopiedImages.TotalOperatorImages + o.CopiedImages.TotalReleaseImages
		if totalImagesMirrored == totalImages && totalImages != 0 {
			o.Log.Info("✅ %d / %d images deleted successfully", totalImagesMirrored, totalImages)
		} else {
			o.Log.Info("❌ %d / %d images deleted: Some images failed to delete - please check the logs", totalImagesMirrored, totalImages)
		}
	}
	if len(errArray) > 0 {
		filename, err := saveErrors(o.Log, o.LogsDir, errArray)
		if err != nil {
			return o.CopiedImages, NewSafeError(workerPrefix+"some errors occurred during the mirroring - unable to log these errors in %s: %v", o.LogsDir+"/"+filename, err)
		} else {
			msg := workerPrefix + "some errors occurred during the mirroring.\n" +
				"\t Please review " + o.LogsDir + "/" + filename + " for a list of mirroring errors.\n" +
				"\t You may consider:\n" +
				"\t * removing images or operators that cause the error from the image set config, and retrying\n" +
				"\t * keeping the image set config (images are mandatory for you), and retrying\n" +
				"\t * mirroring the failing images manually, if retries also fail."
			return o.CopiedImages, NewSafeError(msg)
		}
	}
	endTime := time.Now()
	execTime := endTime.Sub(startTime)
	o.Log.Debug("batch time     : %v", execTime)
	return collectorSchema, nil
}

// later, we can consider making this func smarter:
// by putting related images, release content images first
// and deferring operator bundle images, second
// and lastly release images and catalog images
// CLID-133 + CLID-98
func splitImagesToBatches(images v2alpha1.CollectorSchema, maxBatchSize int) []BatchSchema {
	imgsCount := len(images.AllImages)
	if imgsCount == 0 {
		return []BatchSchema{}
	}
	if imgsCount <= maxBatchSize {
		return []BatchSchema{
			{
				Images: images,
			},
		}
	} else {
		batches := []BatchSchema{}
		for index := 0; index < imgsCount; index += maxBatchSize {
			batch := BatchSchema{}
			batchSize := maxBatchSize
			if imgsCount-index > maxBatchSize {
				batchSize = maxBatchSize
			} else {
				batchSize = imgsCount - index
			}
			batch.Images = v2alpha1.CollectorSchema{
				AllImages: images.AllImages[index : index+batchSize],
			}
			batches = append(batches, batch)
		}
		return batches
	}
}

func positionSpinnerLeft(original mpb.BarFiller) mpb.BarFiller {
	return mpb.SpinnerStyle("⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏", " ").PositionLeft().Build()
}

func emptyDecorator() decor.Decorator {
	return decor.Any(func(s decor.Statistics) string {
		return ""
	})
}

func barFillerClearOnAbort() mpb.BarOption {
	return mpb.BarFillerMiddleware(func(base mpb.BarFiller) mpb.BarFiller {
		return mpb.BarFillerFunc(func(w io.Writer, st decor.Statistics) error {
			if st.Aborted {
				_, err := io.WriteString(w, "")
				return err
			}
			return base.Fill(w, st)
		})
	})
}

// shouldSkipImage helps determine whether the batch should perform the mirroring of the image
// or if the image should be skipped.
// At the moment, only the graph image will be skipped when the mode is MirrorToDisk or MirrorToMirror.
// In later versions, this function can evolve to also skip images that were marked shouldSkip.
// An example would be to skip mirroring an operator bundle image when one of its related images have failed
// to mirror.
// in the latter case, shouldSkipImage will also return an error which will explain the reason for skipping
func shouldSkipImage(img v2alpha1.CopyImageSchema, mode string) (bool, error) {
	if img.Type == v2alpha1.TypeCincinnatiGraph && (mode == mirror.MirrorToDisk || mode == mirror.MirrorToMirror) {
		return true, nil
	}
	return false, nil
}

// handleError makes the necessary changes to errArray and to the collectorSchema.AllImages
// when an error occurs.
// At the moment, only errArray is appended with the new error.
// In a later change (CLID-133 + CLID-98), we might also choose to update
// the "parent" image (such as the bundle image or release image) to signal the operator
// did not get mirrored correctly (easier troubleshooting), and to skip the "parent" or "sibling"
// images to gain more time.
func handleError(err error, errArray []mirrorErrorSchema, img v2alpha1.CopyImageSchema, allImages []v2alpha1.CopyImageSchema) ([]v2alpha1.CopyImageSchema, []mirrorErrorSchema) {
	errArray = append(errArray, mirrorErrorSchema{image: img, err: err})
	return allImages, errArray
}
