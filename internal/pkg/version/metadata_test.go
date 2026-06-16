package version

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/config"
)

func TestWriteAndReadVersionMetadata(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	info := Info{
		GitVersion: "v2.1.0",
		GitCommit:  "abc123",
	}

	require.NoError(t, WriteVersionMetadata(workingDir, info))

	path := filepath.Join(workingDir, config.VersionMetadataPath)
	meta, err := ReadVersionMetadata(path)
	require.NoError(t, err)
	require.Equal(t, info.GitVersion, meta.GitVersion)
	require.Equal(t, info.GitCommit, meta.GitCommit)
}

func TestCheckVersionMetadata(t *testing.T) {
	t.Parallel()

	current := Info{
		GitVersion: "v2.1.0",
		GitCommit:  "current-commit",
	}

	t.Run("missing file warns", func(t *testing.T) {
		t.Parallel()

		warning := CheckVersionMetadata(t.TempDir(), current)
		require.NotEmpty(t, warning)
	})

	t.Run("matching version and commit does not warn", func(t *testing.T) {
		t.Parallel()

		workingDir := t.TempDir()
		require.NoError(t, WriteVersionMetadata(workingDir, current))

		warning := CheckVersionMetadata(workingDir, current)
		require.Empty(t, warning)
	})

	t.Run("version mismatch warns", func(t *testing.T) {
		t.Parallel()

		workingDir := t.TempDir()
		require.NoError(t, WriteVersionMetadata(workingDir, Info{
			GitVersion: "v2.0.0",
			GitCommit:  "current-commit",
		}))

		warning := CheckVersionMetadata(workingDir, current)
		require.Contains(t, warning, "version mismatch")
		require.Contains(t, warning, "v2.0.0")
		require.Contains(t, warning, "v2.1.0")
	})

	t.Run("commit mismatch warns", func(t *testing.T) {
		t.Parallel()

		workingDir := t.TempDir()
		require.NoError(t, WriteVersionMetadata(workingDir, Info{
			GitVersion: "v2.1.0",
			GitCommit:  "other-commit",
		}))

		warning := CheckVersionMetadata(workingDir, current)
		require.Contains(t, warning, "version mismatch")
		require.Contains(t, warning, "other-commit")
		require.Contains(t, warning, "current-commit")
	})

	t.Run("corrupt json warns", func(t *testing.T) {
		t.Parallel()

		workingDir := t.TempDir()
		internalDir := filepath.Join(workingDir, config.InternalDir)
		require.NoError(t, os.MkdirAll(internalDir, 0o755))
		path := filepath.Join(workingDir, config.VersionMetadataPath)
		require.NoError(t, os.WriteFile(path, []byte("{invalid"), 0o600))

		warning := CheckVersionMetadata(workingDir, current)
		require.NotEmpty(t, warning)
	})
}

func TestReadVersionMetadataErrors(t *testing.T) {
	t.Parallel()

	_, err := ReadVersionMetadata(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
}
