package cli

import (
	"errors"
	"fmt"

	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
)

type NormalStorageInterruptError struct {
	message string
}

func (e *NormalStorageInterruptError) Error() string {
	return e.message
}

func NormalStorageInterruptErrorf(format string, a ...any) *NormalStorageInterruptError {
	return &NormalStorageInterruptError{
		message: fmt.Sprintf(format, a...),
	}
}

func (e *NormalStorageInterruptError) Is(err error) bool {
	_, ok := err.(*NormalStorageInterruptError)
	return ok
}

// CodeExiter is an interface implemented by errors that result in an exit code
type CodeExiter interface {
	ExitCode() int
}

// CollectionError is an aggregator of collection errors per type.
type CollectionError struct {
	ReleaseErr       error
	OperatorErr      error
	AdditionalImgErr error
	HelmErr          error
}

func (e *CollectionError) Error() string {
	return fmt.Sprintf("collection error: %s", errors.Join(e.ReleaseErr, e.OperatorErr, e.AdditionalImgErr, e.HelmErr))
}

func (e *CollectionError) ExitCode() int {
	if e == nil {
		return 0
	}

	exitCode := 0
	if e.ReleaseErr != nil {
		exitCode |= common.ReleaseErrCode
	}
	if e.OperatorErr != nil {
		exitCode |= common.OperatorErrCode
	}
	if e.AdditionalImgErr != nil {
		exitCode |= common.AdditionalImgErrCode
	}
	if e.HelmErr != nil {
		exitCode |= common.HelmErrCode
	}
	return exitCode
}
