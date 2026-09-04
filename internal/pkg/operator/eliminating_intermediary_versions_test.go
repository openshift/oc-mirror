package operator

import (
	"testing"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

func TestEliminatingIntermediaryVersions(t *testing.T) {
	log := clog.New("trace")

	tests := []struct {
		name   string
		dc     *declcfg.DeclarativeConfig
		filter v2alpha1.Operator
		want   *declcfg.DeclarativeConfig
	}{
		{
			name: "Jordan's example 1",
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{{Name: "foo"}},
				Channels: []declcfg.Channel{{Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
					{
						Name:     "foo.v1.3.0",
						Replaces: "foo.v1.2.0",
						Skips: []string{
							"foo.v1.2.0",
							"foo.v1.1.0",
							"foo.v1.0.0",
						},
					},
					{
						Name:     "foo.v1.2.0",
						Replaces: "foo.v1.1.0",
						Skips: []string{
							"foo.v1.1.0",
							"foo.v1.0.0",
						},
					},
					{
						Name:     "foo.v1.1.0",
						Replaces: "foo.v1.0.0",
						Skips: []string{
							"foo.v1.0.0",
						},
					},
					{
						Name: "foo.v1.0.0",
					},
				}}},
			},
			filter: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{{Name: "foo", IncludeBundle: v2alpha1.IncludeBundle{MaxVersion: "1.0.0"}}},
				},
			},
			want: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{{Name: "foo"}},
				Channels: []declcfg.Channel{{Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
					{
						Name:     "foo.v1.3.0",
						Replaces: "foo.v1.0.0",
						Skips: []string{
							"foo.v1.2.0",
							"foo.v1.1.0",
							"foo.v1.0.0",
						},
					},
					{
						Name: "foo.v1.0.0",
					},
				}}}},
		},
		{
			name: "Jordan's example 2",
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{{Name: "foo"}},
				Channels: []declcfg.Channel{{Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
					{
						Name:      "foo.v1.3.0",
						Replaces:  "foo.v1.2.0",
						SkipRange: ">=1.0.0 <1.3.0",
					},
					{
						Name:      "foo.v1.2.0",
						Replaces:  "foo.v1.1.0",
						SkipRange: ">=1.0.0 <1.2.0",
					},
					{
						Name:      "foo.v1.1.0",
						Replaces:  "foo.v1.0.0",
						SkipRange: ">=1.0.0 <1.1.0",
					},
					{
						Name: "foo.v1.0.0",
					},
				}}},
			},
			filter: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{{Name: "foo", IncludeBundle: v2alpha1.IncludeBundle{MaxVersion: "1.0.0"}}},
				},
			},
			want: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{{Name: "foo"}},
				Channels: []declcfg.Channel{{Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
					{
						Name:      "foo.v1.3.0",
						Replaces:  "foo.v1.0.0",
						SkipRange: ">=1.0.0 <1.3.0",
					},
					{
						Name: "foo.v1.0.0",
					},
				}}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eliminatingIntermediaryVersions(tt.dc, tt.filter, log)
			assert.Equal(t, tt.want, got)
		})
	}
}
