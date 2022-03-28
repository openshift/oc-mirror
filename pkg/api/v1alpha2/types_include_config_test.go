package v1alpha2

import (
	"testing"

	"github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/stretchr/testify/require"
)

func TestConvertToDiffIncludeConfig(t *testing.T) {
	type spec struct {
		name string
		cfg  IncludeConfig
		exp  action.DiffIncludeConfig
	}

	specs := []spec{
		{
			name: "Valid/WithChannels",
			cfg: IncludeConfig{
				Packages: []IncludePackage{
					{
						Name: "bar",
						Channels: []IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: IncludeBundle{
									StartingVersion: semver.MustParse("0.1.0"),
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []IncludeChannel{
							{
								Name: "stable",
								IncludeBundle: IncludeBundle{
									StartingVersion: semver.MustParse("0.1.0"),
								},
							},
						},
					},
				},
			},
			exp: action.DiffIncludeConfig{
				Packages: []action.DiffIncludePackage{
					{
						Name: "bar",
						Channels: []action.DiffIncludeChannel{
							{
								Name: "stable",
								Versions: []semver.Version{
									semver.MustParse("0.1.0"),
								},
							},
						},
					},
					{
						Name: "foo",
						Channels: []action.DiffIncludeChannel{
							{
								Name: "stable",
								Versions: []semver.Version{
									semver.MustParse("0.1.0"),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Valid/NoChannels",
			cfg: IncludeConfig{
				Packages: []IncludePackage{
					{
						Name: "bar",
						IncludeBundle: IncludeBundle{
							StartingVersion: semver.MustParse("0.1.0"),
						},
					},
					{
						Name: "foo",
						IncludeBundle: IncludeBundle{
							StartingVersion: semver.MustParse("0.1.0"),
						},
					},
				},
			},
			exp: action.DiffIncludeConfig{
				Packages: []action.DiffIncludePackage{
					{
						Name: "bar",
						Versions: []semver.Version{
							semver.MustParse("0.1.0"),
						},
					},
					{
						Name: "foo",
						Versions: []semver.Version{
							semver.MustParse("0.1.0"),
						},
					},
				},
			},
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			dic, err := s.cfg.ConvertToDiffIncludeConfig()
			require.NoError(t, err)
			require.Equal(t, s.exp, dic)
		})
	}
}
