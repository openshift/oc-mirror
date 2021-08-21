package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

func Test_SplitArchive(t *testing.T) {

	testdir, err := os.MkdirTemp("", "test")

	defer os.RemoveAll(testdir)

	if err != nil {
		t.Fatal(err)
	}

	a := NewArchiver()

	tests := []struct {
		name         string
		source       string
		maxSplitSize int64
		files        []v1alpha1.File
		want         string
	}{
		{
			name:   "testing gz format",
			source: "../../test",
			files: []v1alpha1.File{
				{Name: "../../test"},
			},
			maxSplitSize: 1000000,
			want:         "testbundle",
		},
	}
	for _, tt := range tests {

		if err := CreateSplitArchive(a, tt.maxSplitSize, testdir, tt.source, tt.want, tt.files); err != nil {
			t.Errorf("Test %s: Failed to create archives for %s: %v", tt.name, tt.want, err)
		}

		err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {

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

func Test_CombineArchive(t *testing.T) {

	in := NewArchiver()
	out := NewArchiver()

	tests := []struct {
		name   string
		source string
		output string
	}{
		{
			name:   "testing gz format",
			source: "../../test/archiver/testdata/",
			output: "testbundle-combined.tar.gz",
		},
	}
	for _, tt := range tests {

		var bundleList []string

		err := filepath.Walk(tt.source, func(path string, info os.FileInfo, err error) error {

			if strings.Contains(info.Name(), "bar-bundle") {

				bundleList = append(bundleList, path)
			}

			return nil
		})

		if err != nil {
			t.Error(err)
		}

		if err = CombineArchives(in, out, ".", tt.output, bundleList...); err != nil {
			t.Errorf("Test %s: Failed to combine archives for %s: %v", tt.name, tt.output, err)
		}

		if err != nil {
			t.Fatal(err)
		}

		os.RemoveAll(tt.output)

	}
}
