package v1alpha2

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadMetadata(t *testing.T) {
	// TODO(estroz): expected metadata.
	type spec struct {
		name      string
		file      string
		inline    string
		assertion require.ErrorAssertionFunc
	}

	specs := []spec{
		{
			name:      "Valid/Basic",
			file:      filepath.Join("testdata", "metadata", "valid.json"),
			assertion: require.NoError,
		},
		{
			name: "Invalid/BadStructure",
			inline: `---
apiVersion: mirror.openshift.io/v1alpha2
kind: ImageSetConfiguration
foo: bar
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

			_, err := LoadMetadata(data)
			s.assertion(t, err)
		})
	}
}
