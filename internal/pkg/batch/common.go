package batch

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/openshift/oc-mirror/v2/internal/pkg/errcode"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// imageTimeoutHint is built from DefaultImageTimeout so the hint stays
// aligned with the --image-timeout flag default (OCPBUGS-111840).
func imageTimeoutHint() string {
	return " hint: image mirroring timed out (context deadline exceeded). " +
		"Retry with a higher --image-timeout (default " +
		mirror.DefaultImageTimeout.String() + ")."
}

type BatchError struct {
	source                 error
	releaseCountDiff       int
	operatorCountDiff      int
	additionalImgCountDiff int
	helmCountDiff          int
}

func (err *BatchError) Error() string {
	return err.source.Error()
}

func (err *BatchError) ExitCode() int {
	if err == nil {
		return 0
	}
	exitCode := 0
	if err.releaseCountDiff != 0 {
		exitCode |= errcode.ReleaseErr
	}
	if err.operatorCountDiff != 0 {
		exitCode |= errcode.OperatorErr
	}
	if err.additionalImgCountDiff != 0 {
		exitCode |= errcode.AdditionalImgErr
	}
	if err.helmCountDiff != 0 {
		exitCode |= errcode.HelmErr
	}
	return exitCode
}

func saveErrors(logger clog.PluggableLoggerInterface, logsDir, timestamp string, errArray []mirrorErrorSchema) (string, error) {
	if len(errArray) > 0 {
		filename := fmt.Sprintf("mirroring_errors_%s.txt", timestamp)
		file, err := os.Create(filepath.Join(logsDir, filename))
		if err != nil {
			logger.Error(workerPrefix+"failed to create file: %s", err.Error())
			return filename, err
		}
		defer file.Close()

		for _, err := range errArray {
			errorMsg := formatErrorMsg(err)
			logger.Error("%s", workerPrefix+errorMsg)
			fmt.Fprintln(file, errorMsg)
		}
		return filename, nil
	}
	return "", nil
}

func formatErrorMsg(err mirrorErrorSchema) string {
	detail := annotateImageTimeoutError(err.err)
	if len(err.operators) > 0 || len(err.bundles) > 0 {
		bundles := slices.Sorted(maps.Values(err.bundles))
		operators := slices.Sorted(maps.Keys(err.operators))
		return fmt.Sprintf("error mirroring image %s (Operator bundles: %v - Operators: %v) error: %s", err.image.Origin, bundles, operators, detail)
	}

	return fmt.Sprintf("error mirroring image %s error: %s", err.image.Origin, detail)
}

// annotateImageTimeoutError appends remediation guidance when mirroring fails
// because the per-image --image-timeout budget was exhausted.
func annotateImageTimeoutError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "--image-timeout") {
		return msg
	}
	if errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(strings.ToLower(msg), "context deadline exceeded") {
		return msg + imageTimeoutHint()
	}
	return msg
}

func (s StringMap) Has(key string) bool {
	_, ok := s[key]
	return ok
}
