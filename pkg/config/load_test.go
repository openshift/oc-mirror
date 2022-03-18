package config

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// TODO(estroz): expected config.
	type spec struct {
		name      string
		file      string
		inline    string
		assertion require.ErrorAssertionFunc
		expConfig v1alpha2.ImageSetConfigurationSpec
		expError  string
	}

	specs := []spec{
		{
			name:      "Valid/Basic",
			file:      filepath.Join("testdata", "config", "valid.yaml"),
			assertion: require.NoError,
			expConfig: v1alpha2.ImageSetConfigurationSpec{
				Mirror: v1alpha2.Mirror{
					OCP: v1alpha2.OCP{
						Graph: true,
						Channels: []v1alpha2.ReleaseChannel{
							{
								Name: "stable-4.7",
							},
							{
								Name:       "stable-4.6",
								MinVersion: "4.6.3",
								MaxVersion: "4.6.13",
							},
						},
					},
					Operators: []v1alpha2.Operator{
						{
							Catalog:     "redhat-operators:v4.7",
							AllPackages: true,
						},
						{
							Catalog:     "certified-operators:v4.7",
							AllPackages: true,
							IncludeConfig: v1alpha2.IncludeConfig{
								Packages: []v1alpha2.IncludePackage{
									{Name: "couchbase-operator"},
									{
										Name: "mongodb-operator",
										IncludeBundle: v1alpha2.IncludeBundle{
											StartingVersion: semver.Version{Major: 1, Minor: 4, Patch: 0},
										},
									},
									{
										Name: "crunchy-postgresql-operator",
										Channels: []v1alpha2.IncludeChannel{
											{Name: "stable"},
										},
									},
								},
							},
						},
						{
							Catalog: "community-operators:v4.7",
						},
					},
					AdditionalImages: []v1alpha2.AdditionalImages{
						{Image: v1alpha2.Image{Name: "registry.redhat.io/ubi8/ubi:latest"}},
					},
					Helm: v1alpha2.Helm{
						Repos: []v1alpha2.Repo{
							{
								URL:  "https://stefanprodan.github.io/podinfo",
								Name: "podinfo",
								Charts: []v1alpha2.Chart{
									{Name: "podinfo", Version: "5.0.0"},
								},
							},
						},
						Local: []v1alpha2.Chart{
							{Name: "podinfo", Path: "/test/podinfo-5.0.0.tar.gz"},
						},
					},
					BlockedImages: []v1alpha2.BlockedImages{
						{Image: v1alpha2.Image{Name: "alpine"}},
						{Image: v1alpha2.Image{Name: "redis"}},
					},
					Samples: []v1alpha2.SampleImages{
						{Image: v1alpha2.Image{Name: "ruby"}},
						{Image: v1alpha2.Image{Name: "python"}},
						{Image: v1alpha2.Image{Name: "nginx"}},
					},
				},
			},
		},
		{
			name: "Invalid/UnknownKey",
			inline: `
apiVersion: mirror.openshift.io/v1alpha2
kind: ImageSetConfiguration
mirror:
  foo: bar
`,
			assertion: require.Error,
			expError:  `decode mirror.openshift.io/v1alpha2, Kind=ImageSetConfiguration: json: unknown field "foo"`,
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			data := []byte(s.inline)
			if len(data) == 0 {
				var err error
				data, err = ioutil.ReadFile(s.file)
				require.NoError(t, err)
			}

			cfg, err := LoadConfig(data)
			s.assertion(t, err)
			if err != nil {
				require.EqualError(t, err, s.expError)
			} else {
				require.Equal(t, s.expConfig, cfg.ImageSetConfigurationSpec)
			}
		})
	}
}

func TestHeadsOnly(t *testing.T) {

	headsOnlyCfg := `
apiVersion: mirror.openshift.io/v1alpha2
kind: ImageSetConfiguration
mirror:
  ocp:
    channels:
    - name: test-channel1
      allVersions: true
    - name: test-channel2
      allVersions: false
    - name: test-channel3
  operators:
  - catalog: registry.com/ns/foo:v1.2
    allPackages: true
  - catalog: registry.com/ns/bar:v1.2
    allPackages: false
  - catalog: registry.com/ns/baz:v1.2
`

	cfg, err := LoadConfig([]byte(headsOnlyCfg))
	require.NoError(t, err)
	require.Len(t, cfg.Mirror.OCP.Channels, 3)
	require.Len(t, cfg.Mirror.Operators, 3)
	require.Equal(t, cfg.Mirror.OCP.Channels[0].IsHeadsOnly(), false)
	require.Equal(t, cfg.Mirror.OCP.Channels[0].IsHeadsOnly(), false)
	require.Equal(t, cfg.Mirror.OCP.Channels[0].IsHeadsOnly(), false)
	require.Equal(t, cfg.Mirror.Operators[0].IsHeadsOnly(), false)
	require.Equal(t, cfg.Mirror.Operators[1].IsHeadsOnly(), true)
	require.Equal(t, cfg.Mirror.Operators[2].IsHeadsOnly(), true)
}

func TestLoadMetadata(t *testing.T) {
	// TODO(estroz): expected metadata.
	type spec struct {
		name      string
		file      string
		inline    string
		assertion require.ErrorAssertionFunc
	}

	specs := []spec{
		{
			name:      "Valid/Basic",
			file:      filepath.Join("testdata", "metadata", "valid.json"),
			assertion: require.NoError,
		},
		{
			name: "Invalid/BadStructure",
			inline: `---
apiVersion: mirror.openshift.io/v1alpha2
kind: ImageSetConfiguration
foo: bar
`,
			assertion: require.Error,
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			data := []byte(s.inline)
			if len(data) == 0 {
				var err error
				data, err = ioutil.ReadFile(s.file)
				require.NoError(t, err)
			}

			_, err := LoadMetadata(data)
			s.assertion(t, err)
		})
	}
}
