package operator

import (
	"testing"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

// TestVersionFromEntryName covers both bundle-name forms: the "<package>.v<semver>"
// form (e.g. amq-broker) and the "<package>.<semver>" form with no "v" prefix
// (e.g. rhods-operator). The latter used to fail to parse.
func TestVersionFromEntryName(t *testing.T) {
	tests := []struct {
		entryName string
		want      string // empty means "not parseable"
	}{
		{"rhods-operator.3.4.3", "3.4.3"},
		{"rhods-operator.3.5.0-ea.2", "3.5.0-ea.2"},
		{"rhods-operator.1.20.1-8", "1.20.1-8"},
		{"foo.v1.3.0", "1.3.0"},
		{"amq-broker.v7.12.0-opr-1-0.1780501200.p", "7.12.0-opr-1-0.1780501200.p"},
		{"no-version-here", ""},
	}
	for _, tt := range tests {
		t.Run(tt.entryName, func(t *testing.T) {
			v, ok := versionFromEntryName(tt.entryName)
			if tt.want == "" {
				assert.False(t, ok)
				return
			}
			assert.True(t, ok)
			assert.Equal(t, tt.want, v.String())
		})
	}
}

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
					Packages: []v2alpha1.IncludePackage{{Name: "foo", Channels: []v2alpha1.IncludeChannel{{Name: "stable", IncludeBundle: v2alpha1.IncludeBundle{MaxVersion: "1.0.0"}}}}},
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
					Packages: []v2alpha1.IncludePackage{{Name: "foo", Channels: []v2alpha1.IncludeChannel{{Name: "stable", IncludeBundle: v2alpha1.IncludeBundle{MaxVersion: "1.0.0"}}}}},
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
		{
			// Only the head's coverage matters: the intermediary entries
			// (v1.2.0, v1.1.0) carry no skips of their own, yet elimination
			// still proceeds because the surviving head skips every version
			// it jumps over. A check that required each intermediary to skip
			// its successors would refuse this case.
			name: "only head coverage matters",
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
					},
					{
						Name:     "foo.v1.1.0",
						Replaces: "foo.v1.0.0",
					},
					{
						Name: "foo.v1.0.0",
					},
				}}},
			},
			filter: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{{Name: "foo", Channels: []v2alpha1.IncludeChannel{{Name: "stable", IncludeBundle: v2alpha1.IncludeBundle{MaxVersion: "1.0.0"}}}}},
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
			// The replaces chain must be followed by pointer, not by array
			// adjacency. Here the entries are head-first but scrambled so that
			// entries[i].Replaces != entries[i+1].Name. An index-based walk
			// gives up immediately; following the chain still eliminates the
			// intermediaries.
			name: "replaces chain is followed regardless of array order",
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{{Name: "foo"}},
				Channels: []declcfg.Channel{{Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
					{Name: "foo.v1.3.0", Replaces: "foo.v1.2.0", Skips: []string{"foo.v1.2.0", "foo.v1.1.0", "foo.v1.0.0"}},
					{Name: "foo.v1.1.0", Replaces: "foo.v1.0.0"},
					{Name: "foo.v1.2.0", Replaces: "foo.v1.1.0"},
					{Name: "foo.v1.0.0"},
				}}},
			},
			filter: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{{Name: "foo", Channels: []v2alpha1.IncludeChannel{{Name: "stable", IncludeBundle: v2alpha1.IncludeBundle{MaxVersion: "1.0.0"}}}}},
				},
			},
			want: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{{Name: "foo"}},
				Channels: []declcfg.Channel{{Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
					{Name: "foo.v1.3.0", Replaces: "foo.v1.0.0", Skips: []string{"foo.v1.2.0", "foo.v1.1.0", "foo.v1.0.0"}},
					{Name: "foo.v1.0.0"},
				}}}},
		},
		{
			// The head is discovered, not assumed to be entries[0]. Here the
			// entries are in ascending order (oldest first, head last), as real
			// catalogs store them. The head is v1.3.0 because no entry replaces
			// it, and elimination proceeds from there.
			name: "head is discovered when it is not the first entry",
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{{Name: "foo"}},
				Channels: []declcfg.Channel{{Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
					{Name: "foo.v1.0.0"},
					{Name: "foo.v1.1.0", Replaces: "foo.v1.0.0"},
					{Name: "foo.v1.2.0", Replaces: "foo.v1.1.0"},
					{Name: "foo.v1.3.0", Replaces: "foo.v1.2.0", Skips: []string{"foo.v1.2.0", "foo.v1.1.0", "foo.v1.0.0"}},
				}}},
			},
			filter: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{{Name: "foo", Channels: []v2alpha1.IncludeChannel{{Name: "stable", IncludeBundle: v2alpha1.IncludeBundle{MaxVersion: "1.0.0"}}}}},
				},
			},
			want: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{{Name: "foo"}},
				Channels: []declcfg.Channel{{Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
					{Name: "foo.v1.0.0"},
					{Name: "foo.v1.3.0", Replaces: "foo.v1.0.0", Skips: []string{"foo.v1.2.0", "foo.v1.1.0", "foo.v1.0.0"}},
				}}}},
		},
		{
			// maxVersion equals the head's version, so there is nothing above it to
			// eliminate: the walk stops at the first entry below the head and the
			// channel is returned unchanged.
			name: "maxVersion equals head version leaves the channel unchanged",
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{{Name: "foo"}},
				Channels: []declcfg.Channel{{Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
					{Name: "foo.v1.3.0", Replaces: "foo.v1.2.0", Skips: []string{"foo.v1.2.0", "foo.v1.1.0", "foo.v1.0.0"}},
					{Name: "foo.v1.2.0", Replaces: "foo.v1.1.0"},
					{Name: "foo.v1.1.0", Replaces: "foo.v1.0.0"},
					{Name: "foo.v1.0.0"},
				}}},
			},
			filter: v2alpha1.Operator{
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{{Name: "foo", Channels: []v2alpha1.IncludeChannel{{Name: "stable", IncludeBundle: v2alpha1.IncludeBundle{MaxVersion: "1.3.0"}}}}},
				},
			},
			want: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{{Name: "foo"}},
				Channels: []declcfg.Channel{{Name: "stable", Package: "foo", Entries: []declcfg.ChannelEntry{
					{Name: "foo.v1.3.0", Replaces: "foo.v1.2.0", Skips: []string{"foo.v1.2.0", "foo.v1.1.0", "foo.v1.0.0"}},
					{Name: "foo.v1.2.0", Replaces: "foo.v1.1.0"},
					{Name: "foo.v1.1.0", Replaces: "foo.v1.0.0"},
					{Name: "foo.v1.0.0"},
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
