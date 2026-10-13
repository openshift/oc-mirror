package cincinnati

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

func TestDownloadGraphData(t *testing.T) {
	log := clog.New("debug")
	t.Run("should succeed when custom url is valid", func(t *testing.T) {
		const channel string = "fast-4.20"
		serverReached := make(chan struct{}, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serverReached <- struct{}{}
			if r.Header.Get("Accept") != "application/json" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"kind":"invalid_content_type","value":"invalid Content-Type requested"}`)
				return
			}
			query := r.URL.Query()
			if !query.Has("channel") {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"kind":"missing_params","value":"mandatory client parameters missing: channel"}`)
				return
			}
			assert.Equal(t, query.Get("channel"), channel)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"version":1,"nodes":[],"edges":[],"conditionalEdges":[]}`)
		}))
		defer srv.Close()

		_, err := DownloadGraphData(t.Context(), log, WithChannel(channel), WithURL(srv.URL))
		assert.NoError(t, err)
		select {
		case <-serverReached:
		default:
			assert.FailNow(t, "did not contact mock server")
		}
	})
	t.Run("should fail when custom URL is not reachable", func(t *testing.T) {
		_, err := DownloadGraphData(t.Context(), log, WithURL("http://localhost:9999/v1/graph"), WithChannel("channel"))
		assert.ErrorContains(t, err, "request failed")
	})
}

func TestBuildOptions(t *testing.T) {
	t.Run("should succeed when ", func(t *testing.T) {
		t.Run("setting valid parameters", func(t *testing.T) {
			const idValue string = "01234567-0123-0123-0123-0123456789ab"
			id, err := uuid.Parse(idValue)
			assert.NoError(t, err)
			opts := []Option{
				WithArch("arch"),
				WithChannel("channel"),
				WithID(id),
				WithURL("quay.io/release/ocp-release"),
				WithVersion("version"),
			}
			o, err := makeOptions(opts...)
			assert.NoError(t, err)
			assert.Equal(t, "arch", o.arch)
			assert.Equal(t, "channel", o.channel)
			assert.Equal(t, id, o.id)
			assert.Equal(t, "quay.io/release/ocp-release", o.cincinnatiURL.String())
			assert.Equal(t, "version", o.version)
		})
		t.Run("when custom URL not specified", func(t *testing.T) {
			o, err := makeOptions(WithChannel("channel"))
			assert.NoError(t, err)
			assert.Equal(t, "channel", o.channel)
			assert.Equal(t, OcpUpdateURL, o.cincinnatiURL.String())
		})
	})
	t.Run("should fail when", func(t *testing.T) {
		t.Run("invalid url parameter", func(t *testing.T) {
			o := options{}
			fn := WithURL("://foobar//")
			err := fn(&o)
			assert.ErrorContains(t, err, "failed to parse url")
		})
		t.Run("channel is not set", func(t *testing.T) {
			_, err := makeOptions()
			assert.ErrorContains(t, err, "channel value must be set")
		})
	})
}
