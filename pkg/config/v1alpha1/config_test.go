package v1alpha1

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/blang/semver"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// TODO(estroz): expected config.
	type spec struct {
		name      string
		file      string
		inline    string
		assertion require.ErrorAssertionFunc
		expConfig ImageSetConfigurationSpec
		expError  string
	}

	falsePtr := new(bool)

	specs := []spec{
		{
			name:      "Valid/Basic",
			file:      filepath.Join("testdata", "config", "valid.yaml"),
			assertion: require.NoError,
			expConfig: ImageSetConfigurationSpec{
				Mirror: Mirror{
					OCP: OCP{
						Graph: true,
						Channels: []ReleaseChannel{
							{
								Name: "stable-4.7",
							},
							{
								Name:     "stable-4.6",
								Versions: []string{"4.6.36", "4.6.13"},
							},
						},
						PullSecret: "{\"auths\":{\"cloud.openshift.com\":...\"}}}",
					},
					Operators: []Operator{
						{
							Catalog:   "redhat-operators:v4.7",
							HeadsOnly: falsePtr,
						},
						{
							Catalog:   "certified-operators:v4.7",
							HeadsOnly: falsePtr,
							IncludeConfig: IncludeConfig{
								Packages: []IncludePackage{
									{Name: "couchbase-operator"},
									{
										Name: "mongodb-operator",
										IncludeBundle: IncludeBundle{
											StartingVersion: semver.Version{Major: 1, Minor: 4, Patch: 0},
										},
									},
									{
										Name: "crunchy-postgresql-operator",
										Channels: []IncludeChannel{
											{Name: "stable"},
										},
									},
								},
							},
							PullSecret: "{\"auths\":{\"cloud.openshift.com\":...\"}}}",
						},
						{
							Catalog: "community-operators:v4.7",
						},
					},
					AdditionalImages: []AdditionalImages{
						{Image: Image{Name: "registry.redhat.io/ubi8/ubi:latest"}},
					},
					Helm: Helm{
						Repos: []Repo{
							{
								URL:  "https://stefanprodan.github.io/podinfo",
								Name: "podinfo",
								Charts: []Chart{
									{Name: "podinfo", Version: "5.0.0"},
								},
							},
						},
						Local: []Chart{
							{Name: "podinfo", Path: "/test/podinfo-5.0.0.tar.gz"},
						},
					},
					BlockedImages: []BlockedImages{
						{Image: Image{Name: "alpine"}},
						{Image: Image{Name: "redis"}},
					},
					Samples: []SampleImages{
						{Image: Image{Name: "ruby"}},
						{Image: Image{Name: "python"}},
						{Image: Image{Name: "nginx"}},
					},
				},
			},
		},
		{
			name: "Invalid/UnknownKey",
			inline: `
apiVersion: tmp-redhatgov.com/v1alpha1
kind: ImageSetConfiguration
mirror:
  foo: bar
`,
			assertion: require.Error,
			expError:  `decode tmp-redhatgov.com/v1alpha1, Kind=ImageSetConfiguration: json: unknown field "foo"`,
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
apiVersion: tmp-redhatgov.com/v1alpha1
kind: ImageSetConfiguration
mirror:
  operators:
  - catalog: registry.com/ns/foo:v1.2
    headsOnly: false
  - catalog: registry.com/ns/bar:v1.2
    headsOnly: true
  - catalog: registry.com/ns/baz:v1.2
`

	cfg, err := LoadConfig([]byte(headsOnlyCfg))
	require.NoError(t, err)
	require.Len(t, cfg.Mirror.Operators, 3)
	require.Equal(t, cfg.Mirror.Operators[0].IsHeadsOnly(), false)
	require.Equal(t, cfg.Mirror.Operators[1].IsHeadsOnly(), true)
	require.Equal(t, cfg.Mirror.Operators[2].IsHeadsOnly(), true)
}
