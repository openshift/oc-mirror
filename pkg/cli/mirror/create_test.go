package mirror

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/cli"
)

func TestCreate(t *testing.T) {
	path := t.TempDir()
	ctx := context.Background()

	img := v1alpha2.Image{Name: "quay.io/redhatgov/oc-mirror-dev:latest"}

	cfg := v1alpha2.ImageSetConfiguration{}
	cfg.Mirror.AdditionalImages = append(cfg.Mirror.AdditionalImages, img)

	opts := MirrorOptions{
		RootOptions: &cli.RootOptions{
			Dir:      path,
			LogLevel: 2,
			IOStreams: genericclioptions.IOStreams{
				In:     os.Stdin,
				Out:    os.Stdout,
				ErrOut: os.Stderr,
			},
		},
		OutputDir: path,
	}
	_, mappings, _, err := opts.Create(ctx, cfg)
	require.NoError(t, err)
	require.Len(t, mappings, 1)
}
