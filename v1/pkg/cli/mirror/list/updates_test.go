package list

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdatesComplete(t *testing.T) {
	type spec struct {
		name     string
		opts     *UpdatesOptions
		args     []string
		expOpts  *UpdatesOptions
		expError string
	}

	cases := []spec{
		{
			name: "Valid/DefaultArchitecture",
			opts: &UpdatesOptions{},
			args: []string{"foo"},
			expOpts: &UpdatesOptions{
				ConfigPath: "foo",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.opts.Complete(c.args)
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expOpts, c.opts)
			}
		})
	}
}

func TestUpdatesValidate(t *testing.T) {

	type spec struct {
		name     string
		opts     *UpdatesOptions
		expError string
	}

	cases := []spec{
		{
			name:     "Invalid/NoConfigPath",
			opts:     &UpdatesOptions{},
			expError: `must specify imageset configuration`,
		},
		{
			name: "Valid/WithConfig",
			opts: &UpdatesOptions{
				ConfigPath: "foo",
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
