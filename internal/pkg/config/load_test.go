package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	type spec struct {
		name      string
		file      string
		inline    string
		assertion require.ErrorAssertionFunc
		expConfig v2alpha1.ImageSetConfigurationSpec
		expError  string
	}

	specs := []spec{
		{
			name:      "Valid/Basic",
			file:      filepath.Join(consts.TestFolder, "config", "valid.yaml"),
			assertion: require.NoError,
			expConfig: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform: v2alpha1.Platform{
						Graph: true,
						Channels: []v2alpha1.ReleaseChannel{
							{
								Name: "stable-4.7",
							},
							{
								Name:       "stable-4.6",
								MinVersion: "4.6.3",
								MaxVersion: "4.6.13",
							},
							{
								Name: "okd",
								Type: v2alpha1.TypeOKD,
							},
						},
					},
					Operators: []v2alpha1.Operator{
						{
							Catalog: "redhat-operators:v4.7",
							Full:    true,
						},
						{
							Catalog: "certified-operators:v4.7",
							Full:    true,
							IncludeConfig: v2alpha1.IncludeConfig{
								Packages: []v2alpha1.IncludePackage{
									{Name: "couchbase-operator"},
									{
										Name: "mongodb-operator",
										IncludeBundle: v2alpha1.IncludeBundle{
											MinVersion: "1.4.0",
										},
									},
									{
										Name: "crunchy-postgresql-operator",
										Channels: []v2alpha1.IncludeChannel{
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
					AdditionalImages: []v2alpha1.AdditionalImage{
						{Name: "registry.redhat.io/ubi8/ubi:latest"},
					},
					Helm: v2alpha1.Helm{
						Repositories: []v2alpha1.Repository{
							{
								URL:  "https://stefanprodan.github.io/podinfo",
								Name: "podinfo",
								Charts: []v2alpha1.Chart{
									{Name: "podinfo", Version: "5.0.0"},
								},
							},
						},
						Local: []v2alpha1.Chart{
							{Name: "podinfo", Path: "/test/podinfo-5.0.0.tar.gz"},
						},
					},
					BlockedImages: []v2alpha1.BlockedImage{
						{Name: "alpine"},
						{Name: "redis"},
					},
					Samples: []v2alpha1.SampleImage{
						{Name: "ruby"},
						{Name: "python"},
						{Name: "nginx"},
					},
				},
			},
		},
		{
			name: "Invalid/UnknownKey",
			inline: `
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  foo: bar
`,
			assertion: require.Error,
			expError:  `decode ImageSetConfiguration: json: unknown field "foo"`,
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			data := []byte(s.inline)
			if len(data) == 0 {
				var err error
				data, err = os.ReadFile(s.file)
				require.NoError(t, err)
			}

			cfg, err := LoadConfig[v2alpha1.ImageSetConfiguration](data, v2alpha1.ImageSetConfigurationKind)
			s.assertion(t, err)
			if err != nil {
				require.EqualError(t, err, s.expError)
			} else {
				require.Equal(t, s.expConfig, cfg.ImageSetConfigurationSpec)
			}
		})
	}
}

// TestLoadConfigDelete
func TestLoadConfigDelete(t *testing.T) {
	type spec struct {
		name      string
		file      string
		inline    string
		assertion require.ErrorAssertionFunc
		expConfig v2alpha1.DeleteImageSetConfigurationSpec
		expError  string
	}

	deletespecs := []spec{
		{
			name:      "Delete-Valid/Basic",
			file:      filepath.Join(consts.TestFolder, "config", "valid-delete.yaml"),
			assertion: require.NoError,
			expConfig: v2alpha1.DeleteImageSetConfigurationSpec{
				Delete: v2alpha1.Delete{
					Platform: v2alpha1.Platform{
						Graph: true,
						Channels: []v2alpha1.ReleaseChannel{
							{
								Name: "stable-4.7",
							},
							{
								Name:       "stable-4.6",
								MinVersion: "4.6.3",
								MaxVersion: "4.6.13",
							},
							{
								Name: "okd",
								Type: v2alpha1.TypeOKD,
							},
						},
					},
					Operators: []v2alpha1.Operator{
						{
							Catalog: "redhat-operators:v4.7",
							Full:    true,
						},
						{
							Catalog: "certified-operators:v4.7",
							Full:    true,
							IncludeConfig: v2alpha1.IncludeConfig{
								Packages: []v2alpha1.IncludePackage{
									{Name: "couchbase-operator"},
									{
										Name: "mongodb-operator",
										IncludeBundle: v2alpha1.IncludeBundle{
											MinVersion: "1.4.0",
										},
									},
									{
										Name: "crunchy-postgresql-operator",
										Channels: []v2alpha1.IncludeChannel{
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
					AdditionalImages: []v2alpha1.AdditionalImage{
						{Name: "registry.redhat.io/ubi8/ubi:latest"},
					},
					Helm: v2alpha1.Helm{
						Repositories: []v2alpha1.Repository{
							{
								URL:  "https://stefanprodan.github.io/podinfo",
								Name: "podinfo",
								Charts: []v2alpha1.Chart{
									{Name: "podinfo", Version: "5.0.0"},
								},
							},
						},
						Local: []v2alpha1.Chart{
							{Name: "podinfo", Path: "/test/podinfo-5.0.0.tar.gz"},
						},
					},
					Samples: []v2alpha1.SampleImage{
						{Name: "ruby"},
						{Name: "python"},
						{Name: "nginx"},
					},
				},
			},
		},
		{
			name: "Invalid/UnknownKey",
			inline: `
apiVersion: mirror.openshift.io/v2alpha1
kind: DeleteImageSetConfiguration
delete:
  foo: bar
`,
			assertion: require.Error,
			expError:  `decode DeleteImageSetConfiguration: json: unknown field "foo"`,
		},
	}

	for _, s := range deletespecs {
		t.Run(s.name, func(t *testing.T) {
			data := []byte(s.inline)
			if len(data) == 0 {
				var err error
				data, err = os.ReadFile(s.file)
				require.NoError(t, err)
			}
			cfg, err := LoadConfig[v2alpha1.DeleteImageSetConfiguration](data, v2alpha1.DeleteImageSetConfigurationKind)
			s.assertion(t, err)
			if err != nil {
				require.EqualError(t, err, s.expError)
			} else {
				require.Equal(t, s.expConfig, cfg.DeleteImageSetConfigurationSpec)
			}
		})
	}
}

func TestHeadsOnly(t *testing.T) {
	headsOnlyCfg := `
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  platform:
    channels:
    - name: test-channel1
      full: true
    - name: test-channel2
      full: false
    - name: test-channel3
  operators:
  - catalog: registry.com/ns/foo:v1.2
    full: true
  - catalog: registry.com/ns/bar:v1.2
    full: false
  - catalog: registry.com/ns/baz:v1.2
`

	cfg, err := LoadConfig[v2alpha1.ImageSetConfiguration]([]byte(headsOnlyCfg), v2alpha1.ImageSetConfigurationKind)
	require.NoError(t, err)
	require.Len(t, cfg.Mirror.Platform.Channels, 3)
	require.Len(t, cfg.Mirror.Operators, 3)
	require.Equal(t, cfg.Mirror.Platform.Channels[0].IsHeadsOnly(), false)
	require.Equal(t, cfg.Mirror.Platform.Channels[0].IsHeadsOnly(), false)
	require.Equal(t, cfg.Mirror.Platform.Channels[0].IsHeadsOnly(), false)
	require.Equal(t, cfg.Mirror.Operators[0].IsHeadsOnly(), false)
	require.Equal(t, cfg.Mirror.Operators[1].IsHeadsOnly(), true)
	require.Equal(t, cfg.Mirror.Operators[2].IsHeadsOnly(), true)
}

func TestGetUniqueName(t *testing.T) {
	ctlgCfg := `
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  operators:
  - catalog: registry.com/ns/foo:v1.2
    targetCatalog: ns/bar
  - catalog: registry.com/ns/bar:v1.2
    targetCatalog: ns/foo
    targetTag: v1.3
  - catalog: registry.com/ns/baz:v1.2
    targetTag: v1.3
  - catalog: registry.com/ns/baz:v1.2
`

	cfg, err := LoadConfig[v2alpha1.ImageSetConfiguration]([]byte(ctlgCfg), v2alpha1.ImageSetConfigurationKind)
	require.NoError(t, err)
	require.Len(t, cfg.Mirror.Operators, 4)
	ctlgOne, err := cfg.Mirror.Operators[0].GetUniqueName()
	require.NoError(t, err)
	require.Equal(t, "registry.com/ns/bar:v1.2", ctlgOne)
	require.NotEqual(t, ctlgOne, cfg.Mirror.Operators[0].Catalog)
	ctlgTwo, err := cfg.Mirror.Operators[1].GetUniqueName()
	require.NoError(t, err)
	require.Equal(t, "registry.com/ns/foo:v1.3", ctlgTwo)
	require.NotEqual(t, cfg.Mirror.Operators[1].Catalog, ctlgTwo)
	ctlgThree, err := cfg.Mirror.Operators[2].GetUniqueName()
	require.NoError(t, err)
	require.Equal(t, ctlgThree, "registry.com/ns/baz:v1.3")
	require.NotEqual(t, ctlgThree, cfg.Mirror.Operators[2].Catalog)
	ctlgFour, err := cfg.Mirror.Operators[3].GetUniqueName()
	require.NoError(t, err)
	require.Equal(t, ctlgFour, "registry.com/ns/baz:v1.2")
	require.Equal(t, ctlgFour, cfg.Mirror.Operators[3].Catalog)
}

func TestReadConfig(t *testing.T) {
	testReadConfig(t, v2alpha1.ImageSetConfigurationKind)
}

func TestReadConfigDelete(t *testing.T) {
	testReadConfig(t, v2alpha1.DeleteImageSetConfigurationKind)
}

func testReadConfig(t *testing.T, kind string) {
	t.Helper()
	stanza, otherStanza := "mirror", "delete"
	file, otherFile := "isc.yaml", "delete-isc.yaml"
	otherKind := v2alpha1.DeleteImageSetConfigurationKind
	if kind == v2alpha1.DeleteImageSetConfigurationKind {
		stanza, otherStanza = otherStanza, stanza
		file, otherFile = otherFile, file
		otherKind = v2alpha1.ImageSetConfigurationKind
	}
	type testCase struct {
		name, file, inline, expectedError string
		errorFields                       []string
		platform                          v2alpha1.Platform
	}
	header := fmt.Sprintf("apiVersion: mirror.openshift.io/v2alpha1\nkind: %s\n", kind)
	tests := []testCase{
		{
			name: "valid", file: file,
			//nolint:staticcheck // SA1019: the fixture still uses the deprecated architectures field
			platform: v2alpha1.Platform{Architectures: []string{"amd64"}},
		},
		{
			name: "unmatched config kinds", file: otherFile,
			expectedError: fmt.Sprintf("cannot parse %q as %q", otherKind, kind),
		},
		{
			name: "missing stanza", inline: header + "unknown:\n  additionalImages: []\n",
			expectedError: fmt.Sprintf("configuration missing the `%s` stanza", stanza),
		},
		{
			name: "extra stanza", inline: header + stanza + ": {}\n" + otherStanza + ": {}\n",
			expectedError: fmt.Sprintf("%s: is not allowed in %s", otherStanza, kind),
		},
		{
			name:          "duplicated stanza",
			inline:        "\n" + header + stanza + ":\n  additionalImages:\n    - name: mirror.delete/mirror/delete:mirror\n" + stanza + ":\n  operators:\n    - catalog: catalog:latest\n",
			expectedError: fmt.Sprintf("failed to unmarshal config: error converting YAML to JSON: yaml: unmarshal errors:\n  line 8: key %q already set in map", stanza),
		},
		{
			name: "unknown platform field", inline: header + stanza + ":\n  platform:\n    targetRepo: releases\n",
			expectedError: fmt.Sprintf("decode %s: json: unknown field %q", kind, "targetRepo"),
		},
		{name: "default repositories"},
		{
			name:     "payload override",
			platform: v2alpha1.Platform{TargetImageRepo: "openshift4/ocp-release"},
		},
		{
			name:     "component override",
			platform: v2alpha1.Platform{TargetComponentRepo: "openshift4/components"},
		},
		{
			name:     "separate overrides",
			platform: v2alpha1.Platform{TargetImageRepo: "openshift4/ocp-release", TargetComponentRepo: "openshift4/components"},
		},
		{
			name: "invalid payload", errorFields: []string{"targetImageRepo"},
			platform: v2alpha1.Platform{TargetImageRepo: "repo:tag"},
		},
		{
			name: "invalid component", errorFields: []string{"targetComponentRepo"},
			platform: v2alpha1.Platform{TargetComponentRepo: "repo:tag"},
		},
		{
			name: "both invalid", errorFields: []string{"targetImageRepo", "targetComponentRepo"},
			platform: v2alpha1.Platform{TargetImageRepo: "repo:tag", TargetComponentRepo: "UPPER"},
		},
		{
			name: "valid payload and invalid component", errorFields: []string{"targetComponentRepo"},
			platform: v2alpha1.Platform{TargetImageRepo: "openshift4/ocp-release", TargetComponentRepo: "repo:tag"},
		},
		{
			name: "invalid payload and valid component", errorFields: []string{"targetImageRepo"},
			platform: v2alpha1.Platform{TargetImageRepo: "repo:tag", TargetComponentRepo: "openshift4/components"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(consts.TestFolder, tt.file)
			if tt.file == "" {
				data := tt.inline
				if data == "" {
					data = header + fmt.Sprintf("%s:\n  platform:\n    targetImageRepo: %q\n    targetComponentRepo: %q\n", stanza, tt.platform.TargetImageRepo, tt.platform.TargetComponentRepo)
				}
				configPath = filepath.Join(t.TempDir(), "config.yaml")
				require.NoError(t, os.WriteFile(configPath, []byte(data), 0600))
			}
			cfg, err := ReadConfig(configPath, kind)
			if tt.expectedError != "" {
				require.EqualError(t, err, tt.expectedError)
				return
			}
			if len(tt.errorFields) > 0 {
				for _, field := range tt.errorFields {
					require.ErrorContains(t, err, "platform."+field)
				}
				return
			}
			require.NoError(t, err)
			var platform v2alpha1.Platform
			if kind == v2alpha1.ImageSetConfigurationKind {
				platform = cfg.(v2alpha1.ImageSetConfiguration).Mirror.Platform
			} else {
				platform = cfg.(v2alpha1.DeleteImageSetConfiguration).Delete.Platform
			}
			require.Equal(t, tt.platform.TargetImageRepo, platform.TargetImageRepo)
			require.Equal(t, tt.platform.TargetComponentRepo, platform.TargetComponentRepo)
			//nolint:staticcheck // SA1019: retain legacy architecture coverage.
			require.Equal(t, tt.platform.Architectures, platform.Architectures)
		})
	}
}
