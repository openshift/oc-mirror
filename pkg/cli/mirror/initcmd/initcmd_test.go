package initcmd

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/internal/testutils"
	"github.com/openshift/oc-mirror/pkg/cli"
)

func TestInitValidate(t *testing.T) {

	type spec struct {
		name     string
		opts     *InitOptions
		expError string
	}

	cases := []spec{
		{
			name: "Invalid/InvalidOutput",
			opts: &InitOptions{
				Output: "invalid",
			},
			expError: "--output must be 'yaml' or 'json'",
		},
		{
			name: "Valid/YAMLOutput",
			opts: &InitOptions{
				Output: "yaml",
			},
			expError: "",
		},
		{
			name: "Valid/JSONOutput",
			opts: &InitOptions{
				Output: "json",
			},
			expError: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.opts.Validate()
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestInitOptions_Run(t *testing.T) {
	type spec struct {
		name      string
		opts      *InitOptions
		expError  string
		stdIn     string
		expStdout string
		expStderr string
	}
	cases := []spec{
		{
			name: "Default yaml output",
			opts: &InitOptions{
				RootOptions: &cli.RootOptions{},
				Output:      "yaml",
			},
			expError: "",
			expStdout: `kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v1alpha2
storageConfig:
  local:
    path: ./
mirror:
  platform:
    channels:
    - name: stable-0.0
      type: ocp
  operators:
  - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.11
    packages:
    - name: serverless-operator
      channels:
      - name: stable
  additionalImages:
  - name: registry.redhat.io/ubi8/ubi:latest
  helm: {}
`,
		},
		{
			name: "Default json output",
			opts: &InitOptions{
				RootOptions: &cli.RootOptions{},
				Output:      "json",
			},
			expError: "",
			expStdout: `{
  "kind": "ImageSetConfiguration",
  "apiVersion": "mirror.openshift.io/v1alpha2",
  "mirror": {
    "platform": {
      "channels": [
        {
          "name": "stable-0.0",
          "type": "ocp"
        }
      ]
    },
    "operators": [
      {
        "packages": [
          {
            "name": "serverless-operator",
            "channels": [
              {
                "name": "stable"
              }
            ]
          }
        ],
        "catalog": "registry.redhat.io/redhat/redhat-operator-index:v4.11"
      }
    ],
    "additionalImages": [
      {
        "name": "registry.redhat.io/ubi8/ubi:latest"
      }
    ],
    "helm": {}
  },
  "storageConfig": {
    "local": {
      "path": "./"
    }
  }
}
`,
		},
		{
			name: "Default yaml output",
			opts: &InitOptions{
				RootOptions: &cli.RootOptions{},
				Output:      "yaml",
			},
			expError: "",
			expStdout: `kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v1alpha2
storageConfig:
  local:
    path: ./
mirror:
  platform:
    channels:
    - name: stable-0.0
      type: ocp
  operators:
  - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.11
    packages:
    - name: serverless-operator
      channels:
      - name: stable
  additionalImages:
  - name: registry.redhat.io/ubi8/ubi:latest
  helm: {}
`,
		},
		{
			name: "Custom json output",
			opts: &InitOptions{
				RootOptions: &cli.RootOptions{},
				Registry:    "localhost:5000/test:latest",
				Output:      "json",
			},
			expError: "",
			expStdout: `{
  "kind": "ImageSetConfiguration",
  "apiVersion": "mirror.openshift.io/v1alpha2",
  "mirror": {
    "platform": {
      "channels": [
        {
          "name": "stable-0.0",
          "type": "ocp"
        }
      ]
    },
    "operators": [
      {
        "packages": [
          {
            "name": "serverless-operator",
            "channels": [
              {
                "name": "stable"
              }
            ]
          }
        ],
        "catalog": "registry.redhat.io/redhat/redhat-operator-index:v4.11"
      }
    ],
    "additionalImages": [
      {
        "name": "registry.redhat.io/ubi8/ubi:latest"
      }
    ],
    "helm": {}
  },
  "storageConfig": {
    "registry": {
      "imageURL": "localhost:5000/test:latest",
      "skipTLS": false
    }
  }
}
`,
		},
		{
			name: "Custom yaml output",
			opts: &InitOptions{
				RootOptions: &cli.RootOptions{},
				Registry:    "localhost:5000/test:latest",
				Output:      "yaml",
			},
			expError: "",
			expStdout: `kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v1alpha2
storageConfig:
  registry:
    imageURL: localhost:5000/test:latest
    skipTLS: false
mirror:
  platform:
    channels:
    - name: stable-0.0
      type: ocp
  operators:
  - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.11
    packages:
    - name: serverless-operator
      channels:
      - name: stable
  additionalImages:
  - name: registry.redhat.io/ubi8/ubi:latest
  helm: {}
`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {

			server := httptest.NewServer(testutils.RegistryFromFiles("../testdata"))
			t.Cleanup(server.Close)
			u, err := url.Parse(server.URL)
			if err != nil {
				t.Error(err)
			}
			catalog := fmt.Sprintf("%s/redhat/redhat-operator-index", u.Host)
			c.opts.catalogBase = catalog

			c.opts.In = strings.NewReader(c.stdIn)
			outBuf := new(strings.Builder)
			errOutBuf := new(strings.Builder)
			c.opts.Out = outBuf
			c.opts.ErrOut = errOutBuf
			err = c.opts.Run(context.TODO())
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
				out := outBuf.String()
				errOut := errOutBuf.String()
				// The example registry is localhost, and u.Host is always 127.0.0.1, so this should be fine
				out = strings.Replace(out, u.Host, "registry.redhat.io", -1)
				require.Equal(t, c.expStdout, out)
				require.Equal(t, c.expStderr, errOut)
			}
		})
	}

}
