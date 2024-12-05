package batch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"golang.org/x/exp/maps"
)

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
