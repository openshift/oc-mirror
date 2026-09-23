package batch

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

func TestAnnotateUnauthorizedUploadError(t *testing.T) {
	t.Parallel()

	blobUnauthorized := errors.New("unable to upload blob sha256:deadbeef to registry.example.com/ns/repo: unauthorized: access to the requested resource is not authorized")
	annotated := annotateUnauthorizedUploadError(blobUnauthorized)
	assert.Contains(t, annotated, blobUnauthorized.Error())
	assert.Contains(t, annotated, "push credentials/permissions")
	assert.Contains(t, annotated, "--max-nested-paths")
	assert.Contains(t, annotated, "separate remediations")

	plainUnauthorized := errors.New("unauthorized: unauthorized")
	assert.Equal(t, plainUnauthorized.Error(), annotateUnauthorizedUploadError(plainUnauthorized))

	other := errors.New("manifest unknown")
	assert.Equal(t, other.Error(), annotateUnauthorizedUploadError(other))

	assert.Equal(t, "", annotateUnauthorizedUploadError(nil))
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
