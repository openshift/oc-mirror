package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func Test_MetadataError(t *testing.T) {

	path := "test"

	tests := []struct {
		name       string
		createType string
		helper     func(path string) error
		want       string
	}{
		{
			name:       "test diff error",
			createType: "diff",
			helper:     func(path string) error { return NewDiffMetadataError(path) },
			want:       fmt.Sprintf("no metadata found at %s, please run a create full", path),
		},
		{
			name:       "test full error",
			createType: "full",
			helper:     func(path string) error { return NewFullMetadataError(path) },
			want:       fmt.Sprintf("metadata found at %s, please run a create diff", path),
		},
	}

	for _, tt := range tests {

		err := &MetadataError{Path: path, Type: tt.createType}
		if !assert.Equal(t, tt.want, err.Error()) {
			t.Errorf("Test %s: Expected '%v', got '%v'", tt.name, tt.want, err)
		}

		err2 := tt.helper(path)

		if !assert.Equal(t, tt.want, err2.Error()) {
			t.Errorf("Test %s: Expected '%v', got '%v'", tt.name, tt.want, err)
		}

	}
}

func Test_WriteMetadata(t *testing.T) {

	metadata := v1alpha1.Metadata{}

	tmpdir, err := os.MkdirTemp("", "test")

	if err != nil {
		t.Error(err)
	}

	os.MkdirAll(filepath.Join(tmpdir, SourcePath, "publish"), 0755)

	if err := WriteMetadata(metadata, tmpdir); err != nil {
		t.Error(err)
	}

	os.RemoveAll(tmpdir)
}
