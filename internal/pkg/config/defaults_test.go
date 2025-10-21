package config

import (
	"testing"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/stretchr/testify/require"
)

func TestComplete(t *testing.T) {

	type spec struct {
		name      string
		config    v2alpha1.ImageSetConfiguration
		expConfig v2alpha1.ImageSetConfiguration
	}

	cases := []spec{
		{
			name: "Invalid/UnsupportedReleaseArchitecture",
			config: v2alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
					Mirror: v2alpha1.Mirror{
						Platform: v2alpha1.Platform{
							Architectures: []string{},
							Channels: []v2alpha1.ReleaseChannel{
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
			expConfig: v2alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
					Mirror: v2alpha1.Mirror{
						Platform: v2alpha1.Platform{
							Architectures: []string{v2alpha1.DefaultPlatformArchitecture},
							Channels: []v2alpha1.ReleaseChannel{
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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			Complete(&c.config)
			require.Equal(t, c.config, c.expConfig)
		})
	}
}

func TestDeleteComplete(t *testing.T) {

	type spec struct {
		name      string
		config    v2alpha1.DeleteImageSetConfiguration
		expConfig v2alpha1.DeleteImageSetConfiguration
	}

	cases := []spec{
		{
			name: "Invalid/UnsupportedReleaseArchitecture",
			config: v2alpha1.DeleteImageSetConfiguration{
				DeleteImageSetConfigurationSpec: v2alpha1.DeleteImageSetConfigurationSpec{
					Delete: v2alpha1.Delete{
						Platform: v2alpha1.Platform{
							Architectures: []string{},
							Channels: []v2alpha1.ReleaseChannel{
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
			expConfig: v2alpha1.DeleteImageSetConfiguration{
				DeleteImageSetConfigurationSpec: v2alpha1.DeleteImageSetConfigurationSpec{
					Delete: v2alpha1.Delete{
						Platform: v2alpha1.Platform{
							Architectures: []string{v2alpha1.DefaultPlatformArchitecture},
							Channels: []v2alpha1.ReleaseChannel{
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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			CompleteDelete(&c.config)
			require.Equal(t, c.config, c.expConfig)
		})
	}
}
