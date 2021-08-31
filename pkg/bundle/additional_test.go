package bundle

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

// TODO: use some oc lib to mock image mirroring, or mirror from files.

func Test_GetAdditional(t *testing.T) {

	mirror := v1alpha1.PastMirror{}
	cfg := v1alpha1.ImageSetConfiguration{}
	cfg.Mirror = v1alpha1.Mirror{
		BlockedImages: []v1alpha1.BlockedImages{
			{Image: v1alpha1.Image{Name: "quay.io/estroz/pull-tester-blocked"}},
		},
		AdditionalImages: []v1alpha1.AdditionalImages{
			{Image: v1alpha1.Image{Name: "quay.io/estroz/pull-tester-additional:latest"}},
		},
	}

	tmpdir := t.TempDir()

	ro := cli.RootOptions{
		Dir: tmpdir,
		IOStreams: genericclioptions.IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
	}
	opts := NewAdditionalOptions(ro)

	assocs, err := opts.GetAdditional(mirror, cfg)
	require.NoError(t, err)
	if assert.Len(t, assocs, 1) {
		require.Contains(t, assocs, "quay.io/estroz/pull-tester-additional:latest")
	}
}
