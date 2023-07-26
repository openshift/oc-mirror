package config

import (
	"testing"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/stretchr/testify/require"
)

func TestComplete(t *testing.T) {

	type spec struct {
		name      string
		config    v1alpha2.ImageSetConfiguration
		expConfig v1alpha2.ImageSetConfiguration
	}

	cases := []spec{
		{
			name: "Invalid/UnsupportedReleaseArchitecture",
			config: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Architectures: []string{},
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
			expConfig: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Architectures: []string{v1alpha2.DefaultPlatformArchitecture},
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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			Complete(&c.config)
			require.Equal(t, c.config, c.expConfig)
		})
	}
}
