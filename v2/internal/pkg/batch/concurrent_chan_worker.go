package batch

import (
	"context"
	"fmt"
	"net/url"
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

var copiedImages v2alpha1.CollectorSchema

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

	copiedImages = v2alpha1.CollectorSchema{
		AllImages: []v2alpha1.CopyImageSchema{},
	}

	var errArray []mirrorErrorSchema
	var m sync.RWMutex
	var wg sync.WaitGroup

	var mirrorMsg string
	if opts.Function == string(mirror.CopyMode) {
		mirrorMsg = "copying"
	} else if opts.Function == string(mirror.DeleteMode) {
		mirrorMsg = "deleting"
	}

	opts.PreserveDigests = true

	startTime := time.Now()

	o.Log.Info("🚀 Start " + mirrorMsg + " the images...")

	o.Log.Info("images to %s %d ", opts.Function, len(collectorSchema.AllImages))

	imgOverallIndex := 0
	p := mpb.New()
	total := len(collectorSchema.AllImages)
	results := make(chan GoroutineResult, len(collectorSchema.AllImages))
	progressCh := make(chan int, total)
	semaphore := make(chan struct{}, o.MaxGoroutines)
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		for _, img := range collectorSchema.AllImages {

			select {
			case <-cancelCtx.Done():
				return
			default:
			}

			if cancelCtx.Err() != nil {
				break
			}
			imgOverallIndex++
			counterText := fmt.Sprintf("%d/%d : (", imgOverallIndex, total)
			imageText := ") " + path.Base(img.Origin) + " "
			if strings.Contains(img.Destination, opts.LocalStorageFQDN) {
				imageText += "➡️  cache "
			} else {
				imageText += "➡️  " + hostNamespace(img.Destination) + " "
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
				mpb.BarPriority(imgOverallIndex),
			)
			semaphore <- struct{}{}
			wg.Add(1)
			go func(ctx context.Context, semaphore chan struct{}, results chan<- GoroutineResult) {
				defer wg.Done()
				defer func() { <-semaphore }()
				result := GoroutineResult{}

				m.Lock()
				skip, reason := shouldSkipImage(img, opts.Mode, errArray)
				m.Unlock()
				if skip {
					if reason != nil {
						result.err = &mirrorErrorSchema{image: img, err: reason}
					}

					result.imgType = img.Type
					result.img = img

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
				select {
				case <-ctx.Done():
					spinner.Abort(false)
					return
				default:
					err = o.Mirror.Run(ctx, img.Source, img.Destination, mirror.Mode(opts.Function), &opts)
				}

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
			}(cancelCtx, semaphore, results)

		}
		wg.Wait()
		close(results)
		close(semaphore)
	}()

	fmt.Println()

	// Overall progress bar, added last to ensure it stays at the bottom
	overallProgress := p.AddBar(int64(total),
		mpb.PrependDecorators(
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.AppendDecorators(
			decor.Percentage(),
		),
		mpb.BarPriority(total+1),
	)

	// Goroutine to update overall progress bar based on channel updates
	go func(ctx context.Context) {
		select {
		case <-cancelCtx.Done():
			return
		default:
		}

		progress := 0
		for update := range progressCh {
			progress += update
			overallProgress.SetCurrent(int64(progress))
		}
	}(cancelCtx)

	completed := 0
	for completed < len(collectorSchema.AllImages) {
		res := <-results
		err := res.err
		if err == nil {
			copiedImages.AllImages = append(copiedImages.AllImages, res.img)
			// TODO ALEX PUT IT IN A FUNC
			imgType := res.imgType
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
		} else {

			m.Lock()
			errArray = append(errArray, *err)
			m.Unlock()

			if res.imgType.IsRelease() {
				cancel()
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
			return copiedImages, NewSafeError(workerPrefix+"some errors occurred during the mirroring - unable to log these errors in %s: %v", o.LogsDir+"/"+filename, err)
		} else {
			msg := workerPrefix + "some errors occurred during the mirroring.\n" +
				"\t Please review " + o.LogsDir + "/" + filename + " for a list of mirroring errors.\n" +
				"\t You may consider:\n" +
				"\t * removing images or operators that cause the error from the image set config, and retrying\n" +
				"\t * keeping the image set config (images are mandatory for you), and retrying\n" +
				"\t * mirroring the failing images manually, if retries also fail."
			return copiedImages, NewSafeError(msg)
		}
	}
	endTime := time.Now()
	execTime := endTime.Sub(startTime)
	o.Log.Debug("batch time     : %v", execTime)
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
