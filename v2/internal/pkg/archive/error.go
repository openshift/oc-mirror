package archive

import (
	"errors"
	"fmt"

	"github.com/openshift/oc-mirror/v2/internal/pkg/errcode"
)

type ArchiveError struct {
	ReleaseErr       error
	OperatorErr      error
	AdditionalImgErr error
	HelmErr          error
}

type SignatureBlobGathererError struct {
	SigError error
}

func (e *SignatureBlobGathererError) Error() string {
	return fmt.Sprintf("signature error: %s", e.SigError)
}

func (e *ArchiveError) Error() string {
	return fmt.Sprintf("archive error: %s", errors.Join(e.ReleaseErr, e.OperatorErr, e.AdditionalImgErr, e.HelmErr))
}

func (e *ArchiveError) ExitCode() int {
	if e == nil {
		return 0
	}

	exitCode := 0
	if e.ReleaseErr != nil {
		exitCode |= errcode.ReleaseErr
	}
	if e.OperatorErr != nil {
		exitCode |= errcode.OperatorErr
	}
	if e.AdditionalImgErr != nil {
		exitCode |= errcode.AdditionalImgErr
	}
	if e.HelmErr != nil {
		exitCode |= errcode.HelmErr
	}
	return exitCode
}
