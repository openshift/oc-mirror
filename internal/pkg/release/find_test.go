package release

import (
	"testing"

	"github.com/blang/semver/v4"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/stretchr/testify/require"
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
