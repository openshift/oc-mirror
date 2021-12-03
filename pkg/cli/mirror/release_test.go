package mirror

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/google/uuid"
	"github.com/openshift/oc-mirror/pkg/cincinnati"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/stretchr/testify/require"
)

func Test_getDownloads(t *testing.T) {
	clientID := uuid.MustParse("01234567-0123-0123-0123-0123456789ab")

	opts := ReleaseOptions{
		uuid: clientID,
	}

	tests := []struct {
		name string

		channels []v1alpha1.ReleaseChannel
		expected downloads
		channel  string
		arch     []string
		version  string
		err      string
	}{{
		name: "jump one channel and one arch",
		arch: []string{"test-arch"},
		channels: []v1alpha1.ReleaseChannel{
			{
				Name:     "stable-4.0",
				Versions: []string{"4.0.0-5"},
			},
		},
		channel: "stable-4.1",
		version: "4.1.0-6",
		expected: downloads{
			"quay.io/openshift-release-dev/ocp-release:4.0.0-6": download{
				Update: cincinnati.Update{Version: semver.MustParse("4.0.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-6"},
				arch:   "test-arch",
			},
			"quay.io/openshift-release-dev/ocp-release:4.1.0-6": download{
				Update: cincinnati.Update{Version: semver.MustParse("4.1.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.1.0-6"},
				arch:   "test-arch",
			},
		},
	}, {
		name: "reverse",
		channels: []v1alpha1.ReleaseChannel{
			{
				Name:     "stable-4.1",
				Versions: []string{"4.1.0-6"},
			},
		},
		arch:    []string{"test-arch"},
		channel: "stable-4.0",
		version: "4.0.0-6",
		expected: downloads{
			"quay.io/openshift-release-dev/ocp-release:4.0.0-6": download{
				Update: cincinnati.Update{Version: semver.MustParse("4.0.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-6"},
				arch:   "test-arch",
			},
		},
	}, {
		name: "multi-arch",
		channels: []v1alpha1.ReleaseChannel{
			{
				Name:     "stable-4.0",
				Versions: []string{"4.0.0-5"},
			},
		},
		arch:    []string{"test-arch", "another-arch"},
		channel: "stable-4.0",
		version: "4.0.0-6",
		expected: downloads{
			"quay.io/openshift-release-dev/ocp-release:4.0.0-6": download{
				Update: cincinnati.Update{Version: semver.MustParse("4.0.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-6"},
				arch:   "test-arch",
			},
			"quay.io/openshift-release-dev/ocp-release:4.0.0-6-another": download{
				Update: cincinnati.Update{Version: semver.MustParse("4.0.0-6"), Image: "quay.io/openshift-release-dev/ocp-release:4.0.0-6-another"},
				arch:   "another-arch",
			},
		},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestQuery := make(chan string, 10)
			defer close(requestQuery)

			handler := getHandlerMulti(t, requestQuery)

			ts := httptest.NewServer(http.HandlerFunc(handler))
			defer ts.Close()

			c, uri, err := cincinnati.NewClient(ts.URL, clientID)
			if err != nil {
				t.Fatal(err)
			}
			if err != nil {
				t.Fatal(err)
			}

			meta := v1alpha1.Metadata{
				MetadataSpec: v1alpha1.MetadataSpec{
					PastMirrors: []v1alpha1.PastMirror{
						{
							Mirror: v1alpha1.Mirror{
								OCP: v1alpha1.OCP{
									Graph:    false,
									Channels: test.channels,
								},
							},
						},
					},
				},
			}

			allDownloads := downloads{}

			for _, ar := range test.arch {

				downloads, err := opts.getDownloads(context.Background(), c, meta, test.version, test.channel, ar, uri)
				if err != nil {
					require.NoError(t, err)
				}
				allDownloads.Merge(downloads)
			}

			if test.err == "" {
				if err != nil {
					require.NoError(t, err)
				}
				if !reflect.DeepEqual(allDownloads, test.expected) {
					t.Fatalf("expected current %v, got: %v", test.expected, allDownloads)
				}
			} else {
				if err == nil || err.Error() != test.err {
					t.Fatalf("expected err to be %s, got: %v", test.err, err)
				}
			}
		})
	}
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
				"edges": [[0,1],[1,2],[4,5]]
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
				"edges": [[0,1],[1,2],[2,6],[4,5]]
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
