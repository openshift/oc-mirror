package mirror

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/openshift/oc-mirror/pkg/cli"
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
			},
		},
		{
			name: "Valid/RegDest",
			args: []string{"docker://reg.com"},
			opts: &MirrorOptions{},
			expOpts: &MirrorOptions{
				ToMirror: "reg.com",
			},
		},
		{
			name: "Valid/RegNamespace",
			args: []string{"docker://reg.com/foo/bar"},
			opts: &MirrorOptions{},
			expOpts: &MirrorOptions{
				ToMirror:      "reg.com",
				UserNamespace: "foo/bar",
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

func TestOperatorsValidate(t *testing.T) {

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
