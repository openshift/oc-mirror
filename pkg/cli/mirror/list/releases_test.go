package list

import (
	"testing"

	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/stretchr/testify/require"
)

func TestReleasesComplete(t *testing.T) {
	type spec struct {
		name     string
		opts     *ReleasesOptions
		expOpts  *ReleasesOptions
		expError string
	}

	cases := []spec{
		{
			name: "Valid/ChannelEmpty",
			opts: &ReleasesOptions{
				Channel: "",
				Version: "4.8",
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expOpts: &ReleasesOptions{
				Channel: "stable-4.8",
				Version: "4.8",
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.opts.Complete()
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expOpts, c.opts)
			}
		})
	}
}

func TestReleasesValidate(t *testing.T) {

	type spec struct {
		name     string
		opts     *ReleasesOptions
		expError string
	}

	cases := []spec{
		{
			name: "Invalid/NoVersion",
			opts: &ReleasesOptions{
				Channel: "stable-",
			},
			expError: "must specify --version or --channel",
		},
		{
			name: "Invalid/NoCatalog",
			opts: &ReleasesOptions{
				Channels: true,
			},
			expError: `must specify --version`,
		},
		{
			name: "Valid/Channels",
			opts: &ReleasesOptions{
				Channels: true,
				Version:  "4.8",
			},
			expError: "",
		},
		{
			name: "Valid/Versions",
			opts: &ReleasesOptions{
				Channel: "stable-foo",
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
