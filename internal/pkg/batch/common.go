package batch

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/openshift/oc-mirror/v2/internal/pkg/errcode"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

// unauthorizedBlobUploadHint clarifies that destination unauthorized during blob
// upload can mean either push auth/permissions or nested-path limits — distinct remediations.
const unauthorizedBlobUploadHint = " hint: destination registry returned unauthorized while uploading a blob. " +
	"This commonly means missing or insufficient push credentials/permissions for the destination repository path — " +
	"verify destination auth and push rights (not only source pull credentials). " +
	"Separately, some destination registries reject deep nested repository paths with the same unauthorized response; " +
	"that case is unrelated to credentials and is addressed with --max-nested-paths. " +
	"Treat push auth/permissions and nested-path limits as separate remediations."

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
	detail := annotateUnauthorizedUploadError(err.err)
	if len(err.operators) > 0 || len(err.bundles) > 0 {
		bundles := slices.Sorted(maps.Values(err.bundles))
		operators := slices.Sorted(maps.Keys(err.operators))
		return fmt.Sprintf("error mirroring image %s (Operator bundles: %v - Operators: %v) error: %s", err.image.Origin, bundles, operators, detail)
	}

	return fmt.Sprintf("error mirroring image %s error: %s", err.image.Origin, detail)
}

// annotateUnauthorizedUploadError appends remediation guidance when a destination
// blob upload fails with unauthorized. Auth/permission failures and nested-path
// rejections often share that wording; callers should not treat them as one cause.
func annotateUnauthorizedUploadError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "unauthorized") {
		return msg
	}
	// containers/image and registry clients typically phrase push layer failures this way.
	if strings.Contains(lower, "unable to upload blob") ||
		strings.Contains(lower, "uploading blob") ||
		strings.Contains(lower, "pushing blob") ||
		strings.Contains(lower, "error uploading layer") {
		return msg + unauthorizedBlobUploadHint
	}
	return msg
}

func (s StringMap) Has(key string) bool {
	_, ok := s[key]
	return ok
}
