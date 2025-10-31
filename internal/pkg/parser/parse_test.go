package parser

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseJsonFromFile(t *testing.T) {
	type custom struct{ field string } //nolint:unused // just a dummy type
	// We don't really need to test this functionality.
	// These tests exist just to sanity check the error messages.
	t.Run("Testing ParseJsonFile should fail", func(t *testing.T) {
		t.Run("with invalid file", func(t *testing.T) {
			res, err := ParseJsonFile[custom]("invalid-file")
			assert.Empty(t, res)
			assert.Error(t, err)
			assert.Regexp(t, "read file: open .*: no such file or directory", err)
		})

		t.Run("with malformed json", func(t *testing.T) {
			filePath := path.Join(t.TempDir(), "index.json")
			err := os.WriteFile(filePath, []byte("invalid"), 0o644) //nolint:gosec // temp file
			assert.NoError(t, err, "should create invalid index.json")

			res, err := ParseJsonFile[custom](filePath)
			assert.Empty(t, res)
			assert.Error(t, err)
			assert.Regexp(t, "parse file: .*", err)
		})
	})
}

func TestParseYamlFromFile(t *testing.T) {
	type custom struct{ field string } //nolint:unused // just a dummy type
	// We don't really need to test this functionality.
	// These tests exist just to sanity check the error messages.
	t.Run("Testing ParseYamlFile should fail", func(t *testing.T) {
		t.Run("with invalid file", func(t *testing.T) {
			res, err := ParseYamlFile[custom]("invalid-file")
			assert.Empty(t, res)
			assert.Error(t, err)
			assert.Regexp(t, "read file: open .*: no such file or directory", err)
		})

		t.Run("with malformed yaml", func(t *testing.T) {
			filePath := path.Join(t.TempDir(), "index.yaml")
			err := os.WriteFile(filePath, []byte("invalid\nyaml"), 0o644) //nolint:gosec // temp file
			assert.NoError(t, err, "should create invalid index.yaml")

			res, err := ParseYamlFile[custom](filePath)
			assert.Empty(t, res)
			assert.Error(t, err)
			assert.Regexp(t, "parse file: .*", err)
		})
	})
}
