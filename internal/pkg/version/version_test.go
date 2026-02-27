package version

import (
	"testing"

	"github.com/stretchr/testify/require"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

func TestNewVersionCommand(t *testing.T) {
	log := clog.New("info")
	cmd := NewVersionCommand(log)
	require.NotNil(t, cmd)
}

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

func TestVersionRun(t *testing.T) {

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
			expError: "VersionOptions were not validated: --output=\"invalid\" should have been rejected",
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
			err := c.opts.Run()
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
