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

		a, err := NewArchiver()

		if err != nil {
			t.Errorf("Test %s: cannot create archiver for file %s.tar.gz", tt.name, tt.want)
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
			}

			return nil
		})

		if err != nil {
			t.Error(err)
		}
	}
}
