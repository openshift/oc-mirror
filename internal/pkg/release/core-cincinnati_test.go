package release

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/cincinnati"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

var _ Client = &mockClient{}

type mockClient struct {
	url  *url.URL
	Fail bool
}

func TestGetUpdates(t *testing.T) {
	tempDir := t.TempDir()

	arch := "test-arch"
	channelName := "stable-4.0"
	tests := []struct {
		name    string
		version string
		reqVer  string

		expectedQuery string
		current       cincinnati.Update
		requested     cincinnati.Update
		available     []cincinnati.Update
		err           string
	}{{
		name:          "Valid/DirectUpdate",
		version:       "4.0.0-4",
		reqVer:        "4.0.0-5",
		expectedQuery: "arch=test-arch&channel=stable-4.0&id=01234567-0123-0123-0123-0123456789ab&version=4.0.0-4",
		current:       cincinnati.Update{Version: semver.MustParse("4.0.0-4"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-4"},
		requested:     cincinnati.Update{Version: semver.MustParse("4.0.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-5"},
		available: []cincinnati.Update{
			{Version: semver.MustParse("4.0.0-4"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-4"},
			{Version: semver.MustParse("4.0.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-5"},
		},
	}, {
		name:          "Valid/FullChannel",
		version:       "4.0.0-4",
		reqVer:        "4.0.0-8",
		expectedQuery: "arch=test-arch&channel=stable-4.0&id=01234567-0123-0123-0123-0123456789ab&version=4.0.0-4",
		current:       cincinnati.Update{Version: semver.MustParse("4.0.0-4"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-4"},
		requested:     cincinnati.Update{Version: semver.MustParse("4.0.0-8"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-8"},
		available: []cincinnati.Update{
			{Version: semver.MustParse("4.0.0-4"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-4"},
			{Version: semver.MustParse("4.0.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-5"},
			{Version: semver.MustParse("4.0.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-6"},
			{Version: semver.MustParse("4.0.0-8"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-8"},
		},
	}, {
		name:          "Valid/NoUpdates",
		version:       "4.0.0-4",
		reqVer:        "4.0.0-0.okd-0",
		current:       cincinnati.Update{Version: semver.MustParse("4.0.0-4"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-4"},
		requested:     cincinnati.Update{Version: semver.MustParse("4.0.0-0.okd-0"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-0.okd-0"},
		expectedQuery: "arch=test-arch&channel=stable-4.0&id=01234567-0123-0123-0123-0123456789ab&version=4.0.0-4",
		available:     nil,
	}, {
		name:          "Invalid/UnknownCurrentVersion",
		version:       "4.0.0-3",
		reqVer:        "0.0.0",
		expectedQuery: "arch=test-arch&channel=stable-4.0&id=01234567-0123-0123-0123-0123456789ab&version=4.0.0-3",
		err:           "VersionNotFound: current version 4.0.0-3 not found in the \"stable-4.0\" channel",
	}, {
		name:          "Invalid/UnknownRequestedVersion",
		version:       "4.0.0-5",
		reqVer:        "4.0.0-9",
		expectedQuery: "arch=test-arch&channel=stable-4.0&id=01234567-0123-0123-0123-0123456789ab&version=4.0.0-5",
		err:           "VersionNotFound: requested version 4.0.0-9 not found in the \"stable-4.0\" channel",
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestQuery := make(chan string, 1)
			defer close(requestQuery)

			handler := getHandlerMulti(t, requestQuery)

			ts := httptest.NewServer(http.HandlerFunc(handler))
			t.Cleanup(ts.Close)

			endpoint, err := url.Parse(ts.URL)
			require.NoError(t, err)
			c := &mockClient{url: endpoint}

			cs := CincinnatiSchema{Log: clog.New("trace"), Client: c, CincinnatiParams: CincinnatiParams{Arch: arch, GraphDataDir: tempDir}}

			current, requested, updates, err := GetUpdates(context.Background(), cs, channelName, semver.MustParse(test.version), semver.MustParse(test.reqVer))
			if test.err == "" {
				require.NoError(t, err)
				require.Equal(t, test.current, current)
				require.Equal(t, test.requested, requested)
				require.Equal(t, test.available, updates)
			} else {
				require.EqualError(t, err, test.err)
			}

			actualQuery := ""
			select {
			case actualQuery = <-requestQuery:
			default:
				t.Fatal("no request received at upstream URL")
			}
			expectedQueryValues, err := url.ParseQuery(test.expectedQuery)
			require.NoError(t, err)
			actualQueryValues, err := url.ParseQuery(actualQuery)
			require.NoError(t, err)
			require.Equal(t, expectedQueryValues, actualQueryValues)
		})
	}
}

func TestGetMinorMax(t *testing.T) {
	arch := "test-arch"
	channelName := "stable-4.0"

	tempDir := t.TempDir()

	tests := []struct {
		name string

		expectedQuery string
		version       semver.Version
		min           bool
		err           string
	}{{
		name:          "Valid/MaxChannel",
		expectedQuery: "arch=test-arch&channel=stable-4.0&id=01234567-0123-0123-0123-0123456789ab",
		version:       semver.MustParse("4.0.0-8"),
	}, {
		name:          "Valid/MinChannel",
		expectedQuery: "arch=test-arch&channel=stable-4.0&id=01234567-0123-0123-0123-0123456789ab",
		version:       semver.MustParse("4.0.0-0.okd-0"),
		min:           true,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestQuery := make(chan string, 1)
			defer close(requestQuery)

			handler := getHandlerMulti(t, requestQuery)

			ts := httptest.NewServer(http.HandlerFunc(handler))
			t.Cleanup(ts.Close)

			endpoint, err := url.Parse(ts.URL)
			require.NoError(t, err)
			c := &mockClient{url: endpoint}

			cs := CincinnatiSchema{Log: clog.New("trace"), Client: c, CincinnatiParams: CincinnatiParams{Arch: arch, GraphDataDir: tempDir}}
			version, err := GetChannelMinOrMax(context.Background(), cs, channelName, test.min)
			if test.err == "" {
				require.NoError(t, err)
				require.Equal(t, test.version, version)

			} else {
				require.EqualError(t, err, test.err)
			}

			actualQuery := ""
			select {
			case actualQuery = <-requestQuery:
			default:
				t.Fatal("no request received at upstream URL")
			}
			expectedQueryValues, err := url.ParseQuery(test.expectedQuery)
			require.NoError(t, err)
			actualQueryValues, err := url.ParseQuery(actualQuery)
			require.NoError(t, err)
			require.Equal(t, expectedQueryValues, actualQueryValues)
		})
	}
}

func TestGetVersions(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name          string
		channel       string
		arch          string
		expectedQuery string
		versions      []semver.Version
		err           string
	}{
		{
			name:          "Valid/OneChannel",
			channel:       "stable-4.0",
			arch:          "test-arch",
			expectedQuery: "arch=test-arch&channel=stable-4.0&id=01234567-0123-0123-0123-0123456789ab",
			versions:      getSemVers([]string{"4.0.0-0.okd-0", "4.0.0-4", "4.0.0-5", "4.0.0-6", "4.0.0-7", "4.0.0-8"}),
		},
		{
			name:          "Invalid/EmptyChannel",
			channel:       "empty-4.0",
			arch:          "test-arch",
			expectedQuery: "arch=test-arch&channel=empty-4.0&id=01234567-0123-0123-0123-0123456789ab",
			err:           "NoVersionsFound: no cluster versions found in the \"empty-4.0\" channel",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestQuery := make(chan string, 1)
			defer close(requestQuery)

			handler := getHandlerMulti(t, requestQuery)

			ts := httptest.NewServer(http.HandlerFunc(handler))
			t.Cleanup(ts.Close)

			endpoint, err := url.Parse(ts.URL)
			require.NoError(t, err)
			c := &mockClient{url: endpoint}

			cs := CincinnatiSchema{Log: clog.New("trace"), Client: c, CincinnatiParams: CincinnatiParams{Arch: test.arch, GraphDataDir: tempDir}}
			versions, err := GetVersions(context.Background(), cs, test.channel)
			if test.err == "" {
				require.NoError(t, err)
				require.Equal(t, test.versions, versions)

			} else {
				require.EqualError(t, err, test.err)
			}

			actualQuery := ""
			select {
			case actualQuery = <-requestQuery:
			default:
				t.Fatal("no request received at upstream URL")
			}
			expectedQueryValues, err := url.ParseQuery(test.expectedQuery)
			require.NoError(t, err)
			actualQueryValues, err := url.ParseQuery(actualQuery)
			require.NoError(t, err)
			require.Equal(t, expectedQueryValues, actualQueryValues)
		})
	}
}

func TestGetUpdatesInRange(t *testing.T) {
	tempDir := t.TempDir()

	arch := "test-arch"
	channelName := "stable-4.0"
	tests := []struct {
		name          string
		expectedQuery string
		versions      []cincinnati.Update
		releaseRange  semver.Range
		err           string
	}{
		{
			name:          "Valid/OneChannel",
			expectedQuery: "arch=test-arch&channel=stable-4.0&id=01234567-0123-0123-0123-0123456789ab",
			versions: []cincinnati.Update{
				{Version: semver.MustParse("4.0.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-5"},
				{Version: semver.MustParse("4.0.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-6"},
				{Version: semver.MustParse("4.0.0-7"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-7"},
				{Version: semver.MustParse("4.0.0-8"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-8"},
			},
			releaseRange: semver.MustParseRange(">=4.0.0-5"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestQuery := make(chan string, 1)
			defer close(requestQuery)

			handler := getHandlerMulti(t, requestQuery)

			ts := httptest.NewServer(http.HandlerFunc(handler))
			t.Cleanup(ts.Close)

			endpoint, err := url.Parse(ts.URL)
			require.NoError(t, err)
			c := &mockClient{url: endpoint}

			cs := CincinnatiSchema{Log: clog.New("trace"), Client: c, CincinnatiParams: CincinnatiParams{Arch: arch, GraphDataDir: tempDir}}
			versions, err := GetUpdatesInRange(context.TODO(), cs, channelName, test.releaseRange)
			if test.err == "" {
				require.NoError(t, err)
				require.Equal(t, test.versions, versions)

			} else {
				require.EqualError(t, err, test.err)
			}

			actualQuery := ""
			select {
			case actualQuery = <-requestQuery:
			default:
				t.Fatal("no request received at upstream URL")
			}
			expectedQueryValues, err := url.ParseQuery(test.expectedQuery)
			require.NoError(t, err)
			actualQueryValues, err := url.ParseQuery(actualQuery)
			require.NoError(t, err)
			require.Equal(t, expectedQueryValues, actualQueryValues)
		})
	}
}

func TestCalculateUpgrades(t *testing.T) {
	arch := "test-arch"

	tests := []struct {
		name            string
		sourceChannel   string
		targetChannel   string
		curr            semver.Version
		req             semver.Version
		currentUpdate   cincinnati.Update
		requestedUpdate cincinnati.Update
		neededUpdates   []cincinnati.Update
		err             string
	}{{
		name:            "Success/OneChannel",
		sourceChannel:   "stable-4.0",
		targetChannel:   "stable-4.1",
		curr:            semver.MustParse("4.0.0-5"),
		req:             semver.MustParse("4.1.0-6"),
		currentUpdate:   cincinnati.Update{Version: semver.MustParse("4.0.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-5"},
		requestedUpdate: cincinnati.Update{Version: semver.MustParse("4.1.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.1.0-6"},
		neededUpdates: []cincinnati.Update{
			{Version: semver.MustParse("4.0.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-5"},
			{Version: semver.MustParse("4.0.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-6"},
			{Version: semver.MustParse("4.0.0-8"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-8"},
			{Version: semver.MustParse("4.1.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.1.0-6"},
		},
	}, {
		name:            "Success/TwoChannels",
		sourceChannel:   "stable-4.0",
		targetChannel:   "stable-4.2",
		curr:            semver.MustParse("4.0.0-5"),
		req:             semver.MustParse("4.2.0-3"),
		currentUpdate:   cincinnati.Update{Version: semver.MustParse("4.0.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-5"},
		requestedUpdate: cincinnati.Update{Version: semver.MustParse("4.2.0-3"), Image: "quay.io/openshift-release-dev/ocp-release:4.2.0-3"},
		neededUpdates: []cincinnati.Update{
			{Version: semver.MustParse("4.0.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-5"},
			{Version: semver.MustParse("4.0.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-6"},
			{Version: semver.MustParse("4.0.0-8"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-8"},
			{Version: semver.MustParse("4.1.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.1.0-6"},
			{Version: semver.MustParse("4.2.0-3"), Image: "quay.io/openshift-release-dev/ocp-release:4.2.0-3"},
		},
	}, {
		name:            "Success/TwoChannelsDifferentPrefix",
		sourceChannel:   "stable-4.3",
		targetChannel:   "fast-4.3",
		curr:            semver.MustParse("4.3.0"),
		req:             semver.MustParse("4.3.1"),
		currentUpdate:   cincinnati.Update{Version: semver.MustParse("4.3.0"), Image: "quay.io/openshift-release-dev/ocp-release:4.3.0"},
		requestedUpdate: cincinnati.Update{Version: semver.MustParse("4.3.1"), Image: "quay.io/openshift-release-dev/ocp-release:4.3.1"},
		neededUpdates: []cincinnati.Update{
			{Version: semver.MustParse("4.3.0"), Image: "quay.io/openshift-release-dev/ocp-release:4.3.0"},
			{Version: semver.MustParse("4.3.1"), Image: "quay.io/openshift-release-dev/ocp-release:4.3.1"},
		},
	}, {
		name:            "SuccessWithWarning/NoUpgradePath",
		sourceChannel:   "stable-4.1",
		targetChannel:   "stable-4.2",
		curr:            semver.MustParse("4.1.0-6"),
		req:             semver.MustParse("4.2.0-2"),
		currentUpdate:   cincinnati.Update{Version: semver.MustParse("4.1.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.1.0-6"},
		requestedUpdate: cincinnati.Update{Version: semver.MustParse("4.2.0-2"), Image: "quay.io/openshift-release-dev/ocp-release:4.2.0-2"},
		neededUpdates: []cincinnati.Update{
			{Version: semver.MustParse("4.1.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.1.0-6"},
		},
	}, {
		name:            "SuccessWithWarning/BlockedEdge",
		sourceChannel:   "stable-4.2",
		targetChannel:   "stable-4.3",
		curr:            semver.MustParse("4.2.0-3"),
		req:             semver.MustParse("4.3.0"),
		currentUpdate:   cincinnati.Update{Version: semver.MustParse("4.2.0-3"), Image: "quay.io/openshift-release-dev/ocp-release:4.2.0-3"},
		requestedUpdate: cincinnati.Update{Version: semver.MustParse("4.3.0"), Image: "quay.io/openshift-release-dev/ocp-release:4.3.0"},
		neededUpdates: []cincinnati.Update{
			{Version: semver.MustParse("4.2.0-3"), Image: "quay.io/openshift-release-dev/ocp-release:4.2.0-3"},
			{Version: semver.MustParse("4.2.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.2.0-5"},
		},
	}, {
		name:          "Failure/InvalidLastVersion",
		sourceChannel: "stable-4.2",
		targetChannel: "stable-4.3",
		curr:          semver.MustParse("4.2.0-9"),
		req:           semver.MustParse("4.3.4"),
		err:           "channel \"stable-4.2\": VersionNotFound: current version 4.2.0-9 not found in the \"stable-4.2\" channel",
	}, {
		name:          "Failure/InvalidRequestedVersion",
		sourceChannel: "stable-4.2",
		targetChannel: "stable-4.3",
		curr:          semver.MustParse("4.2.0-3"),
		req:           semver.MustParse("4.3.5"),
		err:           "VersionNotFound: current version 4.3.5 not found in the \"stable-4.3\" channel",
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestQuery := make(chan string, 10)
			defer close(requestQuery)

			handler := getHandlerMulti(t, requestQuery)

			ts := httptest.NewServer(http.HandlerFunc(handler))
			t.Cleanup(ts.Close)

			endpoint, err := url.Parse(ts.URL)
			require.NoError(t, err)

			cs := CincinnatiSchema{Log: clog.New("trace"), Client: &mockClient{url: endpoint}, CincinnatiParams: CincinnatiParams{Arch: arch, GraphDataDir: t.TempDir()}}
			cur, req, updates, err := CalculateUpgrades(context.Background(), cs, test.sourceChannel, test.targetChannel, test.curr, test.req)

			if test.err == "" {
				require.NoError(t, err)
				require.Equal(t, test.currentUpdate, cur)
				require.Equal(t, test.requestedUpdate, req)
				require.Equal(t, test.neededUpdates, updates)
			} else {
				require.EqualError(t, err, test.err)
			}
		})
	}
}

func TestHandleBlockedEdges(t *testing.T) {
	arch := "test-arch"

	tests := []struct {
		name          string
		sourceChannel string
		targetChannel string
		last          semver.Version
		req           semver.Version
		exp           bool
		err           string
	}{{
		name:          "Success/OneChannel",
		sourceChannel: "stable-4.0",
		targetChannel: "stable-4.1",
		last:          semver.MustParse("4.0.0-5"),
		req:           semver.MustParse("4.1.0-6"),
		exp:           false,
	}, {
		name:          "Success/TwoChannelsDifferentPrefix",
		sourceChannel: "stable-4.3",
		targetChannel: "fast-4.3",
		last:          semver.MustParse("4.3.0"),
		req:           semver.MustParse("4.3.1"),
		exp:           false,
	}, {
		name:          "SuccessWithWarning/NoUpgradePath",
		sourceChannel: "stable-4.1",
		targetChannel: "stable-4.2",
		last:          semver.MustParse("4.1.0-6"),
		req:           semver.MustParse("4.2.0-2"),
		exp:           false,
	}, {
		name:          "SuccessWithWarning/BlockedEdge",
		sourceChannel: "stable-4.2",
		targetChannel: "stable-4.3",
		last:          semver.MustParse("4.2.0-3"),
		req:           semver.MustParse("4.3.0"),
		exp:           true,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestQuery := make(chan string, 10)
			defer close(requestQuery)

			handler := getHandlerMulti(t, requestQuery)

			ts := httptest.NewServer(http.HandlerFunc(handler))
			t.Cleanup(ts.Close)

			endpoint, err := url.Parse(ts.URL)
			require.NoError(t, err)

			cs := CincinnatiSchema{Log: clog.New("trace"), Client: &mockClient{url: endpoint}, CincinnatiParams: CincinnatiParams{Arch: arch, GraphDataDir: t.TempDir()}}
			isBlocked, err := handleBlockedEdges(context.Background(), cs, test.targetChannel, test.last)

			if test.err == "" {
				require.NoError(t, err)
				require.Equal(t, test.exp, isBlocked)
			} else {
				require.EqualError(t, err, test.err)
			}
		})
	}
}

func (c mockClient) GetID() uuid.UUID {
	return uuid.MustParse("01234567-0123-0123-0123-0123456789ab")
}

func (c mockClient) SetQueryParams(arch, channel, version string) {
	queryParams := c.url.Query()
	queryParams.Add("id", c.GetID().String())
	params := map[string]string{
		"arch":    arch,
		"channel": channel,
		"version": version,
	}
	for key, value := range params {
		if value != "" {
			queryParams.Add(key, value)
		}
	}
	c.url.RawQuery = queryParams.Encode()
}

func (c mockClient) GetURL() *url.URL {
	return c.url
}

func (c mockClient) GetTransport() *http.Transport {
	return &http.Transport{}
}

func TestGetSemVerFromChannel(t *testing.T) {
	tests := []struct {
		name          string
		sourceChannel string
		targetChannel string
		err           string
		expSource     semver.Version
		expTarget     semver.Version
		expPrefix     string
	}{
		{
			name:          "Valid/StableChannels",
			sourceChannel: "stable-4.1",
			targetChannel: "fast-4.2",
			expSource:     semver.MustParse("4.1.0"),
			expTarget:     semver.MustParse("4.2.0"),
			expPrefix:     "stable",
		},
		{
			name:          "Invalid/InvalidChannelPrefix",
			sourceChannel: "stable-4.1",
			targetChannel: "fast:4.2",
			err:           "invalid channel name fast:4.2",
		},
	}
	for _, test := range tests {
		source, target, prefix, err := getSemverFromChannels(test.sourceChannel, test.targetChannel)
		t.Run(test.name, func(t *testing.T) {
			if test.err == "" {
				require.NoError(t, err)
				require.Equal(t, test.expPrefix, prefix)
				require.Equal(t, test.expSource, source)
				require.Equal(t, test.expTarget, target)
			} else {
				require.EqualError(t, err, test.err)
			}
		})
	}
}

func getSemVers(stringVers []string) (vers []semver.Version) {
	for _, stringVer := range stringVers {
		vers = append(vers, semver.MustParse(stringVer))
	}
	return vers
}

// multiChannelGraphs are the graphs served by getHandlerMulti by channel name.
var multiChannelGraphs = map[string]string{
	"empty-4.0": `{
				"nodes": [],
				"edges": []
			  }`,
	"stable-4.0": `{
				"nodes": [
				  {
					"version": "4.0.0-4",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-4"
				  },
				  {
					"version": "4.0.0-5",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-5"
				  },
				  {
					"version": "4.0.0-6",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-6"
				  },
				  {
					"version": "4.0.0-0.okd-0",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.okd-0"
				  },
				  {
					"version": "4.0.0-7",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-7"
				  },
				  {
					"version": "4.0.0-8",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-8"
				  }
				],
				"edges": [[0,1],[1,2],[2,4],[2,5],[4,5]]
			  }`,
	"stable-4.1": `{
				"nodes": [
				  {
					"version": "4.0.0-4",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-4"
				  },
				  {
					"version": "4.0.0-5",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-5"
				  },
				  {
					"version": "4.0.0-6",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-6"
				  },
				  {
					"version": "4.0.0-0.okd-0",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.okd-0"
				  },
				  {
					"version": "4.0.0-7",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-7"
				  },
				  {
					"version": "4.0.0-8",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-8"
				  },
				  {
					"version": "4.1.0-6",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.1.0-6"
				  }
				],
				"edges": [[0,1],[1,2],[2,4],[2,5],[4,5],[5,6]]
			  }`,
	"stable-4.2": `{
				"nodes": [
				{
					"version": "4.1.0-6",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.1.0-6"
				},
				{
					"version": "4.2.0-2",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.2.0-2"
				},
				{
					"version": "4.2.0-3",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.2.0-3"
				},
				{
					"version": "4.2.0-5",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.2.0-5"
				}
				],
				"edges": [[0,2],[1,2],[2,3]]
			}`,
	"stable-4.3": `{
				"nodes": [
				{
					"version": "4.3.0",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.3.0"
				},
				{
					"version": "4.3.1",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.3.1"
				}
				],
				"edges": [[0,1]]
			}`,
	"fast-4.3": `{
				"nodes": [
				{
					"version": "4.2.0-5",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.2.0-5"
				},	
				{
					"version": "4.3.0",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.3.0"
				},
				{
					"version": "4.3.1",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.3.1"
				},
				{
					"version": "4.3.2",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.3.2"
				}
				],
				"edges": [[0,1],[1,2],[2,3]]
			}`,
}

// getHanlderMulti mocks a multi channel Cincinnati API
func getHandlerMulti(t *testing.T, requestQuery chan<- string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case requestQuery <- r.URL.RawQuery:
		default:
			// t.Fatalf("received multiple requests at upstream URL")
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		mtype := r.Header.Get("Accept")
		if mtype != "application/json" {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}

		keys, ok := r.URL.Query()["channel"]
		if !ok {
			t.Fail()
		}

		ch := keys[len(keys)-1]

		graph, ok := multiChannelGraphs[ch]
		if !ok {
			t.Fail()
			return
		}
		if _, err := w.Write([]byte(graph)); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			t.Fatal(err)
			return
		}
	}
}

// OCPBUGS-136257: upgrades between channels of different major versions are calculated from
// the channel graphs, not by incrementing the minor version of the source channel.
func TestCalculateUpgradesCrossMajor(t *testing.T) {
	noPath := func(start, source, req, target, detail string) string {
		return "NoUpgradePath: no upgrade path from version " + start + " in channel \"" + source + "\" to version " + req + " in channel \"" + target + "\": " + detail
	}

	// Graph shapes of the 4.22 to 5.0 boundary: the target channel contains the 4.22 releases,
	// the newest 4.22 release has no edge to 5.0.0 yet, and the next minor version channel only
	// contains pre-releases.
	candidate422 := testGraph(t, []string{"4.22.19", "4.22.20", "4.22.21", "4.22.23"}, [][2]int{{0, 1}, {1, 2}, {2, 3}})
	candidate423 := testGraph(t, []string{"4.22.21", "4.23.0-ec.1"}, [][2]int{{0, 1}})
	candidate50 := testGraph(t, []string{"4.22.19", "4.22.20", "4.22.21", "4.22.23", "5.0.0-rc.1", "5.0.0"},
		[][2]int{{0, 1}, {1, 2}, {1, 5}, {2, 5}, {4, 5}})
	stable50 := testGraph(t, []string{"4.22.1", "4.22.2", "5.0.0"}, [][2]int{{0, 1}, {1, 2}})
	stable422 := testGraph(t, []string{"4.21.1", "4.21.2", "4.22.1", "4.22.2"}, [][2]int{{0, 1}, {1, 2}, {2, 3}})

	tests := []struct {
		name          string
		graphs        map[string]string
		failing       []string
		sourceChannel string
		targetChannel string
		curr          string
		req           string
		expCurrent    string
		expRequested  string
		expUpdates    []string
		expRequests   []string
		err           string
	}{{
		name:          "Success/IssueCandidateChannels",
		graphs:        map[string]string{"candidate-4.22": candidate422, "candidate-4.23": candidate423, "candidate-5.0": candidate50},
		sourceChannel: "candidate-4.22",
		targetChannel: "candidate-5.0",
		curr:          "4.22.20",
		req:           "5.0.0",
		expCurrent:    "4.22.20",
		expRequested:  "5.0.0",
		expUpdates:    []string{"4.22.20", "5.0.0"},
		expRequests:   []string{"candidate-5.0"},
	}, {
		name: "Success/StableChannels",
		graphs: map[string]string{
			"stable-4.22": testGraph(t, []string{"4.22.19", "4.22.20"}, [][2]int{{0, 1}}),
			"stable-5.0":  testGraph(t, []string{"4.22.19", "4.22.20", "5.0.0"}, [][2]int{{0, 1}, {1, 2}}),
		},
		sourceChannel: "stable-4.22",
		targetChannel: "stable-5.0",
		curr:          "4.22.20",
		req:           "5.0.0",
		expCurrent:    "4.22.20",
		expRequested:  "5.0.0",
		expUpdates:    []string{"4.22.20", "5.0.0"},
		expRequests:   []string{"stable-5.0"},
	}, {
		name:          "Success/LastMinorVersionToNextMajor",
		graphs:        map[string]string{"candidate-5.0": testGraph(t, []string{"4.23.1", "4.23.2", "5.0.0"}, [][2]int{{0, 1}, {1, 2}})},
		sourceChannel: "candidate-4.23",
		targetChannel: "candidate-5.0",
		curr:          "4.23.1",
		req:           "5.0.0",
		expCurrent:    "4.23.1",
		expRequested:  "5.0.0",
		expUpdates:    []string{"4.23.1", "4.23.2", "5.0.0"},
		expRequests:   []string{"candidate-5.0"},
	}, {
		// The target channel does not contain the start version: it is entered from the latest
		// 4.22 release, reached in the stable-4.22 channel.
		name:          "Success/EntryRelease",
		graphs:        map[string]string{"stable-5.0": stable50, "stable-4.22": stable422},
		sourceChannel: "stable-4.21",
		targetChannel: "stable-5.0",
		curr:          "4.21.1",
		req:           "5.0.0",
		expCurrent:    "4.21.1",
		expRequested:  "5.0.0",
		expUpdates:    []string{"4.21.1", "4.21.2", "4.22.1", "4.22.2", "5.0.0"},
		expRequests:   []string{"stable-5.0", "stable-4.22"},
	}, {
		// 4.22.3 is the latest release leading to 5.0.0 but it is not reachable from 4.21.2.
		name: "Success/EntryReleaseUnreachableLatest",
		graphs: map[string]string{
			"stable-5.0":  testGraph(t, []string{"4.22.1", "4.22.2", "4.22.3", "5.0.0"}, [][2]int{{0, 1}, {1, 3}, {2, 3}}),
			"stable-4.22": testGraph(t, []string{"4.21.2", "4.22.1", "4.22.2", "4.22.3"}, [][2]int{{0, 1}, {1, 2}}),
		},
		sourceChannel: "stable-4.21",
		targetChannel: "stable-5.0",
		curr:          "4.21.2",
		req:           "5.0.0",
		expCurrent:    "4.21.2",
		expRequested:  "5.0.0",
		expUpdates:    []string{"4.21.2", "4.22.1", "4.22.2", "5.0.0"},
		expRequests:   []string{"stable-5.0", "stable-4.22"},
	}, {
		name: "Failure/EntryReleaseUnreachable",
		graphs: map[string]string{
			"stable-5.0":  stable50,
			"stable-4.22": testGraph(t, []string{"4.21.2", "4.22.1", "4.22.2"}, [][2]int{{1, 2}}),
		},
		sourceChannel: "stable-4.21",
		targetChannel: "stable-5.0",
		curr:          "4.21.2",
		req:           "5.0.0",
		expRequests:   []string{"stable-5.0", "stable-4.22"},
		err: noPath("4.21.2", "stable-4.21", "5.0.0", "stable-5.0",
			`channel "stable-5.0" contains neither version 4.21.2 nor a release of an earlier minor version that is reachable from it and leads to 5.0.0 through generally available releases`),
	}, {
		name: "Success/TargetBeyondFirstMinorOfNextMajor",
		graphs: map[string]string{
			"stable-5.1": testGraph(t, []string{"5.0.0", "5.1.0"}, [][2]int{{0, 1}}),
			"stable-5.0": stable50,
		},
		sourceChannel: "stable-4.22",
		targetChannel: "stable-5.1",
		curr:          "4.22.1",
		req:           "5.1.0",
		expCurrent:    "4.22.1",
		expRequested:  "5.1.0",
		expUpdates:    []string{"4.22.1", "4.22.2", "5.0.0", "5.1.0"},
		expRequests:   []string{"stable-5.1", "stable-5.0"},
	}, {
		// Only one channel of an earlier minor version is used to enter the target channel.
		name: "Failure/TwoMajorVersionsAhead",
		graphs: map[string]string{
			"stable-6.0": testGraph(t, []string{"5.1.0", "6.0.0"}, [][2]int{{0, 1}}),
			"stable-5.1": testGraph(t, []string{"5.0.0", "5.1.0"}, [][2]int{{0, 1}}),
			"stable-5.0": stable50,
		},
		sourceChannel: "stable-4.22",
		targetChannel: "stable-6.0",
		curr:          "4.22.1",
		req:           "6.0.0",
		expRequests:   []string{"stable-6.0", "stable-5.1"},
		err: noPath("4.22.1", "stable-4.22", "6.0.0", "stable-6.0",
			`channel "stable-6.0" contains neither version 4.22.1 nor a release of an earlier minor version that is reachable from it and leads to 6.0.0 through generally available releases`),
	}, {
		name:          "Failure/TargetWithoutSourceReleases",
		graphs:        map[string]string{"fast-4.22": candidate422, "fast-5.0": testGraph(t, []string{"5.0.0"}, nil)},
		sourceChannel: "fast-4.22",
		targetChannel: "fast-5.0",
		curr:          "4.22.20",
		req:           "5.0.0",
		expRequests:   []string{"fast-5.0"},
		err: noPath("4.22.20", "fast-4.22", "5.0.0", "fast-5.0",
			`channel "fast-5.0" contains neither version 4.22.20 nor a release of an earlier minor version that is reachable from it and leads to 5.0.0 through generally available releases`),
	}, {
		name:          "Failure/NoEdgeInTarget",
		graphs:        map[string]string{"candidate-5.0": testGraph(t, []string{"4.22.20", "5.0.0"}, nil)},
		sourceChannel: "candidate-4.22",
		targetChannel: "candidate-5.0",
		curr:          "4.22.20",
		req:           "5.0.0",
		expRequests:   []string{"candidate-5.0"},
		err: noPath("4.22.20", "candidate-4.22", "5.0.0", "candidate-5.0",
			`channel "candidate-5.0" has no path from 4.22.20 to 5.0.0 through generally available releases`),
	}, {
		name:          "Failure/RequestedVersionNotInTarget",
		graphs:        map[string]string{"candidate-5.0": testGraph(t, []string{"4.22.20", "4.22.21"}, [][2]int{{0, 1}})},
		sourceChannel: "candidate-4.22",
		targetChannel: "candidate-5.0",
		curr:          "4.22.20",
		req:           "5.0.0",
		expRequests:   []string{"candidate-5.0"},
		err:           noPath("4.22.20", "candidate-4.22", "5.0.0", "candidate-5.0", `version 5.0.0 is not in channel "candidate-5.0"`),
	}, {
		name:          "Failure/TargetChannelRequestError",
		failing:       []string{"candidate-5.0"},
		sourceChannel: "candidate-4.22",
		targetChannel: "candidate-5.0",
		curr:          "4.22.20",
		req:           "5.0.0",
		expRequests:   []string{"candidate-5.0"},
		err:           `APIRequestError: channel "candidate-5.0": GetGraphDataFailed: unexpected HTTP status 500 Internal Server Error`,
	}, {
		name:          "Failure/EntryChannelRequestError",
		graphs:        map[string]string{"stable-5.0": stable50},
		failing:       []string{"stable-4.22"},
		sourceChannel: "stable-4.21",
		targetChannel: "stable-5.0",
		curr:          "4.21.1",
		req:           "5.0.0",
		expRequests:   []string{"stable-5.0", "stable-4.22"},
		err:           `APIRequestError: channel "stable-4.22": GetGraphDataFailed: unexpected HTTP status 500 Internal Server Error`,
	}, {
		// The pre-release route is shorter, but generally available releases are only upgraded
		// through generally available releases.
		name: "Success/GeneralAvailabilityRoute",
		graphs: map[string]string{"candidate-5.0": testGraph(t, []string{"4.22.20", "4.22.21", "4.22.22", "5.0.0-rc.8", "5.0.0"},
			[][2]int{{0, 3}, {3, 4}, {0, 1}, {1, 2}, {2, 4}})},
		sourceChannel: "candidate-4.22",
		targetChannel: "candidate-5.0",
		curr:          "4.22.20",
		req:           "5.0.0",
		expCurrent:    "4.22.20",
		expRequested:  "5.0.0",
		expUpdates:    []string{"4.22.20", "4.22.21", "4.22.22", "5.0.0"},
		expRequests:   []string{"candidate-5.0"},
	}, {
		name:          "Failure/PreReleaseRouteOnly",
		graphs:        map[string]string{"candidate-5.0": testGraph(t, []string{"4.22.20", "5.0.0-rc.8", "5.0.0"}, [][2]int{{0, 1}, {1, 2}})},
		sourceChannel: "candidate-4.22",
		targetChannel: "candidate-5.0",
		curr:          "4.22.20",
		req:           "5.0.0",
		expRequests:   []string{"candidate-5.0"},
		err: noPath("4.22.20", "candidate-4.22", "5.0.0", "candidate-5.0",
			`channel "candidate-5.0" has no path from 4.22.20 to 5.0.0 through generally available releases`),
	}, {
		name: "Success/GeneralAvailabilityEntryRelease",
		graphs: map[string]string{
			"stable-5.0":  testGraph(t, []string{"4.22.2", "4.23.0-ec.1", "5.0.0"}, [][2]int{{0, 2}, {1, 2}}),
			"stable-4.22": testGraph(t, []string{"4.21.2", "4.22.2"}, [][2]int{{0, 1}}),
			"stable-4.23": testGraph(t, []string{"4.21.2", "4.23.0-ec.1"}, [][2]int{{0, 1}}),
		},
		sourceChannel: "stable-4.21",
		targetChannel: "stable-5.0",
		curr:          "4.21.2",
		req:           "5.0.0",
		expCurrent:    "4.21.2",
		expRequested:  "5.0.0",
		expUpdates:    []string{"4.21.2", "4.22.2", "5.0.0"},
		expRequests:   []string{"stable-5.0", "stable-4.22"},
	}, {
		name: "Failure/PreReleaseEntryReleaseOnly",
		graphs: map[string]string{
			"stable-5.0":  testGraph(t, []string{"4.23.0-ec.1", "5.0.0"}, [][2]int{{0, 1}}),
			"stable-4.23": testGraph(t, []string{"4.22.2", "4.23.0-ec.1"}, [][2]int{{0, 1}}),
		},
		sourceChannel: "stable-4.22",
		targetChannel: "stable-5.0",
		curr:          "4.22.2",
		req:           "5.0.0",
		expRequests:   []string{"stable-5.0"},
		err: noPath("4.22.2", "stable-4.22", "5.0.0", "stable-5.0",
			`channel "stable-5.0" contains neither version 4.22.2 nor a release of an earlier minor version that is reachable from it and leads to 5.0.0 through generally available releases`),
	}, {
		// A requested pre-release can be reached through pre-releases.
		name:          "Success/RequestedPreRelease",
		graphs:        map[string]string{"candidate-5.0": testGraph(t, []string{"4.22.20", "5.0.0-rc.8"}, [][2]int{{0, 1}})},
		sourceChannel: "candidate-4.22",
		targetChannel: "candidate-5.0",
		curr:          "4.22.20",
		req:           "5.0.0-rc.8",
		expCurrent:    "4.22.20",
		expRequested:  "5.0.0-rc.8",
		expUpdates:    []string{"4.22.20", "5.0.0-rc.8"},
		expRequests:   []string{"candidate-5.0"},
	}, {
		// A pre-release start version can be upgraded through pre-releases.
		name:          "Success/StartPreRelease",
		graphs:        map[string]string{"candidate-5.0": testGraph(t, []string{"4.22.0-rc.1", "5.0.0-rc.8", "5.0.0"}, [][2]int{{0, 1}, {1, 2}})},
		sourceChannel: "candidate-4.22",
		targetChannel: "candidate-5.0",
		curr:          "4.22.0-rc.1",
		req:           "5.0.0",
		expCurrent:    "4.22.0-rc.1",
		expRequested:  "5.0.0",
		expUpdates:    []string{"4.22.0-rc.1", "5.0.0-rc.8", "5.0.0"},
		expRequests:   []string{"candidate-5.0"},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			endpoint, requestedChannels := newChannelGraphServer(t, test.graphs, test.failing...)
			cs := CincinnatiSchema{Log: clog.New("trace"), Client: &mockClient{url: endpoint}, CincinnatiParams: CincinnatiParams{Arch: "test-arch", GraphDataDir: t.TempDir()}}

			cur, req, updates, err := CalculateUpgrades(context.Background(), cs, test.sourceChannel, test.targetChannel, semver.MustParse(test.curr), semver.MustParse(test.req))
			if test.err == "" {
				require.NoError(t, err)
				require.Equal(t, testUpdate(test.expCurrent), cur)
				require.Equal(t, testUpdate(test.expRequested), req)
				require.Equal(t, testUpdates(test.expUpdates...), updates)
			} else {
				require.EqualError(t, err, test.err)
			}
			require.Equal(t, test.expRequests, requestedChannels())
		})
	}
}

// OCPBUGS-136257: same major version channels are still walked minor version by minor version.
func TestCalculateUpgradesSameMajorControl(t *testing.T) {
	endpoint, requestedChannels := newChannelGraphServer(t, map[string]string{
		"candidate-4.21": testGraph(t, []string{"4.21.0", "4.21.41"}, [][2]int{{0, 1}}),
		"candidate-4.22": testGraph(t, []string{"4.21.41", "4.22.21"}, [][2]int{{0, 1}}),
	})
	cs := CincinnatiSchema{Log: clog.New("trace"), Client: &mockClient{url: endpoint}, CincinnatiParams: CincinnatiParams{Arch: "test-arch", GraphDataDir: t.TempDir()}}

	cur, req, updates, err := CalculateUpgrades(context.Background(), cs, "candidate-4.21", "candidate-4.22", semver.MustParse("4.21.0"), semver.MustParse("4.22.21"))
	require.NoError(t, err)
	require.Equal(t, testUpdate("4.21.0"), cur)
	require.Equal(t, testUpdate("4.22.21"), req)
	require.Equal(t, testUpdates("4.21.0", "4.21.41", "4.22.21"), updates)
	require.Equal(t, []string{"candidate-4.21", "candidate-4.21", "candidate-4.22", "candidate-4.22"}, requestedChannels())
}

// OCPBUGS-136257: a disk to mirror run repeats the cross major version calculation from the
// graph data stored on disk by the mirror to disk run.
func TestCalculateUpgradesCrossMajorDiskToMirror(t *testing.T) {
	endpoint, requestedChannels := newChannelGraphServer(t, map[string]string{
		"candidate-5.0": testGraph(t, []string{"4.22.20", "4.22.21", "5.0.0"}, [][2]int{{0, 1}, {1, 2}}),
		"stable-5.0":    testGraph(t, []string{"4.22.1", "4.22.2", "5.0.0"}, [][2]int{{0, 1}, {1, 2}}),
		"stable-4.22":   testGraph(t, []string{"4.21.1", "4.22.1", "4.22.2"}, [][2]int{{0, 1}, {1, 2}}),
		"stable-5.1":    testGraph(t, []string{"5.0.0", "5.1.0"}, [][2]int{{0, 1}}),
	})

	for _, tc := range []struct{ source, target, curr, req string }{
		{"candidate-4.22", "candidate-5.0", "4.22.20", "5.0.0"},
		{"stable-4.21", "stable-5.0", "4.21.1", "5.0.0"},
		{"stable-4.22", "stable-5.1", "4.22.1", "5.1.0"},
	} {
		t.Run(tc.source+"_"+tc.target, func(t *testing.T) {
			cs := CincinnatiSchema{Log: clog.New("trace"), Client: &mockClient{url: endpoint}, CincinnatiParams: CincinnatiParams{Arch: "test-arch", GraphDataDir: t.TempDir()}}
			cs.Opts.Mode = mirror.MirrorToDisk
			m2dCur, m2dReq, m2dUpdates, err := CalculateUpgrades(context.Background(), cs, tc.source, tc.target, semver.MustParse(tc.curr), semver.MustParse(tc.req))
			require.NoError(t, err)
			requests := len(requestedChannels())

			d2m := cs
			d2m.Opts.Mode = mirror.DiskToMirror
			d2mCur, d2mReq, d2mUpdates, err := CalculateUpgrades(context.Background(), d2m, tc.source, tc.target, semver.MustParse(tc.curr), semver.MustParse(tc.req))
			require.NoError(t, err)
			require.Equal(t, m2dCur, d2mCur)
			require.Equal(t, m2dReq, d2mReq)
			require.Equal(t, m2dUpdates, d2mUpdates)
			require.Len(t, requestedChannels(), requests, "disk to mirror must not query the Cincinnati API")
		})
	}
}

// newChannelGraphServer mocks a Cincinnati API serving the given graphs by channel name.
// Unknown channels are served as empty graphs and failing channels answer with an internal
// server error. It returns the server URL and a function listing the requested channels in order.
func newChannelGraphServer(t *testing.T, graphs map[string]string, failing ...string) (*url.URL, func() []string) {
	t.Helper()
	emptyGraph := testGraph(t, nil, nil)

	var mu sync.Mutex
	var requested []string
	handler := func(w http.ResponseWriter, r *http.Request) {
		channel := r.URL.Query().Get("channel")
		mu.Lock()
		requested = append(requested, channel)
		mu.Unlock()

		for _, ch := range failing {
			if ch == channel {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		body, ok := graphs[channel]
		if !ok {
			body = emptyGraph
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}

	ts := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(ts.Close)
	endpoint, err := url.Parse(ts.URL)
	require.NoError(t, err)

	return endpoint, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), requested...)
	}
}

// testGraph returns the JSON representation of a Cincinnati graph with the given versions as
// nodes and edges between node indexes.
func testGraph(t *testing.T, versions []string, edges [][2]int) string {
	t.Helper()
	type node struct {
		Version string `json:"version"`
		Payload string `json:"payload"`
	}
	graph := struct {
		Nodes []node   `json:"nodes"`
		Edges [][2]int `json:"edges"`
	}{Nodes: []node{}, Edges: [][2]int{}}
	for _, v := range versions {
		graph.Nodes = append(graph.Nodes, node{Version: v, Payload: testUpdate(v).Image})
	}
	graph.Edges = append(graph.Edges, edges...)

	data, err := json.Marshal(graph)
	require.NoError(t, err)
	return string(data)
}

// testUpdate returns the update of the release version, or an empty update for an empty version.
func testUpdate(version string) cincinnati.Update {
	if version == "" {
		return cincinnati.Update{}
	}
	return cincinnati.Update{Version: semver.MustParse(version), Image: "quay.io/openshift-release-dev/ocp-release:" + version}
}

// testUpdates returns the updates of the release versions.
func testUpdates(versions ...string) []cincinnati.Update {
	updates := make([]cincinnati.Update, 0, len(versions))
	for _, v := range versions {
		updates = append(updates, testUpdate(v))
	}
	return updates
}
