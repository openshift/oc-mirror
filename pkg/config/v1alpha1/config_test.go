package v1alpha1

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// TODO(estroz): expected config.
	type spec struct {
		name      string
		file      string
		inline    string
		assertion require.ErrorAssertionFunc
	}

	specs := []spec{
		{
			name:      "Valid/Basic",
			file:      filepath.Join("testdata", "config", "valid.yaml"),
			assertion: require.NoError,
		},
		{
			name: "Invalid/BadStructure",
			inline: `
{
	"apiVersion": "tmp-redhatgov.com/v1alpha1",
	"kind": "Metadata",
	"pastMirrors": [
		{
			"timestamp": 1624543862,
			"sequence": 1,
			"uid": "360a43c2-8a14-4b5d-906b-07491459f25f",
			"foo": "bar"
		}
	]
}
`,
			assertion: require.Error,
		},
	}

	for _, s := range specs {
		t.Run(s.name, func(t *testing.T) {
			data := []byte(s.inline)
			if len(data) == 0 {
				var err error
				data, err = ioutil.ReadFile(s.file)
				require.NoError(t, err)
			}

			_, err := LoadConfig(data)
			s.assertion(t, err)
		})
	}
}
