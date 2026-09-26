package release

import (
	"testing"

	"github.com/blang/semver/v4"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

func TestFindLatestRelease(t *testing.T) {
	channelName := "test-channel"

	tests := []struct {
		name         string
		min          bool
		expectedVer  semver.Version
		expectedChan string
		channels     []v2alpha1.ReleaseChannel
		err          string
	}{{
		name: "Success/MinVersion",
		channels: []v2alpha1.ReleaseChannel{
			{
				Name:       channelName,
				MinVersion: "4.0.0-5",
			},
			{
				Name:       "another-channel",
				MinVersion: "4.0.0-6",
			},
		},
		expectedVer:  semver.MustParse("4.0.0-5"),
		expectedChan: channelName,
		min:          true,
	}, {
		name: "Success/MaxVersion",
		channels: []v2alpha1.ReleaseChannel{
			{
				Name:       channelName,
				MaxVersion: "4.0.0-5",
			},
			{
				Name:       "another-channel",
				MaxVersion: "4.0.0-6",
			},
		},
		expectedVer:  semver.MustParse("4.0.0-6"),
		expectedChan: "another-channel",
		min:          false,
	}, {
		name:     "FailureNoPreviousRelease",
		channels: []v2alpha1.ReleaseChannel{},
		err:      ErrNoPreviousRelease.Error(),
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			ch, ver, err := FindRelease(test.channels, test.min)

			if len(test.err) != 0 {
				require.Equal(t, err.Error(), test.err)
			} else {
				require.NoError(t, err)
				if !ver.EQ(test.expectedVer) {
					t.Errorf("Test failed. Expected %s, got %s", test.expectedVer.String(), ver.String())
				}
				if ch != test.expectedChan {
					t.Errorf("Test failed. Expected %s, got %s", test.expectedChan, ch)
				}
			}
		})
	}
}

func TestShortestPathChannels(t *testing.T) {
	tests := []struct {
		name        string
		channelName string
		first       string
		last        string
		wantSource  string
		wantTarget  string
		wantErr     bool
	}{
		{
			name:        "same minor keeps channel name",
			channelName: "stable-4.14",
			first:       "4.14.1",
			last:        "4.14.10",
			wantSource:  "stable-4.14",
			wantTarget:  "stable-4.14",
		},
		{
			name:        "OCPBUGS-85582 cross-minor EUS-style path",
			channelName: "stable-4.14",
			first:       "4.12.40",
			last:        "4.14.10",
			wantSource:  "stable-4.12",
			wantTarget:  "stable-4.14",
		},
		{
			name:        "eus channel prefix preserved",
			channelName: "eus-4.14",
			first:       "4.12.40",
			last:        "4.14.10",
			wantSource:  "eus-4.12",
			wantTarget:  "eus-4.14",
		},
		{
			name:        "invalid channel name",
			channelName: "stable",
			first:       "4.12.0",
			last:        "4.14.0",
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, target, err := shortestPathChannels(tt.channelName, semver.MustParse(tt.first), semver.MustParse(tt.last))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantSource, source)
			require.Equal(t, tt.wantTarget, target)
		})
	}
}
