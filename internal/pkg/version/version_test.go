package version

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

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

// TestSupportedOCPVersions guards against the SupportedOCPVersions list
// being accidentally emptied or populated with malformed entries (e.g. a
// typo like "v4.21" or "4.21.0" instead of "4.21").
func TestSupportedOCPVersions(t *testing.T) {
	info := Get()

	require.NotEmpty(t, info.SupportedOCPVersions, "SupportedOCPVersions must not be empty")

	versionPattern := regexp.MustCompile(`^\d+\.\d+$`)
	for _, v := range info.SupportedOCPVersions {
		require.Regexp(t, versionPattern, v, "supported OCP version %q does not look like a X.Y version", v)
	}
}

// TestSupportedOCPVersionsInOutput is a regression guard ensuring the
// supportedOCPVersions field is actually surfaced in both the json and
// yaml output of `oc-mirror version`, not just present on the Go struct.
func TestSupportedOCPVersionsInOutput(t *testing.T) {
	clientVersion := Get()
	versionInfo := Version{ClientVersion: &clientVersion}

	jsonBytes, err := json.Marshal(&versionInfo)
	require.NoError(t, err)
	require.Contains(t, string(jsonBytes), `"supportedOCPVersions"`)

	yamlBytes, err := yaml.Marshal(&versionInfo)
	require.NoError(t, err)
	require.Contains(t, string(yamlBytes), "supportedOCPVersions:")
}
