package batch

import (
	"errors"
	"fmt"
	"testing"

	distributionErrcode "github.com/docker/distribution/registry/api/errcode"
	"github.com/stretchr/testify/assert"
	"go.podman.io/image/v5/docker"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

func TestAnnotateUnauthorizedUploadError(t *testing.T) {
	t.Parallel()

	blobUnauthorized := errors.New("unable to upload blob sha256:deadbeef to registry.example.com/ns/repo: unauthorized: access to the requested resource is not authorized")
	annotated := annotateUnauthorizedUploadError(blobUnauthorized)
	assert.Contains(t, annotated, blobUnauthorized.Error())
	assert.Contains(t, annotated, "push credentials/permissions")
	assert.Contains(t, annotated, "--max-nested-paths")

	writingBlob := errors.New("error writing blob sha256:deadbeef: unauthorized: access to the requested resource is not authorized")
	assert.Contains(t, annotateUnauthorizedUploadError(writingBlob), "--max-nested-paths")

	plainUnauthorized := errors.New("unauthorized: unauthorized")
	assert.Equal(t, plainUnauthorized.Error(), annotateUnauthorizedUploadError(plainUnauthorized))

	other := errors.New("manifest unknown")
	assert.Equal(t, other.Error(), annotateUnauthorizedUploadError(other))

	assert.Equal(t, "", annotateUnauthorizedUploadError(nil))
}

func TestAnnotateUnauthorizedUploadErrorTyped(t *testing.T) {
	t.Parallel()

	typedUnauthorizedBlob := fmt.Errorf("unable to upload blob sha256:deadbeef to registry.example.com/ns/repo: %w",
		distributionErrcode.Error{Code: distributionErrcode.ErrorCodeUnauthorized, Message: "access to the requested resource is not authorized"})
	annotated := annotateUnauthorizedUploadError(typedUnauthorizedBlob)
	assert.Contains(t, annotated, "unable to upload blob")
	assert.Contains(t, annotated, "push credentials/permissions")
	assert.Contains(t, annotated, "--max-nested-paths")

	typedDeniedBlob := fmt.Errorf("error uploading layer: %w",
		distributionErrcode.Error{Code: distributionErrcode.ErrorCodeDenied, Message: "requested access to the resource is denied"})
	annotatedDenied := annotateUnauthorizedUploadError(typedDeniedBlob)
	assert.Contains(t, annotatedDenied, "error uploading layer")
	assert.Contains(t, annotatedDenied, "push credentials/permissions")

	typedUnauthorizedOnly := distributionErrcode.Error{Code: distributionErrcode.ErrorCodeUnauthorized, Message: "unauthorized"}
	assert.Equal(t, typedUnauthorizedOnly.Error(), annotateUnauthorizedUploadError(typedUnauthorizedOnly))

	// Typed non-auth registry errors must not use the unauthorized text fallback.
	typedUnsupported := fmt.Errorf("unable to upload blob sha256:deadbeef: unauthorized: %w",
		distributionErrcode.Error{Code: distributionErrcode.ErrorCodeUnsupported, Message: "The operation is unsupported."})
	assert.Equal(t, typedUnsupported.Error(), annotateUnauthorizedUploadError(typedUnsupported))

	credentialErr := docker.ErrUnauthorizedForCredentials{Err: errors.New("invalid username/password")}
	assert.Equal(t, credentialErr.Error(), annotateUnauthorizedUploadError(credentialErr))

	credentialBlob := fmt.Errorf("unable to upload blob sha256:deadbeef: %w", credentialErr)
	annotatedCred := annotateUnauthorizedUploadError(credentialBlob)
	assert.Contains(t, annotatedCred, "unable to upload blob")
	assert.Contains(t, annotatedCred, "push credentials/permissions")
}

func TestFormatErrorMsgUnauthorizedBlobUpload(t *testing.T) {
	t.Parallel()

	err := mirrorErrorSchema{
		image: v2alpha1.CopyImageSchema{
			Origin: "docker://registry.redhat.io/example/image:latest",
		},
		err: errors.New("unable to upload blob sha256:abc to dest.example.com/a/b/c: unauthorized: access to the requested resource is not authorized"),
	}

	msg := formatErrorMsg(err)
	assert.Contains(t, msg, "error mirroring image docker://registry.redhat.io/example/image:latest error:")
	assert.Contains(t, msg, "unable to upload blob")
	assert.Contains(t, msg, "push credentials/permissions")
	assert.Contains(t, msg, "--max-nested-paths")
}
