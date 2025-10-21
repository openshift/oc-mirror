package describe

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/oc-mirror/pkg/cli"
)

func TestDescribeComplete(t *testing.T) {
	type spec struct {
		name     string
		opts     *DescribeOptions
		args     []string
		expOpts  *DescribeOptions
		expError string
	}

	cases := []spec{
		{
			name: "Valid/DefaultArchitecture",
			opts: &DescribeOptions{},
			args: []string{"foo"},
			expOpts: &DescribeOptions{
				From: "foo",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.opts.Complete(c.args)
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expOpts, c.opts)
			}
		})
	}
}

func TestDescribeValidate(t *testing.T) {

	type spec struct {
		name     string
		opts     *DescribeOptions
		expError string
	}

	cases := []spec{
		{
			name:     "Invalid/NoConfigPath",
			opts:     &DescribeOptions{},
			expError: `must specify path to imageset archive`,
		},
		{
			name: "Valid/WithArchivePath",
			opts: &DescribeOptions{
				From: "foo",
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

func TestDescribeRun(t *testing.T) {
	expOutput := `{
 "kind": "Metadata",
 "apiVersion": "mirror.openshift.io/v1alpha2",
 "uid": "360a43c2-8a14-4b5d-906b-07491459f25f",
 "singleUse": false,
 "pastMirror": {
  "timestamp": 0,
  "sequence": 0,
  "mirror": {
   "platform": {},
   "helm": {}
  }
 }
}
`
	outBuf := new(strings.Builder)
	eOutBuf := new(strings.Builder)
	rootOpts := &cli.RootOptions{
		IOStreams: genericclioptions.IOStreams{
			Out:    outBuf,
			In:     os.Stdin,
			ErrOut: eOutBuf,
		},
	}
	opts := &DescribeOptions{RootOptions: rootOpts}
	opts.From = "testdata"
	require.NoError(t, opts.Run(context.TODO()))
	require.Equal(t, expOutput, outBuf.String())
	require.Equal(t, eOutBuf.Len(), 0)
}
