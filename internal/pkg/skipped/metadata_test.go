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

func TestPartiallyMirroredOperands(t *testing.T) {
	images := map[string][]v2alpha1.InstancePlatformFilter{
		"fully-skipped":      nil,
		"partially-mirrored": {{OS: "linux", Architecture: "amd64"}},
	}
	dir := t.TempDir()
	require.NoError(t, WriteOperandPlatforms(dir, images))
	got, err := ReadOperandPlatforms(dir)
	require.NoError(t, err)
	assert.Equal(t, images, got)
	assert.Contains(t, got, "fully-skipped")
	assert.Empty(t, got["fully-skipped"])
	assert.NotEmpty(t, got["partially-mirrored"], "partial copies must retain their available platforms")
}

func TestWriteReadOperandPlatforms(t *testing.T) {
	images := map[string][]v2alpha1.InstancePlatformFilter{
		"docker://registry.redhat.io/ns/virt-launcher-4-nv-rhel10@sha256:aaa": nil,
		"docker://registry.redhat.io/ns/other@sha256:bbb":                     nil,
	}

	t.Run("round trip", func(t *testing.T) {
		workingDir := t.TempDir()
		require.NoError(t, WriteOperandPlatforms(workingDir, images))

		got, err := ReadOperandPlatforms(workingDir)
		require.NoError(t, err)
		assert.Equal(t, images, got)

		raw, err := os.ReadFile(filepath.Join(workingDir, config.SkippedImagesPath))
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"docker://registry.redhat.io/ns/other@sha256:bbb":null`)
	})

	t.Run("a missing file is not an error", func(t *testing.T) {
		got, err := ReadOperandPlatforms(t.TempDir())
		assert.NoError(t, err, "archives from older releases record nothing")
		assert.Empty(t, got)
	})

	t.Run("writing an empty list clears an earlier record", func(t *testing.T) {
		workingDir := t.TempDir()
		require.NoError(t, WriteOperandPlatforms(workingDir, images))
		require.NoError(t, WriteOperandPlatforms(workingDir, nil))

		got, err := ReadOperandPlatforms(workingDir)
		require.NoError(t, err)
		assert.Empty(t, got, "a stale record must not survive a run that skipped nothing")
	})

}
