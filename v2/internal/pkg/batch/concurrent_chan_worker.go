package batch

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/spinners"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

var (
	copiedImages v2alpha1.CollectorSchema

	errMsgHeader = "%ssome errors occurred during the mirroring"
	errMsg       = errMsgHeader + ".\n" +
		"\t Please review %s/%s for a list of mirroring errors.\n" +
		"\t You may consider:\n" +
		"\t * removing images or operators that cause the error from the image set config, and retrying\n" +
		"\t * keeping the image set config (images are mandatory for you), and retrying\n" +
		"\t * mirroring the failing images manually, if retries also fail."
)

type ChannelConcurrentBatch struct {
	Log           clog.PluggableLoggerInterface
	LogsDir       string
	Mirror        mirror.MirrorInterface
	MaxGoroutines int
}

type GoroutineResult struct {
	err     *mirrorErrorSchema
	imgType v2alpha1.ImageType
	img     v2alpha1.CopyImageSchema
}

// Worker - the main batch processor
func (o *ChannelConcurrentBatch) Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.CollectorSchema, error) {
	startTime := time.Now()

	copiedImages = v2alpha1.CollectorSchema{
		AllImages: []v2alpha1.CopyImageSchema{},
	}

	var errArray []mirrorErrorSchema
	//OCPBUGS-44448
	//TODO this bug fix will work only for images origin by digest only
	//to cover scenarios where one digest is tagged by several images it is necessary to add a field on
	//CopyImageSchema to store the digest number and rely on this field instead of img.Origin
	alreadyDeleted := make(onlyKeyMap)

	var m sync.RWMutex
	var wg sync.WaitGroup

	var mirrorMsg string
	switch {
	case opts.IsCopy():
		mirrorMsg = "copying"
	case opts.IsDelete():
		mirrorMsg = "deleting"
	}

	opts.PreserveDigests = true

	o.Log.Info("🚀 Start " + mirrorMsg + " the images...")
	o.Log.Info("\U0001F4CC images to %s %d ", opts.Function, len(collectorSchema.AllImages))

	total := len(collectorSchema.AllImages)
	p := mpb.New()
	results := make(chan GoroutineResult, total)
	progressCh := make(chan int, total)
	semaphore := make(chan struct{}, o.MaxGoroutines)
	cancelCtx, cancel := opts.Global.CommandTimeoutContext()
	defer cancel()

	go func() {
		defer close(results)
		defer close(semaphore)

		for _, img := range collectorSchema.AllImages {

			select {
			case <-cancelCtx.Done():
				wg.Wait()
				return
			default:
			}

			sp := newSpinner(img, opts.LocalStorageFQDN, p)

			semaphore <- struct{}{}
			wg.Add(1)
			go func(ctx context.Context, semaphore chan struct{}, results chan<- GoroutineResult, spinner *mpb.Bar) {
				defer wg.Done()
				defer func() { <-semaphore }()
				result := GoroutineResult{}

				m.Lock()
				skip, reason := shouldSkipImage(img, opts, errArray, alreadyDeleted)
				m.Unlock()
				if skip {
					if reason != nil {
						result.err = &mirrorErrorSchema{image: img, err: reason}
					}

					result.imgType = img.Type
					result.img = img

					//OCPBUGS-44448
					if opts.IsDelete() {
						spinner.Increment()
						results <- result
						return
					}

					switch img.Type {
					case v2alpha1.TypeOperatorBundle:
						spinner.Abort(false)
					case v2alpha1.TypeCincinnatiGraph:
						spinner.Increment()
					}

					results <- result
					return
				}

				result.imgType = img.Type
				result.img = img

				var err error
				var triggered bool
			loop:
				for {
					select {
					case <-ctx.Done():
						spinner.Abort(false)
						break loop
					default:
						if !triggered {
							triggered = true
							err = o.Mirror.Run(ctx, img.Source, img.Destination, mirror.Mode(opts.Function), &opts)

							switch {
							case err == nil:
								spinner.Increment()
							case img.Type.IsOperator():
								operators := collectorSchema.CopyImageSchemaMap.OperatorsByImage[img.Origin]
								bundles := collectorSchema.CopyImageSchemaMap.BundlesByImage[img.Origin]
								result.err = &mirrorErrorSchema{image: img, err: err, operators: operators, bundles: bundles}
								spinner.Abort(false)
							case img.Type.IsRelease() || img.Type.IsAdditionalImage() || img.Type.IsHelmImage():
								result.err = &mirrorErrorSchema{image: img, err: err}
								spinner.Abort(false)
							}
							results <- result
							break loop
						}

					}
				}
			}(cancelCtx, semaphore, results, sp)
		}
		wg.Wait()
	}()

	overallProgress := newOverallProgress(p, total)

	go runOverallProgress(overallProgress, cancelCtx, progressCh)

	completed := 0
	for completed < len(collectorSchema.AllImages) {
		res := <-results
		err := res.err
		if err == nil {
			copiedImages.AllImages = append(copiedImages.AllImages, res.img)
			incrementTotals(res.imgType, &copiedImages)

			//OCPBUGS-44448
			if opts.IsDelete() {
				m.Lock()
				alreadyDeleted[res.img.Origin] = struct{}{}
				m.Unlock()
			}

		} else {
			m.Lock()
			errArray = append(errArray, *err)
			m.Unlock()

			if res.imgType.IsRelease() {
				cancel()
				break
			}
		}

		completed++
		progressCh <- 1
	}
	close(progressCh)

	p.Wait()

	logResults(o.Log, opts.Function, &copiedImages, &collectorSchema)

	if len(errArray) > 0 {
		filename, err := saveErrors(o.Log, o.LogsDir, errArray)
		if err != nil {
			return copiedImages, NewSafeError(errMsgHeader+" - unable to log these errors in %s/%s: %s", workerPrefix, o.LogsDir, filename, err.Error())
		} else {
			msg := fmt.Sprintf(errMsg, workerPrefix, o.LogsDir, filename)
			return copiedImages, NewSafeError(msg)
		}
	}
	endTime := time.Now()
	execTime := endTime.Sub(startTime)
	o.Log.Debug("concurrent channel worker time     : %v", execTime)
	return collectorSchema, nil
}

