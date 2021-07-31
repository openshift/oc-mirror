package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mholt/archiver/v3"
)

func Test_SplitArchive(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		maxSplitSize int64
		want         string
	}{
		{
			name:         "testing gz format",
			source:       "../../testdata/archives",
			maxSplitSize: 1000000,
			want:         "testbundle",
		},
	}
	for _, tt := range tests {

		a := NewArchiver()

		if err := CreateSplitArchive(a, ".", tt.want, tt.maxSplitSize, tt.source); err != nil {
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
	tests := []struct {
		name   string
		source string
		output string
		want   string
	}{
		{
			name:   "testing gz format",
			source: "../../testdata/archives",
			output: "testbundle-combined.tar.gz",
			want:   "testbundle-combined",
		},
	}
	for _, tt := range tests {

		a := NewArchiver()

		var bundleList []string

		err := filepath.Walk(tt.source, func(path string, info os.FileInfo, err error) error {

			if strings.Contains(info.Name(), tt.want) {

				bundleList = append(bundleList, path)
				fmt.Print(path)
			}

			return nil
		})

		if err != nil {
			t.Error(err)
		}

		if err = CombineArchives(tt.want, bundleList...); err != nil {
			t.Errorf("Test %s: Failed to combine archives for %s: %v", tt.name, tt.want, err)
		}

		defer os.Remove(tt.output)

		err = a.Walk(tt.output, func(f archiver.File) error {
			fmt.Println("Filename:", f.Name())
			return nil
		})

		if err != nil {
			t.Fatal(err)
		}

	}
}
