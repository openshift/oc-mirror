package describe

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDescribeComplete(t *testing.T) {
	type spec struct {
		name     string
		opts     *DescribeOptions
		args     []string
		expOpts  *DescribeOptions
		expError string
	}

	cases := []spec{
		{
			name: "Valid/DefaultArchitecture",
			opts: &DescribeOptions{},
			args: []string{"foo"},
			expOpts: &DescribeOptions{
				From: "foo",
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

func TestDescribeValidate(t *testing.T) {

	type spec struct {
		name     string
		opts     *DescribeOptions
		expError string
	}

	cases := []spec{
		{
			name:     "Invalid/NoConfigPath",
			opts:     &DescribeOptions{},
			expError: `must specify path to imageset archive`,
		},
		{
			name: "Valid/WithArchivePath",
			opts: &DescribeOptions{
				From: "foo",
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
