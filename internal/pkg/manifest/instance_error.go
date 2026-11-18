package manifest

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/types"
)

// InstanceManifestError records a failed Docker child read after a successful open
// and cleanup. It establishes operation provenance, not that content is absent.
type InstanceManifestError struct {
	Source   string
	Instance digest.Digest
	Err      error
}

func (failure *InstanceManifestError) Error() string { return failure.Err.Error() }
func (failure *InstanceManifestError) Unwrap() error { return failure.Err }

// InstanceReadError returns the child cause only for an exact source and digest
// match in a single-cause chain. Joined failures cannot authorize an omission.
func InstanceReadError(err error, source string, instance digest.Digest) error {
	for current := err; current != nil; current = errors.Unwrap(current) {
		if _, multiple := current.(interface{ Unwrap() []error }); multiple {
			return nil
		}
	}
	var failure *InstanceManifestError
	if errors.As(err, &failure) && failure != nil && failure.Source == source && failure.Instance == instance {
		return failure.Err
	}
	return nil
}

func sourceIsLive(source types.ImageSource) bool {
	if source == nil {
		return false
	}
	value := reflect.ValueOf(source)
	return value.Kind() != reflect.Pointer || !value.IsNil()
}

func closeFailedSource(source types.ImageSource, err error) error {
	if sourceIsLive(source) {
		if closeErr := source.Close(); closeErr != nil {
			return fmt.Errorf("%w; closing failed image source: %w", err, closeErr)
		}
	}
	return err
}
