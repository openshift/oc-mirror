package archive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
	"github.com/stretchr/testify/require"
)

/* FIXME(jpower432): known issue with many small files
the tar size will end up larger than specified by the
 user because of the tar header being written*/

func TestSplitArchive(t *testing.T) {

	testdir, err := os.MkdirTemp("", "test")
	require.NoError(t, err)
	defer os.RemoveAll(testdir)

	tests := []struct {
		name         string
		source       string
		maxSplitSize int64
		blobs        []string
		manifests    []string
		skipCleanup  bool
		want         string
	}{
		{
			name:         "testing tar format",
			blobs:        []string{"sha256:123456789"},
			manifests:    []string{"testmanifest"},
			maxSplitSize: 5 * 1024 * 1024,
			skipCleanup:  false,
			want:         "testbundle",
		},
		{
			name:         "testing cleanup",
			blobs:        []string{"sha256:123456789"},
			manifests:    []string{"testmanifest"},
			maxSplitSize: 5 * 1024 * 1024,
			skipCleanup:  true,
			want:         "testbundle",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packager := NewPackager(tt.manifests, tt.blobs)

			require.NoError(t, os.MkdirAll(filepath.Join(testdir, config.SourceDir), os.ModePerm))

			cwd, err := os.Getwd()
			require.NoError(t, err)

			// Change dir before archiving to avoid issues with symlink paths
			require.NoError(t, os.Chdir(filepath.Join(testdir, config.SourceDir)))
			require.NoError(t, writeFiles())

			backend, err := storage.NewLocalBackend(t.TempDir())
			require.NoError(t, err)

			meta := v1alpha2.Metadata{}
			require.NoError(t, backend.WriteMetadata(context.Background(), &meta, config.MetadataBasePath))

			require.NoError(t, packager.CreateSplitArchive(context.Background(), backend, tt.maxSplitSize, cwd, ".", tt.want, tt.skipCleanup))

			_, err = os.Stat(filepath.Join(cwd, "test1"))
			if !tt.skipCleanup {
				if err == nil {
					t.Error("File test1 was found, expected to be cleaned up")
				}
			} else {
				if err != nil {
					t.Error("File test1 was not found, expected to skip cleanup")
				}
			}

			err = filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {

				if strings.Contains(info.Name(), tt.want) {

					if info.Size() > tt.maxSplitSize {
						return fmt.Errorf("Test %s: Expected '%v' to be less than '%v'", tt.name, info.Size(), tt.maxSplitSize)
					}

					return os.RemoveAll(path)
				}

				return nil
			})
			require.NoError(t, err)
		})
	}
}

// writeFiles write out testfiles to be archived
func writeFiles() error {
	d1 := []byte("hello\ngo\n")

	for i := 0; i < 100; i++ {
		if err := os.WriteFile(fmt.Sprintf("test%d", i), d1, 0644); err != nil {
			return err
		}
	}

	return nil
}
