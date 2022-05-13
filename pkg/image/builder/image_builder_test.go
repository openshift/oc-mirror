package builder

import (
	"context"
	"io/ioutil"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/internal/testutils"
)

func TestCreateLayout(t *testing.T) {

	tests := []struct {
		name          string
		existingImage bool
		err           string
	}{
		{
			name:          "Success/ExistingImage",
			existingImage: true,
		},
		{
			name:          "Success/NewImage",
			existingImage: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			tmpdir := t.TempDir()
			server := httptest.NewServer(registry.New())
			t.Cleanup(server.Close)

			targetRef, err := testutils.WriteTestImage(server, tmpdir)
			require.NoError(t, err)

			builder := NewImageBuilder([]name.Option{name.Insecure}, nil)

			var lp layout.Path
			if test.existingImage {
				lp, err = builder.CreateLayout(targetRef, t.TempDir())
			} else {
				lp, err = builder.CreateLayout("", tmpdir)
			}

			if test.err == "" {
				require.NoError(t, err)
				ii, err := lp.ImageIndex()
				require.NoError(t, err)
				im, err := ii.IndexManifest()
				require.NoError(t, err)
				require.Len(t, im.Manifests, 1)
			} else {
				require.EqualError(t, err, test.err)
			}
		})
	}
}

func TestRun(t *testing.T) {

	tests := []struct {
		name             string
		existingImage    bool
		pinToDigest      bool
		update           configUpdateFunc
		configAssertFunc func(cfg v1.ConfigFile) bool
		err              error
	}{
		{
			name:          "Success/ExistingImage",
			existingImage: true,
		},
		{
			name:          "Success/NewImage",
			existingImage: false,
		},
		{
			name:          "Success/WithConfigUpdate",
			existingImage: true,
			update: func(cfg *v1.ConfigFile) {
				cfg.Config.Cmd = []string{"newcommand"}
			},
			configAssertFunc: func(cfg v1.ConfigFile) bool {
				return cfg.Config.Cmd[0] == "newcommand"
			},
		},
		{
			name:        "Failure/DigestReference",
			pinToDigest: true,
			err:         &ErrInvalidReference{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			tmpdir := t.TempDir()
			server := httptest.NewServer(registry.New())
			t.Cleanup(server.Close)

			targetRef, err := testutils.WriteTestImage(server, tmpdir)
			require.NoError(t, err)

			if test.pinToDigest {
				targetRef, err = pinToDigest(targetRef)
				require.NoError(t, err)
			}

			d1 := []byte("hello\ngo\n")
			require.NoError(t, ioutil.WriteFile(filepath.Join(tmpdir, "test"), d1, 0644))

			add, err := LayerFromPath("/testfile", filepath.Join(tmpdir, "test"))
			require.NoError(t, err)

			builder := NewImageBuilder([]name.Option{name.Insecure}, nil)

			var layout layout.Path
			if test.existingImage {
				layout, err = builder.CreateLayout(targetRef, t.TempDir())
				require.NoError(t, err)
			} else {
				layout, err = builder.CreateLayout("", tmpdir)
				require.NoError(t, err)
			}
			err = builder.Run(context.Background(), targetRef, layout, test.update, []v1.Layer{add}...)
			if test.err == nil {
				require.NoError(t, err)

				// Get new image information
				ref, err := name.ParseReference(targetRef, name.Insecure)
				require.NoError(t, err)
				desc, err := remote.Get(ref)
				require.NoError(t, err)
				img, err := desc.Image()
				require.NoError(t, err)
				layers, err := img.Layers()
				require.NoError(t, err)
				idx, err := desc.ImageIndex()
				require.NoError(t, err)
				im, err := idx.IndexManifest()
				require.NoError(t, err)

				// Check that new layer is present
				expectedDigest, err := add.Digest()
				require.NoError(t, err)
				var found bool
				for _, ly := range layers {
					dg, err := ly.Digest()
					require.NoError(t, err)
					if dg == expectedDigest {
						found = true
					}
				}
				require.True(t, found)
				require.Len(t, im.Manifests, 1)

				if test.update != nil {
					config, err := img.ConfigFile()
					require.NoError(t, err)
					require.True(t, test.configAssertFunc(*config))
				}

			} else {
				require.ErrorAs(t, err, &test.err)
			}
		})
	}
}

func TestLayoutFromPath(t *testing.T) {

	tests := []struct {
		name       string
		dir        bool
		targetPath string
		err        string
	}{
		{
			name:       "Valid/DirPath",
			targetPath: "testdir/",
			dir:        true,
		},
		{
			name:       "Valid/FilePath",
			targetPath: "testfile",
			dir:        false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			tmpdir := t.TempDir()

			// prep directory with files to write into layer
			d1 := []byte("hello\ngo\n")
			require.NoError(t, ioutil.WriteFile(filepath.Join(tmpdir, "test"), d1, 0644))

			var sourcePath string
			if test.dir {
				sourcePath = tmpdir
			} else {
				sourcePath = filepath.Join(tmpdir, "test")
			}

			layer, err := LayerFromPath(test.targetPath, sourcePath)
			if test.err == "" {
				require.NoError(t, err)
				digest, err := layer.Digest()
				require.NoError(t, err)
				require.Contains(t, digest.String(), ":")
			} else {
				require.EqualError(t, err, test.err)
			}
		})
	}
}

func pinToDigest(unpinnedImage string) (string, error) {
	ref, err := reference.Parse(unpinnedImage)
	if err != nil {
		return "", err
	}
	ref.ID = "sha256:fc1ca63b4a6ac038808ae33c4498b122f9cf7a43dca278228e985986d3f81091"
	return ref.Exact(), nil
}
