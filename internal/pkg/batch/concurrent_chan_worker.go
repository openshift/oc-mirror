package batch

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/emoji"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/spinners"
)

const (
	skippingMsg              = "skipping operator bundle %s because one of its related images failed to mirror"
	noMatchingPlatformReason = "no instance for the requested platform(s)"

	errMsgHeader = "%ssome errors occurred during the mirroring"
	errMsg       = errMsgHeader + ".\n" +
		"\t Please review %s/%s for a list of mirroring errors.\n" +
		"\t You may consider:\n" +
		"\t * removing images or operators that cause the error from the image set config, and retrying\n" +
		"\t * keeping the image set config (images are mandatory for you), and retrying\n" +
		"\t * mirroring the failing images manually, if retries also fail."
)

type ChannelConcurrentBatch struct {
	Log              clog.PluggableLoggerInterface
	LogsDir          string
	Mirror           mirror.MirrorInterface
	MaxGoroutines    uint
	SynchedTimeStamp string
	OperandResolver  OperandResolver
}

type GoroutineResult struct {
	err     *mirrorErrorSchema
	imgType v2alpha1.ImageType
	img     v2alpha1.CopyImageSchema
	// skipped means intentionally not mirrored: neither copied nor failed. It
	// must stay out of errArray, which would drop the bundle listing it.
	skipped *v2alpha1.SkippedImage
}

