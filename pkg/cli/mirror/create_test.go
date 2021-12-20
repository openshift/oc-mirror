package mirror

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestAddOPMImage(t *testing.T) {

	var cfg v1alpha1.ImageSetConfiguration
	var meta v1alpha1.Metadata

	// No past OPMImage.
	cfg = v1alpha1.ImageSetConfiguration{}
	meta = v1alpha1.Metadata{}
	meta.MetadataSpec.PastMirrors = []v1alpha1.PastMirror{
		{
			Mirror: v1alpha1.Mirror{
				AdditionalImages: []v1alpha1.AdditionalImages{
					{Image: v1alpha1.Image{Name: "reg.com/ns/other:latest"}},
				},
			},
		},
	}

	addOPMImage(&cfg, meta)
	if assert.Len(t, cfg.Mirror.AdditionalImages, 1) {
		require.Equal(t, cfg.Mirror.AdditionalImages[0].Image.Name, OPMImage)
	}

	// Has past OPMImage.
	cfg = v1alpha1.ImageSetConfiguration{}
	meta = v1alpha1.Metadata{}
	meta.MetadataSpec.PastMirrors = []v1alpha1.PastMirror{
		{
			Mirror: v1alpha1.Mirror{
				AdditionalImages: []v1alpha1.AdditionalImages{
					{Image: v1alpha1.Image{Name: OPMImage}},
					{Image: v1alpha1.Image{Name: "reg.com/ns/other:latest"}},
				},
			},
		},
	}

	addOPMImage(&cfg, meta)
	require.Len(t, cfg.Mirror.AdditionalImages, 0)
}

func Test_Create(t *testing.T) {
	path := t.TempDir()
	ctx := context.Background()

	opts := MirrorOptions{
		RootOptions: &cli.RootOptions{
			Dir:      path,
			LogLevel: "info",
			IOStreams: genericclioptions.IOStreams{
				In:     os.Stdin,
				Out:    os.Stdout,
				ErrOut: os.Stderr,
			},
		},
		ConfigPath: "testdata/configs/test.yaml",
		OutputDir:  path,
	}
	flags := NewMirrorCmd().Flags()
	err := opts.Create(ctx, flags)
	require.NoError(t, err)
}

func Test_CreateWithDryRun(t *testing.T) {
	path := t.TempDir()
	ctx := context.Background()

	opts := MirrorOptions{
		RootOptions: &cli.RootOptions{
			Dir:      path,
			LogLevel: "info",
			IOStreams: genericclioptions.IOStreams{
				In:     os.Stdin,
				Out:    os.Stdout,
				ErrOut: os.Stderr,
			},
		},
		ConfigPath: "testdata/configs/test.yaml",
		DryRun:     true,
		OutputDir:  path,
	}
	flags := NewMirrorCmd().Flags()
	err := opts.Create(ctx, flags)
	require.NoError(t, err)

	// should not produce an archive
	_, err = os.Stat(filepath.Join(path, "mirror_seq1_00000.tar"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func Test_CreateWithCancel(t *testing.T) {
	path := t.TempDir()
	ctx := context.Background()

	cancelCh := make(chan struct{})
	// closing the channel will cause the
	// command to exit if using a cancellable context
	close(cancelCh)

	opts := MirrorOptions{
		RootOptions: &cli.RootOptions{
			Dir:      path,
			LogLevel: "info",
			IOStreams: genericclioptions.IOStreams{
				In:     os.Stdin,
				Out:    os.Stdout,
				ErrOut: os.Stderr,
			},
		},
		ConfigPath:  "testdata/configs/test.yaml",
		OutputDir:   path,
		cancelCh:    cancelCh,
		SkipCleanup: true,
	}
	flags := NewMirrorCmd().Flags()
	err := opts.Create(ctx, flags)
	require.NoError(t, err)

	require.Equal(t, true, opts.interrupted)
}
