package batch

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	distributionErrcode "github.com/docker/distribution/registry/api/errcode"
	"go.podman.io/image/v5/docker"

	"github.com/openshift/oc-mirror/v2/internal/pkg/errcode"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

// unauthorizedBlobUploadHint is a short per-image note. Destination unauthorized
// during blob upload can mean missing push credentials/permissions or a nested-path
// limit; those need different remediations.
const unauthorizedBlobUploadHint = " hint: check destination push credentials/permissions, or use --max-nested-paths for deep paths."

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
	if !isUnauthorizedOrDenied(err, lower) {
		return msg
	}
	// containers/image and registry clients typically phrase push layer failures this way.
	if isBlobUploadFailure(lower) {
		return msg + unauthorizedBlobUploadHint
	}
	return msg
}

// isUnauthorizedOrDenied prefers typed credential/registry auth errors when present
// and falls back to message text only when no recognized typed error exists.
func isUnauthorizedOrDenied(err error, lower string) bool {
	var credentialErr docker.ErrUnauthorizedForCredentials
	if errors.As(err, &credentialErr) {
		return true
	}

	var registryErr distributionErrcode.ErrorCoder
	if errors.As(err, &registryErr) {
		code := registryErr.ErrorCode()
		return code == distributionErrcode.ErrorCodeUnauthorized ||
			code == distributionErrcode.ErrorCodeDenied
	}

	return strings.Contains(lower, "unauthorized")
}

func isBlobUploadFailure(lower string) bool {
	return strings.Contains(lower, "unable to upload blob") ||
		strings.Contains(lower, "uploading blob") ||
		strings.Contains(lower, "writing blob") ||
		strings.Contains(lower, "pushing blob") ||
		strings.Contains(lower, "error uploading layer")
}

func (s StringMap) Has(key string) bool {
	_, ok := s[key]
	return ok
}
