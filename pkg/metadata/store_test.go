package metadata

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

func TestUpdateMetadata_Catalogs(t *testing.T) {
	type spec struct {
		name     string
		config   v1alpha2.ImageSetConfiguration
		expIC    v1alpha2.IncludeConfig
		expError string
	}

	cases := []spec{
		{
			name: "Valid/HeadsOnlyFalse",
			config: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								Catalog: "test.registry/catalog@sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
								IncludeConfig: v1alpha2.IncludeConfig{
									Packages: []v1alpha2.IncludePackage{{Name: "foo"}},
								},
								Full: true,
							},
						},
					},
				},
			},
			expIC: v1alpha2.IncludeConfig{},
		},
		{
			name: "Valid/HeadsOnlyTrue",
			config: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								Catalog: "test.registry/catalog@sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
								Full:    false,
							},
						},
					},
				},
			},
			expIC: v1alpha2.IncludeConfig{
				Packages: []v1alpha2.IncludePackage{
					{
						Name: "bar",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "alpha",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "1.0.0",
								},
							},
						},
					},
					{
						Name: "baz",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "1.0.0",
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []v1alpha2.IncludeChannel{
							{
								Name: "beta",
								IncludeBundle: v1alpha2.IncludeBundle{
									MinVersion: "0.1.0",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inputMeta := v1alpha2.NewMetadata()
			inputMeta.PastMirror.Mirror = c.config.Mirror
			cfg := v1alpha2.StorageConfig{
				Local: &v1alpha2.LocalConfig{
					Path: t.TempDir(),
				},
			}
			backend, err := storage.ByConfig("", cfg)
			require.NoError(t, err)
			err = UpdateMetadata(context.TODO(), backend, &inputMeta, "testdata", true, true)
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expIC, inputMeta.PastMirror.Operators[len(inputMeta.PastMirror.Operators)-1].IncludeConfig)
			}
		})
	}
}

func TestUpdateMetadata_OCPReleases(t *testing.T) {

	type spec struct {
		name     string
		config   v1alpha2.ImageSetConfiguration
		expMeta  v1alpha2.PlatformMetadata
		expError string
	}

	cases := []spec{
		{
			name: "Valid/HeadsOnlyFalse",
			config: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.9",
									MinVersion: "4.9.0",
									MaxVersion: "4.9.5",
									Full:       true,
								},
							},
						},
					},
				},
			},
			expMeta: v1alpha2.PlatformMetadata{},
		},
		{
			name: "Valid/HeadsOnlyTrue",
			config: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name:       "stable-4.9",
									MinVersion: "4.9.5",
									MaxVersion: "4.9.5",
									Full:       false,
								},
							},
						},
					},
				},
			},
			expMeta: v1alpha2.PlatformMetadata{
				ReleaseChannel: "stable-4.9",
				MinVersion:     "4.9.5",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inputMeta := v1alpha2.NewMetadata()
			inputMeta.PastMirror.Mirror = c.config.Mirror
			cfg := v1alpha2.StorageConfig{
				Local: &v1alpha2.LocalConfig{
					Path: t.TempDir(),
				},
			}
			backend, err := storage.ByConfig("", cfg)
			require.NoError(t, err)
			err = UpdateMetadata(context.TODO(), backend, &inputMeta, "testdata", true, true)
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
				actual := v1alpha2.PlatformMetadata{}
				if len(inputMeta.PastMirror.Platforms) != 0 {
					actual = inputMeta.PastMirror.Platforms[len(inputMeta.PastMirror.Platforms)-1]
				}
				require.Equal(t, c.expMeta, actual)
			}
		})
	}
}
