package mirror

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/pkg/cincinnati"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha2"
)

func TestGetDownloads(t *testing.T) {
	opts := ReleaseOptions{}

	tests := []struct {
		name string

		channels []v1alpha2.ReleaseChannel
		expected downloads
		arch     []string
		version  string
		err      string
	}{{
		name: "Success/OneChannelOneArch",
		arch: []string{"test-arch"},
		channels: []v1alpha2.ReleaseChannel{
			{
				Name:       "stable-4.1",
				MinVersion: "4.0.0-4",
				MaxVersion: "4.1.0-6",
			},
		},
		expected: downloads{
			"quay.io/openshift-release-dev/ocp-release:4.0.0-4": struct{}{},
			"quay.io/openshift-release-dev/ocp-release:4.0.0-5": struct{}{},
			"quay.io/openshift-release-dev/ocp-release:4.0.0-6": struct{}{},
			"quay.io/openshift-release-dev/ocp-release:4.1.0-6": struct{}{},
		},
	}, {
		name: "Success/OneChannelShortestPath",
		channels: []v1alpha2.ReleaseChannel{
			{
				Name:         "stable-4.1",
				MinVersion:   "4.0.0-4",
				MaxVersion:   "4.1.0-6",
				ShortestPath: true,
			},
		},
		arch: []string{"test-arch"},
		expected: downloads{
			"quay.io/openshift-release-dev/ocp-release:4.0.0-4": struct{}{},
			"quay.io/openshift-release-dev/ocp-release:4.0.0-6": struct{}{},
			"quay.io/openshift-release-dev/ocp-release:4.1.0-6": struct{}{},
		},
	}, {
		name: "Success/MultiArch",
		channels: []v1alpha2.ReleaseChannel{
			{
				Name:       "stable-4.0",
				MinVersion: "4.0.0-5",
				MaxVersion: "4.0.0-6",
			},
		},
		arch: []string{"test-arch", "another-arch"},
		expected: downloads{
			"quay.io/openshift-release-dev/ocp-release:4.0.0-5":         struct{}{},
			"quay.io/openshift-release-dev/ocp-release:4.0.0-5-another": struct{}{},
			"quay.io/openshift-release-dev/ocp-release:4.0.0-6":         struct{}{},
			"quay.io/openshift-release-dev/ocp-release:4.0.0-6-another": struct{}{},
		},
	}, {
		name: "Failure/VersionStringEmpty",
		channels: []v1alpha2.ReleaseChannel{
			{
				Name:       "stable-4.0",
				MinVersion: "4.0.0-5",
			},
		},
		arch: []string{"test-arch"},
		err:  "Version string empty",
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestQuery := make(chan string, 10)
			defer close(requestQuery)

			handler := getHandlerMulti(t, requestQuery)

			ts := httptest.NewServer(http.HandlerFunc(handler))
			t.Cleanup(ts.Close)

			allDownloads := downloads{}
			var newDownloads downloads

			endpoint, err := url.Parse(ts.URL)
			require.NoError(t, err)
			c := &mockClient{url: endpoint}

			for _, ar := range test.arch {
				for _, ch := range test.channels {

					newDownloads, err = opts.getChannelDownloads(context.Background(), c, test.channels, ch, ar)
					if err != nil {
						break
					}
					allDownloads.Merge(newDownloads)
				}
			}

			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, allDownloads)
			}
		})
	}
}

// Create a mock client
type mockClient struct {
	url *url.URL
}

func (c mockClient) GetID() uuid.UUID {
	return uuid.MustParse("01234567-0123-0123-0123-0123456789ab")
}

func (c mockClient) SetQueryParams(arch, channel, version string) {
	queryParams := c.url.Query()
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

// Mock Cincinnati API
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
		if mtype != cincinnati.GraphMediaType {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}

		channels, ok := r.URL.Query()["channel"]
		if !ok {
			t.Fail()
		}

		ch := channels[len(channels)-1]

		arch, ok := r.URL.Query()["arch"]
		if !ok {
			t.Fail()
		}

		ar := arch[len(arch)-1]

		switch {
		case ch == "stable-4.0" && ar == "test-arch":
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
				"edges": [[0,1],[1,2],[2,4],[4,5]]
			  }`))
			if err != nil {
				t.Fatal(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case ch == "stable-4.0" && ar == "another-arch":
			_, err := w.Write([]byte(`{
				"nodes": [
				  {
					"version": "4.0.0-4",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-4-another"
				  },
				  {
					"version": "4.0.0-5",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-5-another"
				  },
				  {
					"version": "4.0.0-6",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-6-another"
				  },
				  {
					"version": "4.0.0-0.okd-0",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.okd-0-another"
				  },
				  {
					"version": "4.0.0-0.2",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.2-another"
				  },
				  {
					"version": "4.0.0-0.3",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.3-another"
				  }
				],
				"edges": [[0,1],[1,2],[4,5]]
			  }`))
			if err != nil {
				t.Fatal(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case ch == "stable-4.1" && ar == "test-arch":
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
				"edges": [[0,1],[0,2],[1,2],[2,6],[4,5]]
			  }`))
			if err != nil {
				t.Fatal(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case ch == "stable-4.1" && ar == "another-arch":
			_, err := w.Write([]byte(`{
				"nodes": [
				  {
					"version": "4.0.0-4",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-4-another"
				  },
				  {
					"version": "4.0.0-5",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-5-another"
				  },
				  {
					"version": "4.0.0-6",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-6-another"
				  },
				  {
					"version": "4.0.0-0.okd-0",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.okd-0-another"
				  },
				  {
					"version": "4.0.0-0.2",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.2-another"
				  },
				  {
					"version": "4.0.0-0.3",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.3-another"
				  },
				  {
					"version": "4.1.0-6",
					"payload": "quay.io/openshift-release-dev/ocp-release:4.1.0-6-another"
				  }
				],
				"edges": [[0,1],[1,2],[2,6],[4,5]]
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
