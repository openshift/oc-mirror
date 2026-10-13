package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeArchivePath(t *testing.T) {
	cases := []struct {
		name        string
		basedir     string
		filepath    string
		expected    string
		expectedErr bool
	}{
		{
			name:     "absolute path",
			basedir:  "/workdir",
			filepath: "path/to/file",
			expected: "/workdir/path/to/file",
		},
		{
			name:     "relative to current path",
			basedir:  "./workdir",
			filepath: "path/to/file",
			expected: "workdir/path/to/file",
		},
		{
			name:     "current dir as '.'",
			basedir:  ".",
			filepath: "path/to/file",
			expected: "path/to/file",
		},
		{
			name:     "filepath starts with '.'",
			basedir:  ".",
			filepath: "./path/to/file",
			expected: "path/to/file",
		},
		{
			name:     "hidden file in '.'",
			basedir:  ".",
			filepath: ".hidden",
			expected: ".hidden",
		},
		{
			name:     "non-tainted '..'",
			basedir:  "/workdir",
			filepath: "../workdir/path/to/file",
			expected: "/workdir/path/to/file",
		},
		{
			name:        "tainted '..'",
			basedir:     ".",
			filepath:    "../../../../../../../../../../../../etc/shadow",
			expectedErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := SanitizeArchivePath(tc.basedir, tc.filepath)
			if tc.expectedErr {
				assert.ErrorContains(t, err, "content filepath is tainted")
			} else if assert.NoError(t, err, res) {
				assert.Equal(t, tc.expected, res)
			}
		})
	}
}