func hostNamespace(input string) string {
	parsedURL, err := url.Parse(input)
	if err != nil {
		return ""
	}

	host := parsedURL.Host
	pathSegments := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")

	if len(pathSegments) > 1 {
		namespacePath := strings.Join(pathSegments[:len(pathSegments)-1], "/")
		hostAndNamespace := path.Join(host, namespacePath) + "/"
		return hostAndNamespace
	} else if len(pathSegments) == 1 {
		return path.Join(host, pathSegments[0]) + "/"
	} else {
		return host
	}
}

func logResults(log clog.PluggableLoggerInterface, copyMode string, copiedImages, collectorSchema *v2alpha1.CollectorSchema) {
	log.Info("=== Results ===")

	var copyModeMsg string
	if copyMode == string(mirror.CopyMode) {
		copyModeMsg = "mirrored"
	} else {
		copyModeMsg = "deleted"
	}

	logResult(log, copyModeMsg, "release", copiedImages.TotalReleaseImages, collectorSchema.TotalReleaseImages)
	logResult(log, copyModeMsg, "operator", copiedImages.TotalOperatorImages, collectorSchema.TotalOperatorImages)
	logResult(log, copyModeMsg, "additional", copiedImages.TotalAdditionalImages, collectorSchema.TotalAdditionalImages)
	logResult(log, copyModeMsg, "helm", copiedImages.TotalHelmImages, collectorSchema.TotalHelmImages)
}

func logResult(log clog.PluggableLoggerInterface, copyMode, imageType string, copied, total int) {
	if total != 0 {
		if copied == total {
			log.Info("✅ %d / %d %s images %s successfully", copied, total, imageType, copyMode)
		} else {
			log.Info("❌ %d / %d %s images %s: Some %s images failed to be %s - please check the logs", copied, total, imageType, copyMode, imageType, copyMode)
		}
	}
}

