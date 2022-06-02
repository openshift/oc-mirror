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
			name: "Valid/ChannelEmpty/Version X.Y",
			opts: &ReleasesOptions{
				Channel: "",
				Version: "4.8",
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expOpts: &ReleasesOptions{
				Channel:       "stable-4.8",
				Version:       "4.8",
				FilterByArchs: []string{"amd64"},
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
		},
		{
			name: "Valid/ChannelEmpty/Version X.Y.Z",
			opts: &ReleasesOptions{
				Channel: "",
				Version: "4.9.10",
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expOpts: &ReleasesOptions{
				Channel:       "stable-4.9",
				Version:       "4.9.10",
				FilterByArchs: []string{"amd64"},
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
		},
		{
			name: "Valid/ChannelEmpty/Version invalid string",
			opts: &ReleasesOptions{
				Channel: "",
				Version: "bad",
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expOpts: &ReleasesOptions{
				Channel: "",
				Version: "bad",
				RootOptions: &cli.RootOptions{
					Dir: "bar",
				},
			},
			expError: "Unable parse major.minor version from: bad",
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
			name: "Invalid/NoVersionsWithChannels",
			opts: &ReleasesOptions{
				Channels: true,
			},
			expError: `must specify --version`,
		},
		{
			name: "Invalid/UnsupportedArch",
			opts: &ReleasesOptions{
				FilterByArchs: []string{"fake"},
			},
			expError: "architecture \"fake\" is not a supported release architecture",
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

func TestParseVersionTags(t *testing.T) {
	tags := []string{
		"4.1.10-something",
		"5.0.1",
		"4.11.2-other",
		"4.10.0-arch",
		"v4.0",
		"4.2",
		"sometag",
	}
	expected := []releaseVersion{
		{major: 4, minor: 1},
		{major: 4, minor: 2},
		{major: 4, minor: 10},
		{major: 4, minor: 11},
		{major: 5, minor: 0},
	}
	verlist := parseVersionTags(tags)
	for i, v := range verlist {
		require.Equal(t, expected[i], v)
	}

	// verify that both lists are equal length
	require.Equal(t, len(expected), len(verlist))

}
