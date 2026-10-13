package v1alpha2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseImageReference(t *testing.T) {
	type spec struct {
		desc          string
		input         string
		expRegistry   string
		expNamespace  string
		expRepository string
		expDigest     string
		expTag        string
	}

	specs := []spec{
		{
			desc:          "remote image with both tag and digest",
			input:         "cp.icr.io/cp/cpd/postgresql:13.7@sha256:e05434bfdb4b306fbc2e697112e1343907e368eb5f348c1779562d31b9f32ac5",
			expRegistry:   "cp.icr.io",
			expNamespace:  "cp/cpd",
			expRepository: "postgresql",
			expDigest:     "e05434bfdb4b306fbc2e697112e1343907e368eb5f348c1779562d31b9f32ac5",
			expTag:        "13.7",
		},
		{
			desc:          "remote image with tag",
			input:         "quay.io/redhatgov/oc-mirror-dev:foo-bundle-v0.3.1",
			expRegistry:   "quay.io",
			expNamespace:  "redhatgov",
			expRepository: "oc-mirror-dev",
			expDigest:     "",
			expTag:        "foo-bundle-v0.3.1",
		},
		{
			desc:          "remote image with digest",
			input:         "quay.io/redhatgov/oc-mirror-dev@sha256:7e1e74b87a503e95db5203334917856f61aece90a72e8d53a9fd903344eb78a5",
			expRegistry:   "quay.io",
			expNamespace:  "redhatgov",
			expRepository: "oc-mirror-dev",
			expDigest:     "7e1e74b87a503e95db5203334917856f61aece90a72e8d53a9fd903344eb78a5",
			expTag:        "",
		},
	}

	for _, s := range specs {
		t.Run(s.desc, func(t *testing.T) {
			reg, ns, repo, tag, id := ParseImageReference(s.input)
			require.Equal(t, s.expRegistry, reg)
			require.Equal(t, s.expNamespace, ns)
			require.Equal(t, s.expRepository, repo)
			require.Equal(t, s.expTag, tag)
			require.Equal(t, s.expDigest, id)
		})
	}
}
func TestGetUniqueName(t *testing.T) {
	type spec struct {
		desc     string
		ref      Operator
		exp      string
		expError string
	}

	specs := []spec{
		{
			desc: "simple flow",
			ref: Operator{
				Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.12",
				IncludeConfig: IncludeConfig{
					Packages: []IncludePackage{
						{
							Name: "aws-load-balancer-operator",
						},
					},
				},
			},
			exp:      "registry.redhat.io/redhat/redhat-operator-index:v4.12",
			expError: "",
		},
		{
			desc: "simple flow with targetCatalog",
			ref: Operator{
				Catalog:       "registry.redhat.io/redhat/redhat-operator-index:v4.12",
				TargetCatalog: "def/redhat-operator-index",
				IncludeConfig: IncludeConfig{
					Packages: []IncludePackage{
						{
							Name: "aws-load-balancer-operator",
						},
					},
				},
			},
			exp:      "registry.redhat.io/def/redhat-operator-index:v4.12",
			expError: "",
		},
		{
			desc: "OCI FBC flow",
			ref: Operator{
				Catalog:       "oci:///home/myuser/random/folder/redhat-operator-index:v4.12",
				TargetCatalog: "def/redhat-operator-index",
				IncludeConfig: IncludeConfig{
					Packages: []IncludePackage{
						{
							Name: "aws-load-balancer-operator",
						},
					},
				},
			},
			exp:      "def/redhat-operator-index:v4.12",
			expError: "",
		},
	}

	for _, s := range specs {
		t.Run(s.desc, func(t *testing.T) {
			un, err := s.ref.GetUniqueName()
			if s.expError != "" {
				require.EqualError(t, err, s.expError)
			} else {
				require.NoError(t, err)
				require.Equal(t, s.exp, un)
			}
		})
	}
}
