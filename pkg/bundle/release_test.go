package bundle

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
)

type MockDoType func(req *http.Request) (*http.Response, error)

type MockClient struct {
	MockDo MockDoType
}

func (m *MockClient) Do(req *http.Request) (*http.Response, error) {
	return m.MockDo(req)
}

func Test_GetChannelLatest(t *testing.T) {

	client, upstream, err := newClient()
	if err != nil {
		t.Error("newClient failed", err)
	}

	ctx := context.Background()

	tests := []struct {
		name         string
		channel      string
		responseFile string
		responseCode int
	}{
		{
			name:         "test valid response",
			channel:      "stable-test",
			responseFile: "../../test/release/testdata/updategraph.json",
			responseCode: 200,
		},
	}
	// Does not currently handle arch selection
	arch := "x86_64"

	for _, tc := range tests {
		// r := ioutil.NopCloser(bytes.NewReader([]byte(json)))
		b, err := os.ReadFile(tc.responseFile)
		if err == nil {
			r := ioutil.NopCloser(bytes.NewReader(b))

			HClient = &MockClient{
				MockDo: func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: tc.responseCode,
						Body:       r,
					}, nil
				},
			}

			latest, err := client.GetChannelLatest(ctx, upstream, arch, tc.channel)

			if err != nil {
				t.Error("GetChannelLatest failed", err)
			}
			t.Log("Latest: ", latest)

		} else {
			t.Error("Unable to read file: ", tc.responseFile, err)

		}

	}

}
