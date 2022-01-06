package cincinnati

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/google/uuid"
	_ "k8s.io/klog/v2" // integration tests set glog flags.
)

func TestGetUpdates(t *testing.T) {
	clientID := uuid.MustParse("01234567-0123-0123-0123-0123456789ab")
	arch := "test-arch"
	channelName := "test-channel"
	tests := []struct {
		name    string
		version string
		reqVer  string

		expectedQuery string
		current       Update
		requested     Update
		available     []Update
		err           string
	}{{
		name:          "one update available",
		version:       "4.0.0-4",
		reqVer:        "4.0.0-5",
		expectedQuery: "arch=test-arch&channel=test-channel&id=01234567-0123-0123-0123-0123456789ab&version=4.0.0-4",
		current:       Update{Version: semver.MustParse("4.0.0-4"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-4"},
		requested:     Update{Version: semver.MustParse("4.0.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-5"},
		available: []Update{
			{Version: semver.MustParse("4.0.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-5"},
		},
	}, {
		name:          "two updates available",
		version:       "4.0.0-5",
		reqVer:        "4.0.0-6",
		expectedQuery: "arch=test-arch&channel=test-channel&id=01234567-0123-0123-0123-0123456789ab&version=4.0.0-5",
		current:       Update{Version: semver.MustParse("4.0.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-5"},
		requested:     Update{Version: semver.MustParse("4.0.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-6"},
		available: []Update{
			{Version: semver.MustParse("4.0.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-6"},
		},
	}, {
		name:          "no updates available",
		version:       "4.0.0-0.okd-0",
		reqVer:        "4.0.0-0.okd-0",
		current:       Update{Version: semver.MustParse("4.0.0-0.okd-0"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-0.okd-0"},
		requested:     Update{Version: semver.MustParse("4.0.0-0.okd-0"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-0.okd-0"},
		expectedQuery: "arch=test-arch&channel=test-channel&id=01234567-0123-0123-0123-0123456789ab&version=4.0.0-0.okd-0",
	}, {
		name:          "unknown version current",
		version:       "4.0.0-3",
		reqVer:        "0.0.0",
		expectedQuery: "arch=test-arch&channel=test-channel&id=01234567-0123-0123-0123-0123456789ab&version=4.0.0-3",
		err:           "VersionNotFound: current version 4.0.0-3 not found in the \"test-channel\" channel",
	}, {
		name:          "unknown version requested",
		version:       "4.0.0-5",
		reqVer:        "4.0.0-7",
		expectedQuery: "arch=test-arch&channel=test-channel&id=01234567-0123-0123-0123-0123456789ab&version=4.0.0-5",
		err:           "VersionNotFound: requested version 4.0.0-7 not found in the \"test-channel\" channel",
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestQuery := make(chan string, 1)
			defer close(requestQuery)

			handler := getHandler(t, requestQuery)

			ts := httptest.NewServer(http.HandlerFunc(handler))
			t.Cleanup(ts.Close)

			c, uri, err := NewClient(ts.URL, clientID)
			if err != nil {
				t.Fatal(err)
			}

			current, requested, updates, err := c.GetUpdates(context.Background(), uri, arch, channelName, semver.MustParse(test.version), semver.MustParse(test.reqVer))
			if test.err == "" {
				if err != nil {
					t.Fatalf("expected nil error, got: %v", err)
				}
				if !reflect.DeepEqual(current, test.current) {
					t.Fatalf("expected current %v, got: %v", test.current, current)
				}
				if !reflect.DeepEqual(requested, test.requested) {
					t.Fatalf("expected current %v, got: %v", test.requested, requested)
				}
				if !reflect.DeepEqual(updates, test.available) {
					t.Fatalf("expected updates %v, got: %v", test.available, updates)
				}
			} else {
				if err == nil || err.Error() != test.err {
					t.Fatalf("expected err to be %s, got: %v", test.err, err)
				}
			}

			actualQuery := ""
			select {
			case actualQuery = <-requestQuery:
			default:
				t.Fatal("no request received at upstream URL")
			}
			expectedQueryValues, err := url.ParseQuery(test.expectedQuery)
			if err != nil {
				t.Fatalf("could not parse expected query: %v", err)
			}
			actualQueryValues, err := url.ParseQuery(actualQuery)
			if err != nil {
				t.Fatalf("could not parse acutal query: %v", err)
			}
			if e, a := expectedQueryValues, actualQueryValues; !reflect.DeepEqual(e, a) {
				t.Errorf("expected query to be %q, got: %q", e, a)
			}
		})
	}
}

func TestGetLatest(t *testing.T) {
	clientID := uuid.MustParse("01234567-0123-0123-0123-0123456789ab")
	arch := "test-arch"
	channelName := "test-channel"
	tests := []struct {
		name string

		expectedQuery string
		latest        semver.Version
		err           string
	}{{
		name:          "one update available",
		expectedQuery: "arch=test-arch&channel=test-channel&id=01234567-0123-0123-0123-0123456789ab",
		latest:        semver.MustParse("4.0.0-6+2"),
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestQuery := make(chan string, 1)
			defer close(requestQuery)

			handler := getHandler(t, requestQuery)

			ts := httptest.NewServer(http.HandlerFunc(handler))
			t.Cleanup(ts.Close)

			c, uri, err := NewClient(ts.URL, clientID)
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}

			latest, err := c.GetChannelLatest(context.Background(), uri, arch, channelName)
			if test.err == "" {
				if err != nil {
					t.Fatalf("expected nil error, got: %v", err)
				}
				if !reflect.DeepEqual(latest, test.latest) {
					t.Fatalf("expected version %v, got: %v", test.latest, latest)
				}
			} else {
				if err == nil || err.Error() != test.err {
					t.Fatalf("expected err to be %s, got: %v", test.err, err)
				}
			}

			actualQuery := ""
			select {
			case actualQuery = <-requestQuery:
			default:
				t.Fatal("no request received at upstream URL")
			}
			expectedQueryValues, err := url.ParseQuery(test.expectedQuery)
			if err != nil {
				t.Fatalf("could not parse expected query: %v", err)
			}
			actualQueryValues, err := url.ParseQuery(actualQuery)
			if err != nil {
				t.Fatalf("could not parse acutal query: %v", err)
			}
			if e, a := expectedQueryValues, actualQueryValues; !reflect.DeepEqual(e, a) {
				t.Errorf("expected query to be %q, got: %q", e, a)
			}
		})
	}
}

func TestGetVersions(t *testing.T) {
	clientID := uuid.MustParse("01234567-0123-0123-0123-0123456789ab")
	channelName := "test-channel"
	tests := []struct {
		name string

		expectedQuery string
		versions      []semver.Version
		err           string
	}{{
		name:          "one update available",
		expectedQuery: "channel=test-channel&id=01234567-0123-0123-0123-0123456789ab",
		versions:      getSemVers([]string{"4.0.0-0.2", "4.0.0-0.3", "4.0.0-0.okd-0", "4.0.0-4", "4.0.0-5", "4.0.0-6", "4.0.0-6+2"}),
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestQuery := make(chan string, 1)
			defer close(requestQuery)

			handler := getHandler(t, requestQuery)

			ts := httptest.NewServer(http.HandlerFunc(handler))
			t.Cleanup(ts.Close)

			c, uri, err := NewClient(ts.URL, clientID)
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}

			versions, err := c.GetVersions(context.Background(), uri, channelName)
			if test.err == "" {
				if err != nil {
					t.Fatalf("expected nil error, got: %v", err)
				}
				if !reflect.DeepEqual(versions, test.versions) {
					t.Fatalf("expected version %v, got: %v", test.versions, versions)
				}
			} else {
				if err == nil || err.Error() != test.err {
					t.Fatalf("expected err to be %s, got: %v", test.err, err)
				}
			}

			actualQuery := ""
			select {
			case actualQuery = <-requestQuery:
			default:
				t.Fatal("no request received at upstream URL")
			}
			expectedQueryValues, err := url.ParseQuery(test.expectedQuery)
			if err != nil {
				t.Fatalf("could not parse expected query: %v", err)
			}
			actualQueryValues, err := url.ParseQuery(actualQuery)
			if err != nil {
				t.Fatalf("could not parse acutal query: %v", err)
			}
			if e, a := expectedQueryValues, actualQueryValues; !reflect.DeepEqual(e, a) {
				t.Errorf("expected query to be %q, got: %q", e, a)
			}
		})
	}
}

func TestCalculateUpgrades(t *testing.T) {
	clientID := uuid.MustParse("01234567-0123-0123-0123-0123456789ab")
	arch := "test-arch"

	tests := []struct {
		name          string
		sourceChannel string
		targetChannel string
		last          semver.Version
		req           semver.Version
		current       Update
		requested     Update
		needed        []Update
		err           string
	}{{
		name:          "jump one channel",
		sourceChannel: "stable-4.0",
		targetChannel: "stable-4.1",
		last:          semver.MustParse("4.0.0-5"),
		req:           semver.MustParse("4.1.0-6"),
		current:       Update{Version: semver.MustParse("4.0.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-5"},
		requested:     Update{Version: semver.MustParse("4.1.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.1.0-6"},
		needed: []Update{
			{Version: semver.MustParse("4.0.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-6"},
			{Version: semver.MustParse("4.1.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.1.0-6"},
		},
	}, {
		name:          "jump two channel",
		sourceChannel: "stable-4.0",
		targetChannel: "stable-4.2",
		last:          semver.MustParse("4.0.0-5"),
		req:           semver.MustParse("4.2.0-3"),
		current:       Update{Version: semver.MustParse("4.0.0-5"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-5"},
		requested:     Update{Version: semver.MustParse("4.2.0-3"), Image: "quay.io/openshift-release-dev/ocp-release:4.2.0-3"},
		needed: []Update{
			{Version: semver.MustParse("4.0.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-6"},
			{Version: semver.MustParse("4.1.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.1.0-6"},
			{Version: semver.MustParse("4.2.0-3"), Image: "quay.io/openshift-release-dev/ocp-release:4.2.0-3"},
		},
	}, {
		name:          "no upgrade path",
		sourceChannel: "stable-4.1",
		targetChannel: "stable-4.2",
		last:          semver.MustParse("4.1.0-6"),
		req:           semver.MustParse("4.2.0-2"),
		current:       Update{Version: semver.MustParse("4.1.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.1.0-6"},
		requested:     Update{Version: semver.MustParse("4.2.0-2"), Image: "quay.io/openshift-release-dev/ocp-release:4.2.0-2"},
		needed:        nil,
	}, {
		name:          "no upgrade path and no version in channel",
		sourceChannel: "stable-4.2",
		targetChannel: "stable-4.3",
		last:          semver.MustParse("4.2.0-3"),
		req:           semver.MustParse("4.3.0"),
		current:       Update{Version: semver.MustParse("4.2.0-3"), Image: "quay.io/openshift-release-dev/ocp-release:4.2.0-3"},
		requested:     Update{Version: semver.MustParse("4.3.0"), Image: "quay.io/openshift-release-dev/ocp-release:4.3.0"},
		needed:        nil,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestQuery := make(chan string, 10)
			defer close(requestQuery)

			handler := getHandlerMulti(t, requestQuery)

			ts := httptest.NewServer(http.HandlerFunc(handler))
			t.Cleanup(ts.Close)

			c, uri, err := NewClient(ts.URL, clientID)
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}

			cur, req, updates, err := c.CalculateUpgrades(context.Background(), uri, arch, test.sourceChannel, test.targetChannel, test.last, test.req)

			if test.err == "" {
				if err != nil {
					t.Fatalf("expected nil error, got: %v", err)
				}
				if !reflect.DeepEqual(cur, test.current) {
					t.Fatalf("expected current %v, got: %v", test.current, cur)
				}
				if !reflect.DeepEqual(req, test.requested) {
					t.Fatalf("expected requested %v, got: %v", test.requested, req)
				}
				if !reflect.DeepEqual(updates, test.needed) {
					t.Fatalf("expected updates %v, got: %v", test.needed, updates)
				}
			} else {
				if err == nil || err.Error() != test.err {
					t.Fatalf("expected err to be %s, got: %v", test.err, err)
				}
			}
		})
	}
}

func TestNodeUnmarshalJSON(t *testing.T) {
	tests := []struct {
		raw []byte

		exp node
		err string
	}{{
		raw: []byte(`{
			"version": "4.0.0-5",
			"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-5",
			"metadata": {}
		  }`),

		exp: node{
			Version:  semver.MustParse("4.0.0-5"),
			Image:    "quay.io/openshift-release-dev/ocp-release:4.0.0-5",
			Metadata: map[string]string{},
		},
	}, {
		raw: []byte(`{
			"version": "4.0.0-0.1",
			"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.1",
			"metadata": {
			  "description": "This is the beta1 image based on the 4.0.0-0.nightly-2019-01-15-010905 build"
			}
		  }`),
		exp: node{
			Version: semver.MustParse("4.0.0-0.1"),
			Image:   "quay.io/openshift-release-dev/ocp-release:4.0.0-0.1",
			Metadata: map[string]string{
				"description": "This is the beta1 image based on the 4.0.0-0.nightly-2019-01-15-010905 build",
			},
		},
	}, {
		raw: []byte(`{
			"version": "v4.0.0-0.1",
			"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.1",
			"metadata": {
			  "description": "This is the beta1 image based on the 4.0.0-0.nightly-2019-01-15-010905 build"
			}
		  }`),
		err: `Invalid character(s) found in major number "v4"`,
	}, {
		raw: []byte(`{
			"version": "4-0-0+0.1",
			"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.1",
			"metadata": {
			  "description": "This is the beta1 image based on the 4.0.0-0.nightly-2019-01-15-010905 build"
			}
		  }
	  `),

		err: "No Major.Minor.Patch elements found",
	}}

	for idx, test := range tests {
		t.Run(fmt.Sprintf("#%d", idx), func(t *testing.T) {
			var n node
			err := json.Unmarshal(test.raw, &n)
			if test.err == "" {
				if err != nil {
					t.Fatalf("expecting nil error, got: %v", err)
				}
				if !reflect.DeepEqual(n, test.exp) {
					t.Fatalf("expecting %v got %v", test.exp, n)
				}
			} else {
				if err.Error() != test.err {
					t.Fatalf("expecting %s error, got: %v", test.err, err)
				}
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

func getHandler(t *testing.T, requestQuery chan<- string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case requestQuery <- r.URL.RawQuery:
		default:
			t.Fatalf("received multiple requests at upstream URL")
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		mtype := r.Header.Get("Accept")
		if mtype != GraphMediaType {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}

		_, err := w.Write([]byte(`{
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
				"version": "4.0.0-6+2",
				"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-6+2"
			  },
			  {
				"version": "4.0.0-0.okd-0",
				"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.okd-0"
			  },
			  {
				"version": "4.0.0-0.2",
				"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.2"
			  },
			  {
				"version": "4.0.0-0.3",
				"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.3"
			  }
			],
			"edges": [[0,1],[1,2],[1,3],[5,6]]
		  }`))
		if err != nil {
			t.Fatal(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func getHandlerMulti(t *testing.T, requestQuery chan<- string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case requestQuery <- r.URL.RawQuery:
		default:
			t.Fatalf("received multiple requests at upstream URL")
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		mtype := r.Header.Get("Accept")
		if mtype != GraphMediaType {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}

		keys, ok := r.URL.Query()["channel"]
		if !ok {
			t.Fail()
		}

		ch := keys[len(keys)-1]

		switch {
		case ch == "stable-4.0":
			_, err := w.Write([]byte(`{
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
					"version": "4.0.0-0.2",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.2"
				  },
				  {
					"version": "4.0.0-0.3",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.3"
				  }
				],
				"edges": [[0,1],[1,2],[4,5]]
			  }`))
			if err != nil {
				t.Fatal(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case ch == "stable-4.1":
			_, err := w.Write([]byte(`{
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
					"version": "4.0.0-0.2",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.2"
				  },
				  {
					"version": "4.0.0-0.3",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.3"
				  },
				  {
					"version": "4.1.0-6",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.1.0-6"
				  }
				],
				"edges": [[0,1],[1,2],[2,6],[4,5]]
			  }`))
			if err != nil {
				t.Fatal(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case ch == "stable-4.2":
			_, err := w.Write([]byte(`{
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
			}`))
			if err != nil {
				t.Fatal(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case ch == "stable-4.3":
			_, err := w.Write([]byte(`{
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
			}`))
			if err != nil {
				t.Fatal(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		default:
			t.Fail()
		}
	}
}
