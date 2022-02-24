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
			name: "Valid/MinandMaxReleaseVersionSet",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						OCP: v1alpha2.OCP{
							Channels: []v1alpha2.ReleaseChannel{
								{
									MinVersion: "1.2.3",
									MaxVersion: "1.2.3",
								},
							},
						},
					},
				},
			},
			expError: "",
		},
		{
			name: "Valid/MinandMaxReleaseVersionNotSet",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						OCP: v1alpha2.OCP{
							Channels: []v1alpha2.ReleaseChannel{
								{},
							},
						},
					},
				},
			},
			expError: "",
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
			name: "Invalid/NoMinimumReleaseVersion",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						OCP: v1alpha2.OCP{
							Channels: []v1alpha2.ReleaseChannel{
								{
									MaxVersion: "1.2.3",
								},
							},
						},
					},
				},
			},
			expError: "invalid configuration option: release channel must have a minimum version specified",
		},
		{
			name: "Invalid/NoMaximunReleaseVersion",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						OCP: v1alpha2.OCP{
							Channels: []v1alpha2.ReleaseChannel{
								{
									MinVersion: "1.2.3",
								},
							},
						},
					},
				},
			},
			expError: "invalid configuration option: release channel must have a maximum version specified",
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
