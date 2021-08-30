package archive

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RedHatGov/bundle/pkg/bundle"
	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

/* FIXME(jpower): known issue with many small files
the tar size will end up larger than specified by the
 user because of the tar header being written*/

func Test_SplitArchive(t *testing.T) {

	testdir, err := os.MkdirTemp("", "test")

	defer os.RemoveAll(testdir)

	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		source       string
		maxSplitSize int64
		blobs        []v1alpha1.Blob
		manifests    []v1alpha1.Manifest
		want         string
	}{
		{
			name:         "testing tar format",
			blobs:        []v1alpha1.Blob{{Name: "sha256:123456789"}},
			manifests:    []v1alpha1.Manifest{{Name: "testmanifest"}},
			maxSplitSize: 5 * 1024 * 1024,
			want:         "testbundle",
		},
	}
	for _, tt := range tests {

		packager := NewPackager(tt.manifests, tt.blobs)

		if err := bundle.MakeCreateDirs(testdir); err != nil {
			t.Fatal(err)
		}

		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}

		// Change dir before archiving to avoid issues with symlink paths
		if err := os.Chdir(filepath.Join(testdir, config.SourceDir)); err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(cwd)

		if err := writeFiles(); err != nil {
			t.Fatal(err)
		}

		if err := packager.CreateSplitArchive(tt.maxSplitSize, cwd, ".", tt.want); err != nil {
			t.Errorf("Test %s: Failed to create archives for %s: %v", tt.name, tt.want, err)
		}

		err = filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {

			if strings.Contains(info.Name(), tt.want) {

				if info.Size() > tt.maxSplitSize {
					return fmt.Errorf("Test %s: Expected '%v' to be less than '%v'", tt.name, info.Size(), tt.maxSplitSize)
				}

				os.RemoveAll(path)
			}

			return nil
		})

		if err != nil {
			t.Error(err)
		}
	}
}

// writeFiles write out testfiles to be archived
func writeFiles() error {
	d1 := []byte("hello\ngo\n")

	for i := 0; i < 100; i++ {
		if err := ioutil.WriteFile(fmt.Sprintf("test%d", i), d1, 0644); err != nil {
			return err
		}
	}

	return nil
}