func newSpinner(img v2alpha1.CopyImageSchema, localStorage string, p *mpb.Progress) *mpb.Bar {
	imageText := " " + path.Base(img.Origin) + " "
	if strings.Contains(img.Destination, localStorage) {
		imageText += "➡️  cache "
	} else {
		imageText += "➡️  " + hostNamespace(img.Destination) + " "
	}

	return p.AddSpinner(
		1,
		// mpb.BarFillerMiddleware(spinners.PositionSpinnerLeft),//TODO ALEX - check with @sherine-k why it was necessary before removing
		mpb.BarWidth(3),
		mpb.PrependDecorators(
			decor.OnComplete(spinners.EmptyDecorator(), "\x1b[1;92m ✓ \x1b[0m"),
			decor.OnAbort(spinners.EmptyDecorator(), "\x1b[1;91m ✗ \x1b[0m"),
		),
		mpb.BarRemoveOnComplete(),
		mpb.AppendDecorators(
			decor.Name("("),
			decor.Elapsed(decor.ET_STYLE_GO),
			decor.Name(")"),
			decor.Name(imageText),
		),
		mpb.BarFillerClearOnComplete(),
		spinners.BarFillerClearOnAbort(),
	)
}

func newOverallProgress(p *mpb.Progress, total int) *mpb.Bar {
	return p.AddBar(int64(total),
		mpb.PrependDecorators(
			decor.CountersNoUnit("%d / %d"),
			decor.Name(" ("),
			decor.Elapsed(decor.ET_STYLE_GO),
			decor.Name(")"),
		),
		mpb.AppendDecorators(
			decor.Percentage(),
		),
		mpb.BarPriority(total+1),
	)
}

func runOverallProgress(overallProgress *mpb.Bar, cancelCtx context.Context, progressCh chan int) {
	var progress int

	for {
		select {
		case <-cancelCtx.Done():
			overallProgress.Abort(false)
			return
		case <-progressCh:
			progress++
			overallProgress.SetCurrent(int64(progress))
		}
	}

}

func incrementTotals(imgType v2alpha1.ImageType, copiedImages *v2alpha1.CollectorSchema) {
	switch imgType {
	case v2alpha1.TypeCincinnatiGraph, v2alpha1.TypeOCPRelease, v2alpha1.TypeOCPReleaseContent:
		copiedImages.TotalReleaseImages++
	case v2alpha1.TypeGeneric:
		copiedImages.TotalAdditionalImages++
	case v2alpha1.TypeOperatorBundle, v2alpha1.TypeOperatorCatalog, v2alpha1.TypeOperatorRelatedImage:
		copiedImages.TotalOperatorImages++
	case v2alpha1.TypeHelmImage:
		copiedImages.TotalHelmImages++
	}
}

// shouldSkipImage helps determine whether the batch should perform the mirroring of the image
// or if the image should be skipped.
func shouldSkipImage(img v2alpha1.CopyImageSchema, opts mirror.CopyOptions, errArray []mirrorErrorSchema, alreadyDeleted onlyKeyMap) (bool, error) {
	// In MirrorToMirror and MirrorToDisk, the release collector will generally build and push the graph image
	// to the destination registry (disconnected registry or cache resp.)
	// Therefore this image can be skipped.
	// OCPBUGS-38037: The only exception to this is in the enclave environment. Enclave environment is detected by the presence
	// of env var UPDATE_URL_OVERRIDE.
	// When in enclave environment, release collector cannot build nor push the graph image. Therefore graph image
	// should not be skipped.
	updateURLOverride := os.Getenv("UPDATE_URL_OVERRIDE")
	if img.Type == v2alpha1.TypeCincinnatiGraph && (opts.Mode == mirror.MirrorToDisk || opts.Mode == mirror.MirrorToMirror) && len(updateURLOverride) == 0 {
		return true, nil
	}

	//OCPBUGS-44448
	if opts.IsDelete() {
		if len(alreadyDeleted) > 0 && alreadyDeleted.Has(img.Origin) {
			return true, nil
		}
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
