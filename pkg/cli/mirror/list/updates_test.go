package list

import (
	"testing"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
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
				FilterOptions: []string{v1alpha2.DefaultPlatformArchitecture},
				ConfigPath:    "foo",
			},
		},
		{
			name: "Valid/SuppliedArchitecture",
			opts: &UpdatesOptions{
				FilterOptions: []string{"test"},
			},
			args: []string{"foo"},
			expOpts: &UpdatesOptions{
				FilterOptions: []string{"test"},
				ConfigPath:    "foo",
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
			name: "Invalid/UnsupportedArch",
			opts: &UpdatesOptions{
				ConfigPath:    "foo",
				FilterOptions: []string{"fake"},
			},
			expError: "architecture \"fake\" is not a supported release architecture",
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
