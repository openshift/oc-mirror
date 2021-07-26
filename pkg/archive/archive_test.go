package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test__Split_Archive(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		maxSplitSize int64
		ext          string
		want         string
	}{
		{
			name:         "testing gz format",
			source:       "../../testdata/archives",
			maxSplitSize: 1000000,
			ext:          ".tar.gz",
			want:         "testbundle",
		},
		{
			name:         "testing tar format",
			source:       "../../testdata/archives",
			maxSplitSize: 1000000,
			ext:          ".tar",
			want:         "testbundle",
		},
	}
	for _, tt := range tests {

		a, err := NewArchiver(tt.ext)

		if err != nil {
			t.Errorf("Test %s: cannot create archiver for file %s%s", tt.name, tt.want, tt.ext)
		}

		if err = CreateSplitArchive(a, ".", tt.want, tt.maxSplitSize, tt.source); err != nil {
			t.Errorf("Test %s: Failed to create archives for %s: %v", tt.name, tt.want, err)
		}

		err = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {

			if strings.Contains(info.Name(), tt.want) {

				if info.Size() > tt.maxSplitSize {
					return fmt.Errorf("Test %s: Expected '%v' to be less than '%v'", tt.name, info.Size(), tt.maxSplitSize)
				}

				os.RemoveAll(path)
				os.Remove("sha256sum.txt")
			}

			return nil
		})

		if err != nil {
			t.Error(err)
		}
	}
}
