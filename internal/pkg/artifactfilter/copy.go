package artifactfilter

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/opencontainers/go-digest"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

// RetryCopyWithoutExcluded is called after the normal copy of src with options
// finally failed with copyErr. It retries the copy at most once through
// copyImage, leaving out the entries excluded by the enabled rule, when:
//
//   - options selects copy.CopyAllImages,
//   - neither ctx nor copyErr reports cancellation or a deadline,
//   - the source index can be inspected, and
//   - it lists excluded entries and at least one entry not excluded by the rule.
//
// copyImage applies the normal retry policy to that single retry. Otherwise
// copyErr is retained in the error chain. Inspection and retry failures retain
// their causes as well. options is not modified.
//
// TODO: remove filterAttestationInstances when OCI1.1 is supported by containers-libs [OCPNODE-2018](https://redhat.atlassian.net/browse/OCPNODE-2018)
// OCPBUGS-62723: attestation entries cannot be pulled from proxy registries, so
// the retry selects only the real entries by digest and skips the unpullable
// attestation manifests.
func RetryCopyWithoutExcluded(ctx context.Context, log clog.PluggableLoggerInterface, src string, srcRef types.ImageReference, options *copy.Options, copyErr error, copyImage func(types.ImageReference, *copy.Options) error) error {
	if options.ImageListSelection != copy.CopyAllImages || ctx.Err() != nil ||
		errors.Is(copyErr, context.Canceled) || errors.Is(copyErr, context.DeadlineExceeded) {
		return copyErr
	}

	if srcRef.Transport().Name() != "docker" {
		return copyErr
	}
	filtered, expected, err := filterAttestationInstances(ctx, srcRef, options.SourceCtx)
	if err != nil {
		log.Debug("attestation check skipped for %s: %v", src, err)
		return fmt.Errorf("%w; attestation check failed: %w", copyErr, err)
	}
	if !filtered.narrowed() {
		return copyErr
	}
	if ctx.Err() != nil {
		return fmt.Errorf("%w; attestation check canceled: %w", copyErr, ctx.Err())
	}

	// tests/integration/attestation_archive_test.go matches "retrying once without".
	log.Warn("copying %s failed: %v; retrying once without %d attestation manifest(s)", src, copyErr, len(filtered.Excluded))
	retry := *options
	retry.ImageListSelection = copy.CopySpecificImages
	retry.Instances = filtered.Retained
	retry.InstancePlatforms = nil
	if err := copyImage(guardedReference{ImageReference: srcRef, expected: expected}, &retry); err != nil {
		return fmt.Errorf("%w; retrying without attestation manifests also failed: %w", copyErr, err)
	}
	return nil
}

// filterAttestationInstances inspects a source manifest list and classifies its
// entries. Attestation manifests cannot be copied from proxy registries like
// registry.connect.redhat.com.
//
// Returns an empty instanceSelection if the source is not a manifest list.
func filterAttestationInstances(ctx context.Context, srcRef types.ImageReference, sysCtx *types.SystemContext) (selection instanceSelection, expected digest.Digest, retErr error) {
	src, err := srcRef.NewImageSource(ctx, sysCtx)
	if err != nil {
		return instanceSelection{}, "", closeSourceOnFailure(src, fmt.Errorf("opening image source for attestation check: %w", err))
	}
	if !sourceIsLive(src) {
		return instanceSelection{}, "", fmt.Errorf("opening attestation inspection returned no source")
	}
	defer func() {
		if err := src.Close(); err != nil {
			retErr = noteCloseFailure(retErr, "closing image source for attestation check", err)
		}
	}()

	manifestBytes, mimeType, err := src.GetManifest(ctx, nil)
	if err != nil {
		return instanceSelection{}, "", fmt.Errorf("reading manifest for attestation check: %w", err)
	}

	if !manifest.MIMETypeIsMultiImage(mimeType) {
		return instanceSelection{}, "", nil
	}

	var list manifest.List
	if mimeType == specv1.MediaTypeImageIndex {
		list, err = manifest.OCI1IndexFromManifest(manifestBytes)
	} else {
		list, err = manifest.ListFromBlob(manifestBytes, mimeType)
	}
	if err != nil {
		return instanceSelection{}, "", fmt.Errorf("parsing manifest list for attestation check: %w", err)
	}

	expected, err = manifest.Digest(manifestBytes)
	if err != nil {
		return instanceSelection{}, "", fmt.Errorf("digesting inspected manifest: %w", err)
	}
	selection, err = selectInstances(list)
	return selection, expected, err
}

// guardedReference preserves the original reference and policy identity while
// checking the opened index snapshot. Returning the raw source preserves private
// transport capabilities; pinning or wrapping it would change those contracts.
type guardedReference struct {
	types.ImageReference
	expected digest.Digest
}

//nolint:ireturn // implements types.ImageReference and must return the unwrapped source
func (ref guardedReference) NewImageSource(ctx context.Context, system *types.SystemContext) (types.ImageSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("attestation fallback canceled: %w", err)
	}
	source, err := ref.ImageReference.NewImageSource(ctx, system)
	if err != nil {
		return nil, closeSourceOnFailure(source, err)
	}
	if !sourceIsLive(source) {
		return nil, fmt.Errorf("opening attestation source returned no source")
	}
	data, _, err := source.GetManifest(ctx, nil)
	if err != nil {
		return nil, closeSourceOnFailure(source, err)
	}
	actual, err := manifest.Digest(data)
	if err != nil {
		return nil, closeSourceOnFailure(source, err)
	}
	if actual != ref.expected {
		return nil, closeSourceOnFailure(source, fmt.Errorf("source index changed before attestation fallback: inspected %s, opened %s", ref.expected, actual))
	}
	if err := ctx.Err(); err != nil {
		return nil, closeSourceOnFailure(source, err)
	}
	return source, nil
}

func closeSourceOnFailure(source types.ImageSource, err error) error {
	if !sourceIsLive(source) {
		return err
	}
	if closeErr := source.Close(); closeErr != nil {
		return noteCloseFailure(err, "closing failed attestation source", closeErr)
	}
	return err
}

func sourceIsLive(source types.ImageSource) bool {
	if source == nil {
		return false
	}
	value := reflect.ValueOf(source)
	return value.Kind() != reflect.Pointer || !value.IsNil()
}

// noteCloseFailure produces the same messages as mirror.NoteCloseFailure; this
// package cannot import internal/pkg/mirror because mirror depends on it. Both
// errors stay in the chain; the primary error is reported first.
func noteCloseFailure(err error, description string, closeErr error) error {
	if err == nil {
		return fmt.Errorf("%s: %w", description, closeErr)
	}
	return fmt.Errorf("%w (%s: %w)", err, description, closeErr)
}
