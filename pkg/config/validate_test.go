package config

import (
	"testing"

	"github.com/openshift/oc-mirror/pkg/config/v1alpha2"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {

	trueValue := true
	falseValue := false

	type spec struct {
		name     string
		config   *v1alpha2.ImageSetConfiguration
		expError string
	}

	cases := []spec{
		{
			name: "Valid/HeadsOnlyFalse",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								IncludeConfig: v1alpha2.IncludeConfig{
									Packages: []v1alpha2.IncludePackage{{Name: "foo"}},
								},
								HeadsOnly: &falseValue,
							},
						},
					},
				},
			},
			expError: "",
		},
		{
			name: "Valid/NoIncludePackages",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								IncludeConfig: v1alpha2.IncludeConfig{},
								HeadsOnly:     &trueValue,
							},
						},
					},
				},
			},
		},
		{
			name: "Valid/HeadsOnlyFalse",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								IncludeConfig: v1alpha2.IncludeConfig{
									Packages: []v1alpha2.IncludePackage{{Name: "foo"}},
								},
								HeadsOnly: &falseValue,
							},
						},
					},
				},
			},
		},
		{
			name: "Valid/UniqueReleaseChannels",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						OCP: v1alpha2.OCP{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name: "channel1",
								},
								{
									Name: "channel2",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Invalid/HeadsOnlyTrue",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								IncludeConfig: v1alpha2.IncludeConfig{
									Packages: []v1alpha2.IncludePackage{{Name: "foo"}},
								},
								HeadsOnly: &trueValue,
							},
						},
					},
				},
			},
			expError: "invalid configuration option: catalog cannot define packages with headsOnly set to true",
		},
		{
			name: "Invalid/DuplicateChannels",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						OCP: v1alpha2.OCP{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name: "channel",
								},
								{
									Name: "channel",
								},
							},
						},
					},
				},
			},
			expError: "invalid configuration option: duplicate release channel channel found in configuration",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.config)
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
