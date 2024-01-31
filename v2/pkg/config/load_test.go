package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
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
					Platform: v1alpha2.Platform{
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
							{
								Name: "okd",
								Type: v1alpha2.TypeOKD,
							},
						},
					},
					Operators: []v1alpha2.Operator{
						{
							Catalog: "redhat-operators:v4.7",
							Full:    true,
						},
						{
							Catalog: "certified-operators:v4.7",
							Full:    true,
							IncludeConfig: v1alpha2.IncludeConfig{
								Packages: []v1alpha2.IncludePackage{
									{Name: "couchbase-operator"},
									{
										Name: "mongodb-operator",
										IncludeBundle: v1alpha2.IncludeBundle{
											MinVersion: "1.4.0",
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
					AdditionalImages: []v1alpha2.Image{
						{Name: "registry.redhat.io/ubi8/ubi:latest"},
					},
					Helm: v1alpha2.Helm{
						Repositories: []v1alpha2.Repository{
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
					BlockedImages: []v1alpha2.Image{
						{Name: "alpine"},
						{Name: "redis"},
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
				data, err = os.ReadFile(s.file)
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

	cfg, err := LoadConfig([]byte(headsOnlyCfg))
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
apiVersion: mirror.openshift.io/v1alpha2
kind: ImageSetConfiguration
mirror:
  operators:
  - catalog: registry.com/ns/foo:v1.2
    targetName: bar
  - catalog: registry.com/ns/bar:v1.2
    targetName: foo
    targetTag: v1.3
  - catalog: registry.com/ns/baz:v1.2
    targetTag: v1.3
  - catalog: registry.com/ns/baz:v1.2
`

	cfg, err := LoadConfig([]byte(ctlgCfg))
	require.NoError(t, err)
	require.Len(t, cfg.Mirror.Operators, 4)
	ctlgOne, err := cfg.Mirror.Operators[0].GetUniqueName()
	require.NoError(t, err)
	require.Equal(t, ctlgOne, "registry.com/ns/bar:v1.2")
	require.NotEqual(t, ctlgOne, cfg.Mirror.Operators[0].Catalog)
	ctlgTwo, err := cfg.Mirror.Operators[1].GetUniqueName()
	require.NoError(t, err)
	require.Equal(t, ctlgTwo, "registry.com/ns/foo:v1.3")
	require.NotEqual(t, ctlgTwo, cfg.Mirror.Operators[1].Catalog)
	ctlgThree, err := cfg.Mirror.Operators[2].GetUniqueName()
	require.NoError(t, err)
	require.Equal(t, ctlgThree, "registry.com/ns/baz:v1.3")
	require.NotEqual(t, ctlgThree, cfg.Mirror.Operators[2].Catalog)
	ctlgFour, err := cfg.Mirror.Operators[3].GetUniqueName()
	require.NoError(t, err)
	require.Equal(t, ctlgFour, "registry.com/ns/baz:v1.2")
	require.Equal(t, ctlgFour, cfg.Mirror.Operators[3].Catalog)
}

func TestLoadMetadata(t *testing.T) {
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
				data, err = os.ReadFile(s.file)
				require.NoError(t, err)
			}

			_, err := LoadMetadata(data)
			s.assertion(t, err)
		})
	}
}

func TestReadConfig(t *testing.T) {
	t.Run("Testing ReadConfig : should pass ", func(t *testing.T) {
		res, err := ReadConfig("../../tests/isc.yaml")
		if err != nil {
			t.Fatalf("should not fail")
		}
		require.Equal(t, []string{"amd64"}, res.ImageSetConfigurationSpec.Mirror.Platform.Architectures)
	})
}
