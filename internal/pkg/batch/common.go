package batch

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/openshift/oc-mirror/v2/internal/pkg/errcode"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

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
			logger.Error(workerPrefix + errorMsg)
			fmt.Fprintln(file, errorMsg)
		}
		return filename, nil
	}
	return "", nil
}

func formatErrorMsg(err mirrorErrorSchema) string {
	if len(err.operators) > 0 || len(err.bundles) > 0 {
		bundles := slices.Sorted(maps.Values(err.bundles))
		operators := slices.Sorted(maps.Keys(err.operators))
		return fmt.Sprintf("error mirroring image %s (Operator bundles: %v - Operators: %v) error: %s", err.image.Origin, bundles, operators, err.err.Error())
	}

	return fmt.Sprintf("error mirroring image %s error: %s", err.image.Origin, err.err.Error())
}

func (s StringMap) Has(key string) bool {
	_, ok := s[key]
	return ok
}
