package v1alpha2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
