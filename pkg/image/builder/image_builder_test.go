package builder

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
)

func TestCreateLayout(t *testing.T) {

	tests := []struct {
		name          string
		existingImage bool
		err           string
	}{
		{
			name:          "Valid/ExistingImage",
			existingImage: true,
		},
		{
			name:          "Valid/NewImage",
			existingImage: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			tmpdir := t.TempDir()

			targetRef := prepareImage(t, tmpdir)

			builder := &ImageBuilder{
				NameOpts: []name.Option{name.Insecure},
			}

			var err error
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
		name          string
		existingImage bool
		err           string
	}{
		{
			name:          "Valid/ExistingImage",
			existingImage: true,
		},
		{
			name:          "Valid/NewImage",
			existingImage: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			tmpdir := t.TempDir()

			targetRef := prepareImage(t, tmpdir)

			d1 := []byte("hello\ngo\n")
			require.NoError(t, ioutil.WriteFile(filepath.Join(tmpdir, "test"), d1, 0644))

			add, err := LayerFromPath("/testfile", filepath.Join(tmpdir, "test"))
			require.NoError(t, err)

			builder := &ImageBuilder{
				NameOpts: []name.Option{name.Insecure},
			}

			var layout layout.Path
			if test.existingImage {
				layout, err = builder.CreateLayout(targetRef, t.TempDir())
				require.NoError(t, err)
			} else {
				layout, err = builder.CreateLayout("", tmpdir)
				require.NoError(t, err)
			}
			err = builder.Run(context.Background(), targetRef, layout, nil, []v1.Layer{add}...)
			if test.err == "" {
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

			} else {
				require.EqualError(t, err, test.err)
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

			// prep directory will files to write into layer
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

func prepareImage(t *testing.T, dir string) string {
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	c := map[string][]byte{
		"/testfile": []byte("test contents contents"),
	}
	targetRef := fmt.Sprintf("%s/bar:foo", u.Host)
	tag, err := name.NewTag(targetRef)
	require.NoError(t, err)
	i, _ := crane.Image(c)
	require.NoError(t, crane.Push(i, tag.String()))
	lp, err := layout.Write(dir, empty.Index)
	require.NoError(t, err)
	require.NoError(t, lp.AppendImage(i))
	idx, err := lp.ImageIndex()
	require.NoError(t, err)
	require.NoError(t, remote.WriteIndex(tag, idx))
	return targetRef
}
