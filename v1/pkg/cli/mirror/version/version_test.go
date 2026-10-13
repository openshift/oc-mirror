package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersionValidate(t *testing.T) {

	type spec struct {
		name     string
		opts     *VersionOptions
		expError string
	}

	cases := []spec{
		{
			name: "Invalid/InvalidOutput",
			opts: &VersionOptions{
				Output: "invalid",
			},
			expError: "--output must be 'yaml' or 'json'",
		},
		{
			name: "Valid/YAMLOutput",
			opts: &VersionOptions{
				Output: "yaml",
			},
			expError: "",
		},
		{
			name: "Valid/JSONOutput",
			opts: &VersionOptions{
				Output: "json",
			},
			expError: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.opts.Validate()
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
