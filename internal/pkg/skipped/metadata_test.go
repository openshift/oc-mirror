package skipped

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/config"
)

func TestWriteReadSkippedImages(t *testing.T) {
	images := []v2alpha1.SkippedImage{
		{
			Origin:    "docker://registry.redhat.io/ns/virt-launcher-4-nv-rhel10@sha256:aaa",
			Type:      v2alpha1.TypeOperatorOperand,
			Reason:    "no instance for the requested platform(s)",
			Platforms: []string{"linux/amd64"},
		},
		{
			Origin: "docker://registry.redhat.io/ns/other@sha256:bbb",
			Reason: "no instance for the requested platform(s)",
		},
	}

	t.Run("round trip", func(t *testing.T) {
		workingDir := t.TempDir()
		require.NoError(t, WriteSkippedImages(workingDir, images))

		got, err := ReadSkippedImages(workingDir)
		require.NoError(t, err)
		assert.Equal(t, images, got)

		raw, err := os.ReadFile(filepath.Join(workingDir, config.SkippedImagesPath))
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"type": "operatorOperand"`)
	})

	t.Run("a missing file is not an error", func(t *testing.T) {
		got, err := ReadSkippedImages(t.TempDir())
		assert.NoError(t, err, "archives from older releases record nothing")
		assert.Empty(t, got)
	})

	t.Run("writing an empty list clears an earlier record", func(t *testing.T) {
		workingDir := t.TempDir()
		require.NoError(t, WriteSkippedImages(workingDir, images))
		require.NoError(t, WriteSkippedImages(workingDir, nil))

		got, err := ReadSkippedImages(workingDir)
		require.NoError(t, err)
		assert.Empty(t, got, "a stale record must not survive a run that skipped nothing")
	})

}
