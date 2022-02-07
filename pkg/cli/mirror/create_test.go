package mirror

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"io/ioutil"
	"strings"

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

func TestCreate(t *testing.T) {
	path := t.TempDir()
	ctx := context.Background()

	configPath := path+"/test.yaml"
	generateConfigPath(path, "testdata/configs/test.yaml", configPath)

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
		ConfigPath: configPath,
		OutputDir:  path,
	}
	err := opts.Create(ctx)
	require.NoError(t, err)
}

func TestCreateWithNoChanges(t *testing.T) {
	path := t.TempDir()
	ctx := context.Background()

	configPath := path+"/test.yaml"
	generateConfigPath(path, "testdata/configs/test.yaml", configPath)

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
			ConfigPath: configPath,
			OutputDir:  path,
	}
	// First run will create mirror_seq1_0000.tar
	err := opts.Create(ctx)
	require.NoError(t, err)

	// should produce an archive
	_, err = os.Stat(filepath.Join(path, "mirror_seq1_000000.tar"))

	// Second run should not create mirror_seq2_0000.tar
	err = opts.Create(ctx)
	// Second run should throw a error
	require.ErrorIs(t, err, NoUpdatesExist)

	// should not produce an archive
	_, err = os.Stat(filepath.Join(path, "mirror_seq2_000000.tar"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCreateWithDryRun(t *testing.T) {
	path := t.TempDir()
	ctx := context.Background()

	configPath := path+"/test.yaml"
	generateConfigPath(path, "testdata/configs/test.yaml", configPath)

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
		ConfigPath: configPath,
		DryRun:     true,
		OutputDir:  path,
	}
	err := opts.Create(ctx)
	require.NoError(t, err)

	// should not produce an archive
	_, err = os.Stat(filepath.Join(path, "mirror_seq1_00000.tar"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCreateWithCancel(t *testing.T) {
	path := t.TempDir()
	ctx := context.Background()

	configPath := path+"/test.yaml"
	generateConfigPath(path, "testdata/configs/test.yaml", configPath)

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
		ConfigPath:  configPath,
		OutputDir:   path,
		SkipCleanup: true,
	}
	// initialize cancelCh so it
	// does not get reinitialized during the function call
	opts.once.Do(opts.init)
	cancelCh := make(chan struct{})
	opts.cancelCh = cancelCh
	// closing the channel will cause the
	// command to exit if using a cancellable context
	close(cancelCh)
	err := opts.Create(ctx)
	require.NoError(t, err)

	require.Equal(t, true, opts.interrupted)
}

func generateConfigPath(path string, baseFile string, targetFile string) {

	read, err := ioutil.ReadFile(baseFile)
	if err != nil {
		panic(err)
	}

	newContents := strings.Replace(string(read), "testdata", path+"/testdata", -1)

	err = ioutil.WriteFile(targetFile, []byte(newContents), 0744)
	if err != nil {
		panic(err)
	}

}
