package batch

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"golang.org/x/exp/maps"
)

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
