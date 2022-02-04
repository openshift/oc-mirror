package mirror

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestMirrorComplete(t *testing.T) {
	type spec struct {
		name     string
		args     []string
		opts     *MirrorOptions
		expOpts  *MirrorOptions
		expError string
	}

	cases := []spec{
		{
			name: "Valid/FileDest",
			args: []string{"file://foo"},
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expOpts: &MirrorOptions{
				OutputDir: "foo",
				RootOptions: &cli.RootOptions{
					Dir: "foo/bar",
				},
				FilterOptions: []string{"amd64"},
			},
		},
		{
			name: "Valid/FileDestRel",
			args: []string{"file://./foo"},
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expOpts: &MirrorOptions{
				OutputDir: "foo",
				RootOptions: &cli.RootOptions{
					Dir: "foo/bar",
				},
				FilterOptions: []string{"amd64"},
			},
		},
		{
			name: "Valid/EmptyFileDest",
			args: []string{"file://"},
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expOpts: &MirrorOptions{
				OutputDir: ".",
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
				FilterOptions: []string{"amd64"},
			},
		},
		{
			name: "Valid/RegDest",
			args: []string{"docker://reg.com"},
			opts: &MirrorOptions{},
			expOpts: &MirrorOptions{
				ToMirror:      "reg.com",
				FilterOptions: []string{"amd64"},
			},
		},
		{
			name: "Valid/RegNamespace",
			args: []string{"docker://reg.com/foo/bar"},
			opts: &MirrorOptions{},
			expOpts: &MirrorOptions{
				ToMirror:      "reg.com",
				UserNamespace: "foo/bar",
				FilterOptions: []string{"amd64"},
			},
		},
		{
			name: "Valid/SetFilterOps",
			args: []string{"file://foo"},
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
				FilterOptions: []string{"amd64", "ppc64le"},
			},
			expOpts: &MirrorOptions{
				OutputDir: "foo",
				RootOptions: &cli.RootOptions{
					Dir: "foo/bar",
				},
				FilterOptions: []string{"amd64", "ppc64le"},
			},
		},
		{
			name:     "Invalid/TaggedReg",
			args:     []string{"docker://reg.com/foo/bar:latest"},
			opts:     &MirrorOptions{},
			expError: "destination registry must consist of registry host and namespace(s) only",
		},
		{
			name:     "Invalid/EmptyRegistry",
			args:     []string{"docker://"},
			opts:     &MirrorOptions{},
			expError: `"" is not a valid image reference: repository name must have at least one component`,
		},
		{
			name:     "Invalid/EmptyScheme",
			args:     []string{"://foo"},
			opts:     &MirrorOptions{},
			expError: `unknown destination scheme ""`,
		},
		{
			name:     "Invalid/NoSchemeDelim",
			args:     []string{"foo"},
			opts:     &MirrorOptions{},
			expError: "no scheme delimiter in destination argument",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.opts.Complete(&cobra.Command{}, c.args)
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expOpts, c.opts)
			}
		})
	}
}

func TestMirrorValidate(t *testing.T) {

	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Error(err)
	}

	type spec struct {
		name     string
		opts     *MirrorOptions
		expError string
	}

	cases := []spec{
		{
			name: "Invalid/NoDest",
			opts: &MirrorOptions{
				From: "dir",
			},
			expError: "must specify a registry destination",
		},
		{
			name: "Invalid/NoSource",
			opts: &MirrorOptions{
				ToMirror: u.Host,
			},
			expError: `must specify --config or --from with registry destination`,
		},
		{
			name: "Invalid/NoConfig",
			opts: &MirrorOptions{
				OutputDir: "dir",
			},
			expError: `must specify a configuration file with --config`,
		},
		{
			name: "Invalid/DryRunWithMirror",
			opts: &MirrorOptions{
				ConfigPath: "foo",
				ToMirror:   u.Host,
				DryRun:     true,
			},
			expError: "--dry-run is not supported for mirror publishing operations",
		},
		{
			name: "Invalid/UnsupportReleaseArch",
			opts: &MirrorOptions{
				ConfigPath:    "foo",
				ToMirror:      u.Host,
				FilterOptions: []string{"arm64"},
			},
			expError: "architecture \"arm64\" is not a supported release architecture",
		},
		{
			name: "Valid/MirrortoDisk",
			opts: &MirrorOptions{
				ConfigPath: "foo",
				ToMirror:   u.Host,
			},
			expError: "",
		},
		{
			name: "Valid/DisktoMirror",
			opts: &MirrorOptions{
				From:     t.TempDir(),
				ToMirror: u.Host,
			},
			expError: "",
		},
		{
			name: "Valid/MirrorToMirror",
			opts: &MirrorOptions{
				ConfigPath: "foo",
				ToMirror:   u.Host,
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
