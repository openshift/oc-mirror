package mirror

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

			builder := &catalogBuilder{
				nameOpts: []name.Option{name.Insecure},
			}

			var err error
			if test.existingImage {
				_, err = builder.CreateLayout(targetRef, t.TempDir())
			} else {
				_, err = builder.CreateLayout("", tmpdir)
			}

			if test.err == "" {
				require.NoError(t, err)
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

			old := "/.wh.binary"
			d1 := []byte("hello\ngo\n")
			require.NoError(t, ioutil.WriteFile(filepath.Join(tmpdir, "test"), d1, 0644))

			add, err := layerFromFile("binary", filepath.Join(tmpdir, "test"))
			require.NoError(t, err)
			delete, err := deleteLayer(old)
			require.NoError(t, err)

			builder := &catalogBuilder{
				nameOpts: []name.Option{name.Insecure},
			}

			var layout layout.Path
			if test.existingImage {
				layout, err = builder.CreateLayout(targetRef, t.TempDir())
				require.NoError(t, err)
			} else {
				layout, err = builder.CreateLayout("", tmpdir)
				require.NoError(t, err)
			}

			err = builder.Run(context.Background(), targetRef, layout, []v1.Layer{add, delete}...)
			if test.err == "" {
				require.NoError(t, err)
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
		"/binary": []byte("binary contents"),
	}
	targetRef := fmt.Sprintf("%s/nginx:foo", u.Host)
	tag, err := name.NewTag(targetRef)
	require.NoError(t, err)
	i, _ := crane.Image(c)
	require.NoError(t, crane.Push(i, tag.String()))
	lp, err := layout.Write(dir, empty.Index)
	require.NoError(t, err)
	require.NoError(t, lp.AppendImage(i))
	idx, err := lp.ImageIndex()
	require.NoError(t, err)
	remote.WriteIndex(tag, idx)
	return targetRef
}
