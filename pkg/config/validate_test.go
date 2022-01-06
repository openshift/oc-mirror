package config

import (
	"testing"

	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {

	trueValue := true
	falseValue := false

	type spec struct {
		name     string
		config   v1alpha1.ImageSetConfiguration
		expError string
	}

	cases := []spec{
		{
			name: "Valid/HeadsOnlyFalse",
			config: v1alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha1.ImageSetConfigurationSpec{
					Mirror: v1alpha1.Mirror{
						Operators: []v1alpha1.Operator{
							{
								IncludeConfig: v1alpha1.IncludeConfig{
									Packages: []v1alpha1.IncludePackage{{Name: "foo"}},
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
			config: v1alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha1.ImageSetConfigurationSpec{
					Mirror: v1alpha1.Mirror{
						Operators: []v1alpha1.Operator{
							{
								IncludeConfig: v1alpha1.IncludeConfig{},
								HeadsOnly:     &trueValue,
							},
						},
					},
				},
			},
		},
		{
			name: "Valid/HeadsOnlyFalse",
			config: v1alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha1.ImageSetConfigurationSpec{
					Mirror: v1alpha1.Mirror{
						Operators: []v1alpha1.Operator{
							{
								IncludeConfig: v1alpha1.IncludeConfig{
									Packages: []v1alpha1.IncludePackage{{Name: "foo"}},
								},
								HeadsOnly: &falseValue,
							},
						},
					},
				},
			},
		},
		{
			name: "Invalid/HeadsOnlyTrue",
			config: v1alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha1.ImageSetConfigurationSpec{
					Mirror: v1alpha1.Mirror{
						Operators: []v1alpha1.Operator{
							{
								IncludeConfig: v1alpha1.IncludeConfig{
									Packages: []v1alpha1.IncludePackage{{Name: "foo"}},
								},
								HeadsOnly: &trueValue,
							},
						},
					},
				},
			},
			expError: "invalid configuration option: catalog cannot define packages with headsOnly set to true",
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
