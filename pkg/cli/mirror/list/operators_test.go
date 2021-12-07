package list

import (
	"testing"

	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/stretchr/testify/require"
)

func TestOperatorsComplete(t *testing.T) {
	type spec struct {
		name     string
		opts     *OperatorsOptions
		expOpts  *OperatorsOptions
		expError string
	}

	cases := []spec{
		{
			name: "Valid/VersionSet",
			opts: &OperatorsOptions{
				Version: "2.4.0",
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expOpts: &OperatorsOptions{
				Version:  "2.4.0",
				Catalogs: true,
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

func TestOperatorsValidate(t *testing.T) {

	type spec struct {
		name     string
		opts     *OperatorsOptions
		expError string
	}

	cases := []spec{
		{
			name: "Invalid/NoVersion",
			opts: &OperatorsOptions{
				Catalogs: true,
			},
			expError: "must specify --version with --catalogs",
		},
		{
			name: "Invalid/NoCatalog",
			opts: &OperatorsOptions{
				Package: "foo",
			},
			expError: `must specify --catalog with --package`,
		},
		{
			name: "Invalid/NoPackage",
			opts: &OperatorsOptions{
				Channel: "foo-channel",
			},
			expError: `must specify --catalog and --package with --channel`,
		},
		{
			name: "Invalid/NoCatalogwithChannel",
			opts: &OperatorsOptions{
				Channel: "foo-channel",
				Package: "foo",
			},
			expError: `must specify --catalog and --package with --channel`,
		},
		{
			name: "Valid/Catalogs",
			opts: &OperatorsOptions{
				Catalogs: true,
				Version:  "4.8",
			},
			expError: "",
		},
		{
			name: "Valid/Packages",
			opts: &OperatorsOptions{
				Catalog: "foo-catalog",
				Package: "foo",
			},
			expError: "",
		},
		{
			name: "Valid/Channels",
			opts: &OperatorsOptions{
				Catalog: "foo-catalog",
				Package: "foo",
				Channel: "foo-channel",
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
