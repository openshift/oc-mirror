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
	})

	t.Run("written under the internal directory", func(t *testing.T) {
		workingDir := t.TempDir()
		require.NoError(t, WriteSkippedImages(workingDir, images))

		_, err := os.Stat(filepath.Join(workingDir, config.SkippedImagesPath))
		assert.NoError(t, err, "the file must live in the working dir, which is what gets archived")
	})

	t.Run("a missing file is not an error", func(t *testing.T) {
		got, err := ReadSkippedImages(t.TempDir())
		assert.NoError(t, err, "archives from older releases record nothing")
		assert.Empty(t, got)
	})

	t.Run("nothing skipped round trips as empty", func(t *testing.T) {
		workingDir := t.TempDir()
		require.NoError(t, WriteSkippedImages(workingDir, nil))

		got, err := ReadSkippedImages(workingDir)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("the image type survives the round trip", func(t *testing.T) {
		workingDir := t.TempDir()
		require.NoError(t, WriteSkippedImages(workingDir, images))

		raw, err := os.ReadFile(filepath.Join(workingDir, config.SkippedImagesPath))
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"type": "operatorOperand"`,
			"the persisted record should be readable by a human debugging a d2m exclusion")

		got, err := ReadSkippedImages(workingDir)
		require.NoError(t, err)
		assert.Equal(t, v2alpha1.TypeOperatorOperand, got[0].Type)
	})

	t.Run("writing an empty list clears an earlier record", func(t *testing.T) {
		workingDir := t.TempDir()
		require.NoError(t, WriteSkippedImages(workingDir, images))
		require.NoError(t, WriteSkippedImages(workingDir, nil))

		got, err := ReadSkippedImages(workingDir)
		require.NoError(t, err)
		assert.Empty(t, got, "a stale record must not survive a run that skipped nothing")
	})

	t.Run("a corrupt file is reported", func(t *testing.T) {
		workingDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(workingDir, config.InternalDir), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(workingDir, config.SkippedImagesPath), []byte("not json"), 0o600))

		_, err := ReadSkippedImages(workingDir)
		assert.Error(t, err)
	})
}

func TestOrigins(t *testing.T) {
	origins := Origins([]v2alpha1.SkippedImage{
		{Origin: "docker://a"},
		{Origin: "docker://b"},
	})

	assert.Len(t, origins, 2)
	assert.Contains(t, origins, "docker://a")
	assert.Contains(t, origins, "docker://b")
	assert.NotContains(t, origins, "docker://c")
}
