package archive

import (
	"archive/tar"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnarchiveRoutesByRootDirectory(t *testing.T) {
	testDir := t.TempDir()
	workingDir := filepath.Join(testDir, "destination", workingDirectory)
	cacheDir := filepath.Join(testDir, "cache")
	entries := []struct {
		name, parent string
	}{
		{"working-dir/hold-release/payload/release-manifests/image-references", filepath.Dir(workingDir)},
		{"working-dir/docker/registry/v2/metadata", filepath.Dir(workingDir)},
		{"docker/registry/v2/repositories/team/working-dir-release/_manifests/tags/latest/current/link", cacheDir},
		{"docker/registry/v2/repositories/team/working-dir/release/_manifests/tags/latest/current/link", cacheDir},
		{"docker/registry/v2/repositories/team/release/_manifests/tags/latest/current/link", cacheDir},
		{"other/working-dir/ignored", ""},
		{"working-directory/ignored", ""},
		{"other/docker/registry/v2/ignored", ""},
		{"docker/registry/v20/ignored", ""},
	}
	archivePath := filepath.Join(testDir, fmt.Sprintf(archiveFileNameFormat, archiveFilePrefix, 1))
	file, err := os.Create(archivePath)
	require.NoError(t, err)
	writer := tar.NewWriter(file)
	for _, entry := range entries {
		require.NoError(t, writer.WriteHeader(&tar.Header{
			Name: entry.name, Mode: 0600, Size: int64(len(entry.name)), Typeflag: tar.TypeReg,
		}))
		_, err := writer.Write([]byte(entry.name))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())

	extractor, err := NewArchiveExtractor(testDir, workingDir, cacheDir)
	require.NoError(t, err)
	require.NoError(t, extractor.Unarchive())
	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			if entry.parent != "" {
				content, err := os.ReadFile(filepath.Join(entry.parent, entry.name))
				require.NoError(t, err)
				require.Equal(t, entry.name, string(content))
			}
			for _, parent := range []string{filepath.Dir(workingDir), cacheDir} {
				if parent != entry.parent {
					require.NoFileExists(t, filepath.Join(parent, entry.name))
				}
			}
		})
	}
}
