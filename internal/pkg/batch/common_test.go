package batch

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

func TestAnnotateImageTimeoutError(t *testing.T) {
	t.Parallel()

	t.Run("deadline exceeded gets image-timeout hint", func(t *testing.T) {
		t.Parallel()
		got := annotateImageTimeoutError(context.DeadlineExceeded)
		assert.Contains(t, got, "context deadline exceeded")
		assert.Contains(t, got, "--image-timeout")
		assert.Contains(t, got, "10m0s")
	})

	t.Run("wrapped writing blob deadline exceeded gets hint", func(t *testing.T) {
		t.Parallel()
		err := fmt.Errorf("copying image 4/4 from manifest list: writing blob: Patch %q: %w",
			"https://registry:8443/v2/openshift/release/blobs/uploads/uuid",
			context.DeadlineExceeded)
		got := annotateImageTimeoutError(err)
		assert.Contains(t, got, "--image-timeout")
		assert.Contains(t, got, "30m")
	})

	t.Run("unrelated errors unchanged", func(t *testing.T) {
		t.Parallel()
		err := errors.New("unauthorized: authentication required")
		assert.Equal(t, err.Error(), annotateImageTimeoutError(err))
	})

	t.Run("already annotated errors are not doubled", func(t *testing.T) {
		t.Parallel()
		msg := "context deadline exceeded" + imageTimeoutHint
		got := annotateImageTimeoutError(errors.New(msg))
		assert.Equal(t, msg, got)
	})
}

func TestFormatErrorMsgImageTimeout(t *testing.T) {
	t.Parallel()
	msg := formatErrorMsg(mirrorErrorSchema{
		image: v2alpha1.CopyImageSchema{Origin: "quay.io/openshift-release-dev/ocp-v5.0-art-dev@sha256:abc"},
		err:   context.DeadlineExceeded,
	})
	require.Contains(t, msg, "error mirroring image")
	require.Contains(t, msg, "--image-timeout")
}
