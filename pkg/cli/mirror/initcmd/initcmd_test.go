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
	promptStderr := `Enter custom registry image URL or blank for none.
Example: localhost:5000/test:latest
`
	cases := []spec{
		{
			name: "Default yaml output",
			opts: &InitOptions{
				RootOptions: &cli.RootOptions{},
				Output:      "yaml",
			},
			expError: "",
			stdIn:    "\n",
			expStdout: `kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v1alpha2
storageConfig:
  local:
    path: ./
mirror:
  platform:
    channels:
    - name: stable-4.1
      type: ocp
  operators:
  - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.11
    packages:
    - name: serverless-operator
      startingVersion: 0.0.0
      channels:
      - name: stable
        startingVersion: 0.0.0
  additionalImages:
  - name: registry.redhat.io/ubi8/ubi:latest
  helm: {}
`,
			expStderr: promptStderr,
		},
		{
			name: "Default json output",
			opts: &InitOptions{
				RootOptions: &cli.RootOptions{},
				Output:      "json",
			},
			expError: "",
			stdIn:    "\n",
			expStdout: `{
  "kind": "ImageSetConfiguration",
  "apiVersion": "mirror.openshift.io/v1alpha2",
  "mirror": {
    "platform": {
      "channels": [
        {
          "name": "stable-4.1",
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
                "name": "stable",
                "startingVersion": "0.0.0"
              }
            ],
            "startingVersion": "0.0.0"
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
			expStderr: promptStderr,
		},
		{
			name: "Custom registry yaml output",
			opts: &InitOptions{
				RootOptions: &cli.RootOptions{},
				Output:      "yaml",
			},
			expError: "",
			stdIn:    "localhost:5000/test:latest\n",
			expStdout: `kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v1alpha2
storageConfig:
  registry:
    imageURL: localhost:5000/test:latest
    skipTLS: false
  local:
    path: ./
mirror:
  platform:
    channels:
    - name: stable-4.1
      type: ocp
  operators:
  - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.11
    packages:
    - name: serverless-operator
      startingVersion: 0.0.0
      channels:
      - name: stable
        startingVersion: 0.0.0
  additionalImages:
  - name: registry.redhat.io/ubi8/ubi:latest
  helm: {}
`,
			expStderr: promptStderr,
		},
		{
			name: "Custom registry json output",
			opts: &InitOptions{
				RootOptions: &cli.RootOptions{},
				Output:      "json",
			},
			expError: "",
			stdIn:    "localhost:5000/test:latest\n",
			expStdout: `{
  "kind": "ImageSetConfiguration",
  "apiVersion": "mirror.openshift.io/v1alpha2",
  "mirror": {
    "platform": {
      "channels": [
        {
          "name": "stable-4.1",
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
                "name": "stable",
                "startingVersion": "0.0.0"
              }
            ],
            "startingVersion": "0.0.0"
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
    },
    "local": {
      "path": "./"
    }
  }
}
`,
			expStderr: promptStderr,
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
			ctx := context.WithValue(context.TODO(), "catalogBase", catalog)

			c.opts.In = strings.NewReader(c.stdIn)
			outBuf := new(strings.Builder)
			errOutBuf := new(strings.Builder)
			c.opts.Out = outBuf
			c.opts.ErrOut = errOutBuf
			err = c.opts.Run(ctx)
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
