package v2alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInstancePlatformFilter(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedOS   string
		expectedArch string
		errContains  string
	}{
		{
			name:         "valid linux/amd64",
			input:        "linux/amd64",
			expectedOS:   "linux",
			expectedArch: "amd64",
		},
		{
			name:         "valid linux/arm64",
			input:        "linux/arm64",
			expectedOS:   "linux",
			expectedArch: "arm64",
		},
		{
			name:         "trims whitespace",
			input:        "  linux/s390x  ",
			expectedOS:   "linux",
			expectedArch: "s390x",
		},
		{
			name:        "missing slash",
			input:       "linuxamd64",
			errContains: "expected format is os/architecture",
		},
		{
			name:        "empty OS",
			input:       "/amd64",
			errContains: "OS must not be empty",
		},
		{
			name:        "empty Architecture",
			input:       "linux/",
			errContains: "architecture must not be empty",
		},
		{
			name:        "empty string",
			input:       "",
			errContains: "expected format is os/architecture",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParseInstancePlatformFilter(tt.input)
			if tt.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedOS, p.OS)
			assert.Equal(t, tt.expectedArch, p.Architecture)
		})
	}
}