// Worker - the main batch processor
func (o *ChannelConcurrentBatch) Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.CollectorSchema, error) {
	startTime := time.Now()

	copiedImages := v2alpha1.CollectorSchema{
		AllImages:       []v2alpha1.CopyImageSchema{},
		PlatformFilters: collectorSchema.PlatformFilters,
	}

	var errArray []mirrorErrorSchema
	var skippedImages []v2alpha1.SkippedImage

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

	total := len(collectorSchema.AllImages)

	o.Log.Info(emoji.Rocket+" Start %s the images...", mirrorMsg)
	o.Log.Info(emoji.Pushpin+" images to %s %d ", opts.Function, total)

	p := mpb.New(mpb.PopCompletedMode(), mpb.ContainerOptional(mpb.WithOutput(io.Discard), !opts.Global.IsTerminal))
	results := make(chan GoroutineResult, total)
	progressCh := make(chan int, total)
	semaphore := make(chan struct{}, o.MaxGoroutines)

	cancelCtx, cancel := context.WithCancel(ctx)
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

			semaphore <- struct{}{}

			sp := newSpinner(img, opts.LocalStorageFQDN, p)

			wg.Add(1)
			go func(cancelCtx context.Context, semaphore chan struct{}, results chan<- GoroutineResult, spinner *mpb.Bar) {
				defer wg.Done()
				defer func() { <-semaphore }()
				result := GoroutineResult{imgType: img.Type, img: img}

				m.Lock()
				skip, reason := shouldSkipImage(img, opts, errArray)
				m.Unlock()
				if skip {
					if reason != nil {
						result.err = &mirrorErrorSchema{image: img, err: reason}
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

				var err error
				var triggered bool
			loop:
				for {
					select {
					case <-cancelCtx.Done():
						spinner.Abort(false)
						break loop
					default:
						if !triggered {
							triggered = true
							timeoutCtx, _ := opts.Global.CommandTimeoutContext()

							options := opts
							if img.Type.IsOperatorCatalog() && img.RebuiltTag != "" {
								options.RemoveSignatures = true
								options.PreserveDigests = false
							}

							// If this image has specific platform requirements, use them
							if platforms, ok := collectorSchema.PlatformFilters[img.Origin]; ok && len(platforms) > 0 {
								strs := make([]string, len(platforms))
								for i, p := range platforms {
									strs[i] = p.String()
								}
								options.InstancePlatforms = strs
							}

							err = o.Mirror.Run(timeoutCtx, img.Source, img.Destination, mirror.Mode(opts.Function), &options) //nolint:contextcheck

							// "no instances found for platform" is only emitted by the copy library
							// for manifest lists (multi-arch indexes) when none of the instances
							// match the requested InstancePlatforms. The error itself confirms the
							// source is a manifest list, so retrying without the filter would mirror
							// all platforms and contradict the user's configuration.
							if err != nil && len(options.InstancePlatforms) > 0 && strings.Contains(err.Error(), "no instances found for platform") {
								requested := strings.Join(options.InstancePlatforms, ", ")

								// Only worth asking now: the answer costs a bundle pull.
								var class v2alpha1.ImageType
								var resolveErr error
								if img.Type.IsOperator() {
									class, resolveErr = o.classifyOperand(timeoutCtx, img, collectorSchema, opts) //nolint:contextcheck
								}

								switch {
								case class == v2alpha1.TypeOperatorOperand:
									// The catalog lists this, not the user. Operators ship
									// optional operands for other architectures that this
									// cluster never pulls, so skip rather than fail the operator.
									img.Type = class
									result.img = img
									result.imgType = img.Type
									o.Log.Warn("Skipping operand %s: no instance for the requested platform(s) %s", img.Origin, requested)
									result.skipped = &v2alpha1.SkippedImage{
										Origin:    img.Origin,
										Type:      img.Type,
										Reason:    noMatchingPlatformReason,
										Platforms: options.InstancePlatforms,
									}
									err = nil

								case resolveErr != nil:
									// Unclassified: keep the original error rather than claim
									// a lack of support we have not shown.
									o.Log.Warn("Unable to tell whether %s is an operand: %v. Treating it as an image the operator runs, so it will not be skipped.", img.Origin, resolveErr)

								case class == v2alpha1.TypeOperatorRelatedImage:
									// An image the operator runs itself having no matching
									// instance is the proof that it cannot run here. No catalog
									// annotation tells us this reliably.
									if operators := operatorNames(collectorSchema.CopyImageSchemaMap.OperatorsByImage[img.Origin]); operators != "" {
										err = fmt.Errorf("operator %s does not support %s: %w", operators, requested, err)
									}

								case !img.Type.IsOperator():
									// The user named this image explicitly, so a platform miss is
									// a configuration mistake.
									o.Log.Warn("Requested platform(s) not found in source manifest list for %s; verify the platforms field in your ISC", img.Origin)
								}
							}

							switch {
							case result.skipped != nil:
								spinner.Increment()
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
collect:
	for completed < len(collectorSchema.AllImages) {
		res := <-results
		err := res.err
		switch {
		case res.skipped != nil:
			// Out of both copiedImages and errArray, so the batch still
			// succeeds and the bundle listing it is still mirrored.
			m.Lock()
			skippedImages = append(skippedImages, *res.skipped)
			m.Unlock()
		case err == nil:
			logImageSuccess(o.Log, &res.img, &opts)
			copiedImages.AllImages = append(copiedImages.AllImages, res.img)
			incrementTotals(res.imgType, &copiedImages)
		default:
			m.Lock()
			errArray = append(errArray, *err)
			m.Unlock()

			logImageError(o.Log, &res.img, &opts)
			if res.imgType.IsRelease() {
				cancel()
				// Labelled: plain break would leave only the switch.
				break collect
			}
		}

		completed++
		progressCh <- 1
	}
	close(progressCh)

	p.Wait()

	copiedImages.SkippedImages = skippedImages

	logResults(o.Log, opts.Function, &copiedImages, &collectorSchema, skippedImages)
	logSkipped(o.Log, skippedImages)

	if len(errArray) > 0 {
		batchErr := &BatchError{
			releaseCountDiff:       collectorSchema.TotalReleaseImages - copiedImages.TotalReleaseImages,
			operatorCountDiff:      collectorSchema.TotalOperatorImages - copiedImages.TotalOperatorImages,
			additionalImgCountDiff: collectorSchema.TotalAdditionalImages - copiedImages.TotalAdditionalImages,
			helmCountDiff:          collectorSchema.TotalHelmImages - copiedImages.TotalHelmImages,
		}
		filename, err := saveErrors(o.Log, o.LogsDir, o.SynchedTimeStamp, errArray)
		if err != nil {
			batchErr.source = fmt.Errorf(errMsgHeader+" - unable to log these errors in %s/%s: %w", workerPrefix, o.LogsDir, filename, err)
		} else {
			batchErr.source = fmt.Errorf(errMsg, workerPrefix, o.LogsDir, filename)
		}
		return copiedImages, batchErr
	}
	o.Log.Debug("concurrent channel worker time     : %v", time.Since(startTime))
	return copiedImages, nil
}

// classifyOperand returns TypeInvalid when nothing classified the image, so an
// unanswered question is never mistaken for proof the operator runs it.
//
// Disk-to-mirror and delete cannot reach the bundle's registry, and do not need
// to: the executor drops operands recorded during mirror-to-disk.
func (o *ChannelConcurrentBatch) classifyOperand(ctx context.Context, img v2alpha1.CopyImageSchema, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.ImageType, error) {
	if o.OperandResolver == nil || opts.IsDiskToMirror() || opts.IsDeleteMode() {
		return v2alpha1.TypeInvalid, nil
	}
	bundles := collectorSchema.CopyImageSchemaMap.BundlesByImage[img.Origin]
	if len(bundles) == 0 {
		return v2alpha1.TypeInvalid, nil
	}

	operand, err := o.OperandResolver.IsOperand(ctx, img.Origin, bundles)
	switch {
	case err != nil:
		return v2alpha1.TypeInvalid, err
	case operand:
		return v2alpha1.TypeOperatorOperand, nil
	default:
		return v2alpha1.TypeOperatorRelatedImage, nil
	}
}

// operatorNames renders an image's operators, sorted so the message is stable.
func operatorNames(operators map[string]struct{}) string {
	if len(operators) == 0 {
		return ""
	}
	names := make([]string, 0, len(operators))
	for name := range operators {
		names = append(names, strconv.Quote(name))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
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

func logResults(log clog.PluggableLoggerInterface, copyMode string, copiedImages, collectorSchema *v2alpha1.CollectorSchema, skipped []v2alpha1.SkippedImage) {
	log.Info("=== Results ===")

	var copyModeMsg string
	if copyMode == string(mirror.CopyMode) {
		copyModeMsg = "mirrored"
	} else {
		copyModeMsg = "deleted"
	}

	// Skipped images are only ever operands, counted in the operator totals.
	logResult(log, copyModeMsg, "release", copiedImages.TotalReleaseImages, collectorSchema.TotalReleaseImages, 0)
	logResult(log, copyModeMsg, "operator", copiedImages.TotalOperatorImages, collectorSchema.TotalOperatorImages, len(skipped))
	logResult(log, copyModeMsg, "additional", copiedImages.TotalAdditionalImages, collectorSchema.TotalAdditionalImages, 0)
	logResult(log, copyModeMsg, "helm", copiedImages.TotalHelmImages, collectorSchema.TotalHelmImages, 0)
}

func logResult(log clog.PluggableLoggerInterface, copyMode, imageType string, copied, total, skipped int) {
	if total == 0 {
		return
	}
	switch {
	case copied == total:
		log.Info(emoji.SpinnerCheckMark+" %d / %d %s images %s successfully", copied, total, imageType, copyMode)
	case copied+skipped == total:
		// Nothing failed: the remainder was intentionally skipped.
		log.Info(emoji.SpinnerCheckMark+" %d / %d %s images %s successfully (%d skipped)", copied, total, imageType, copyMode, skipped)
	default:
		log.Info(emoji.SpinnerCrossMark+" %d / %d %s images %s: Some %s images failed to be %s - please check the logs", copied, total, imageType, copyMode, imageType, copyMode)
	}
}

// logSkipped reports what was intentionally left out, and why.
func logSkipped(log clog.PluggableLoggerInterface, skipped []v2alpha1.SkippedImage) {
	if len(skipped) == 0 {
		return
	}
	log.Info("=== Skipped images ===")
	for _, img := range skipped {
		if len(img.Platforms) > 0 {
			log.Info("%s: %s %s", img.Origin, img.Reason, strings.Join(img.Platforms, ", "))
		} else {
			log.Info("%s: %s", img.Origin, img.Reason)
		}
	}
}

func logImageSuccess(log clog.PluggableLoggerInterface, image *v2alpha1.CopyImageSchema, opts *mirror.CopyOptions) {
	if opts.Global.IsTerminal {
		// It'll be printed by the spinner
		return
	}

	var dest string
	if strings.Contains(image.Destination, opts.LocalStorageFQDN) {
		dest = "cache"
	} else {
		dest = hostNamespace(image.Destination)
	}

	action := "copying"
	if opts.IsDelete() {
		action = "deleting"
	}

	log.Info("Success %s %s %s %s", action, image.Origin, emoji.RightArrow, dest)
}

func logImageError(log clog.PluggableLoggerInterface, image *v2alpha1.CopyImageSchema, opts *mirror.CopyOptions) {
	if opts.Global.IsTerminal {
		// It'll be printed by the spinner
		return
	}

	action := "copy"
	if opts.IsDelete() {
		action = "delete"
	}

	log.Error("Failed to %s %s %s", action, image.Type, image.Origin)
}

func newSpinner(img v2alpha1.CopyImageSchema, localStorage string, p *mpb.Progress) *mpb.Bar {
	imageText := " " + path.Base(img.Origin) + " "
	if strings.Contains(img.Destination, localStorage) {
		imageText += emoji.RightArrow + "  cache "
	} else {
		imageText += emoji.RightArrow + "  " + hostNamespace(img.Destination) + " "
	}

	return spinners.AddSpinner(p, imageText)
}

func newOverallProgress(p *mpb.Progress, total int) *mpb.Bar {
	return p.AddBar(
		int64(total),
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
		case _, ok := <-progressCh:
			if !ok {
				// channel closed (end of progress)
				overallProgress.Abort(false)
				return
			}
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
	case v2alpha1.TypeOperatorBundle, v2alpha1.TypeOperatorCatalog, v2alpha1.TypeOperatorRelatedImage, v2alpha1.TypeOperatorOperand:
		copiedImages.TotalOperatorImages++
	case v2alpha1.TypeHelmImage:
		copiedImages.TotalHelmImages++
	}
}

// shouldSkipImage helps determine whether the batch should perform the mirroring of the image
// or if the image should be skipped.
func shouldSkipImage(img v2alpha1.CopyImageSchema, opts mirror.CopyOptions, errArray []mirrorErrorSchema) (bool, error) {
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
