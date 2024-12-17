package batch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/exp/maps"
	"golang.org/x/sync/errgroup"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/spinners"
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

var skippingMsg = "skipping operator bundle %s because one of its related images failed to mirror"

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

	opts.PreserveDigests = true

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
			imageText := ") " + img.Origin + " "
			if strings.Contains(img.Destination, opts.LocalStorageFQDN) {
				imageText += "➡️  cache "
			}
			spinner := p.AddSpinner(
				1, mpb.BarFillerMiddleware(spinners.PositionSpinnerLeft),
				mpb.BarWidth(3),
				mpb.PrependDecorators(
					decor.OnComplete(spinners.EmptyDecorator(), "\x1b[1;92m ✓ \x1b[0m"),
					decor.OnAbort(spinners.EmptyDecorator(), "\x1b[1;91m ✗ \x1b[0m"),
				),
				mpb.AppendDecorators(
					decor.Name(counterText),
					decor.Elapsed(decor.ET_STYLE_GO),
					decor.Name(imageText),
				),
				mpb.BarFillerClearOnComplete(),
				spinners.BarFillerClearOnAbort(),
			)
			wg.Go(func() error {
				mu.Lock()
				skip, reason := shouldSkipImage(img, opts.Mode, errArray)
				mu.Unlock()
				if skip {
					mu.Lock()

					if reason != nil {
						errArray = append(errArray, mirrorErrorSchema{image: img, err: reason})
					}

					switch img.Type {
					case v2alpha1.TypeOperatorBundle:
						spinner.Abort(false)
					case v2alpha1.TypeCincinnatiGraph:
						o.CopiedImages.TotalReleaseImages++
						o.CopiedImages.AllImages = append(o.CopiedImages.AllImages, img)
						spinner.Increment()
					}

					mu.Unlock()
					return nil
				}

				// OCPBUGS-43489
				// Ensure local cache images get deleted when --force-delete-cache flag is used
				// This reverts OCPBUGS-44448 (the root cause was a problem is in the DeleteDestination)
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
					case v2alpha1.TypeHelmImage:
						o.CopiedImages.TotalHelmImages++
					}
				case img.Type.IsOperator():
					operators := collectorSchema.CopyImageSchemaMap.OperatorsByImage[img.Origin]
					bundles := collectorSchema.CopyImageSchemaMap.BundlesByImage[img.Origin]
					errArray = append(errArray, mirrorErrorSchema{image: img, err: err, operators: operators, bundles: bundles})
					spinner.Abort(false)
				case img.Type.IsRelease():
					// error on release image, save the errArray and immediately return `UnsafeError` to caller
					currentMirrorError := mirrorErrorSchema{image: img, err: err}
					errArray = append(errArray, currentMirrorError)
					spinner.Abort(false)
					mu.Unlock()
					return NewUnsafeError(currentMirrorError)
				case img.Type.IsAdditionalImage() || img.Type.IsHelmImage():
					errArray = append(errArray, mirrorErrorSchema{image: img, err: err})
					spinner.Abort(false)
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
				o.Log.Warn("%s is ongoing. Total errors: %d.", mirrorMsg, len(errArray))
			} else {
				o.Log.Info("%s is ongoing. No errors.", mirrorMsg)
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
				o.Log.Info("✅ %d / %d additional images mirrored successfully", o.CopiedImages.TotalAdditionalImages, collectorSchema.TotalAdditionalImages)
			} else {
				o.Log.Info("❌ %d / %d addtional images mirrored: Some additional images failed to mirror - please check the logs", o.CopiedImages.TotalAdditionalImages, collectorSchema.TotalAdditionalImages)
			}
		}
		if collectorSchema.TotalHelmImages != 0 {
			if o.CopiedImages.TotalHelmImages == collectorSchema.TotalHelmImages {
				o.Log.Info("✅ %d / %d helm images mirrored successfully", o.CopiedImages.TotalHelmImages, collectorSchema.TotalHelmImages)
			} else {
				o.Log.Info("❌ %d / %d helm images mirrored: Some helm images failed to mirror - please check the logs", o.CopiedImages.TotalHelmImages, collectorSchema.TotalHelmImages)
			}
		}
	} else {
		o.Log.Info("=== Results ===")
		totalImages := len(collectorSchema.AllImages)
		totalImagesMirrored := o.CopiedImages.TotalAdditionalImages + o.CopiedImages.TotalOperatorImages + o.CopiedImages.TotalReleaseImages + o.CopiedImages.TotalHelmImages
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

// shouldSkipImage helps determine whether the batch should perform the mirroring of the image
// or if the image should be skipped.
func shouldSkipImage(img v2alpha1.CopyImageSchema, mode string, errArray []mirrorErrorSchema) (bool, error) {
	// In MirrorToMirror and MirrorToDisk, the release collector will generally build and push the graph image
	// to the destination registry (disconnected registry or cache resp.)
	// Therefore this image can be skipped.
	// OCPBUGS-38037: The only exception to this is in the enclave environment. Enclave environment is detected by the presence
	// of env var UPDATE_URL_OVERRIDE.
	// When in enclave environment, release collector cannot build nor push the graph image. Therefore graph image
	// should not be skipped.
	updateURLOverride := os.Getenv("UPDATE_URL_OVERRIDE")
	if img.Type == v2alpha1.TypeCincinnatiGraph && (mode == mirror.MirrorToDisk || mode == mirror.MirrorToMirror) && len(updateURLOverride) == 0 {
		return true, nil
	}

	if img.Type == v2alpha1.TypeOperatorBundle {
		for _, err := range errArray {
			bundleImage := img.Origin
			if strings.Contains(bundleImage, "://") {
				bundleImage = strings.Split(img.Origin, "://")[1]
			}

			if err.bundles != nil && err.bundles.Has(bundleImage) {
				return true, fmt.Errorf(skippingMsg, img.Origin)
			}
		}
	}

	return false, nil
}

func saveErrors(logger clog.PluggableLoggerInterface, logsDir string, errArray []mirrorErrorSchema) (string, error) {
	if len(errArray) > 0 {
		timestamp := time.Now().Format("20060102_150405")
		filename := fmt.Sprintf("mirroring_errors_%s.txt", timestamp)
		file, err := os.Create(filepath.Join(logsDir, filename))
		if err != nil {
			logger.Error(workerPrefix+"failed to create file: %s", err.Error())
			return filename, err
		}
		defer file.Close()

		for _, err := range errArray {
			errorMsg := formatErrorMsg(err)
			logger.Error(workerPrefix + errorMsg)
			fmt.Fprintln(file, errorMsg)
		}
		return filename, nil
	}
	return "", nil
}

func formatErrorMsg(err mirrorErrorSchema) string {
	if len(err.operators) > 0 || len(err.bundles) > 0 {
		return fmt.Sprintf("error mirroring image %s (Operator bundles: %v - Operators: %v) error: %s", err.image.Origin, maps.Values(err.bundles), maps.Keys(err.operators), err.err.Error())
	}

	return fmt.Sprintf("error mirroring image %s error: %s", err.image.Origin, err.err.Error())
}

func (s StringMap) Has(key string) bool {
	_, ok := s[key]
	return ok
}
