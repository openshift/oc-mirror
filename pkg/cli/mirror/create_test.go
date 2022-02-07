package mirror

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
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

// TODO(jpower432): test mapping output
func TestCreate(t *testing.T) {
	path := t.TempDir()
	ctx := context.Background()

	configPath := path + "/test.yaml"
	require.NoError(t, generateConfigPath(path, "testdata/configs/test.yaml", configPath))
	cfg, err := config.LoadConfig(configPath)
	require.NoError(t, err)

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
	_, _, err = opts.Create(ctx, cfg)
	require.NoError(t, err)
}

func generateConfigPath(path string, baseFile string, targetFile string) error {

	read, err := ioutil.ReadFile(baseFile)
	if err != nil {
		return err
	}

	newContents := strings.Replace(string(read), "testdata", path+"/testdata", -1)

	return ioutil.WriteFile(targetFile, []byte(newContents), 0744)
}
