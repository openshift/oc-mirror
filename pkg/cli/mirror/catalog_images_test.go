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
	"github.com/stretchr/testify/require"
)

// TODO: Pull image for manifest and index checks
func TestBuildCatalogLayer(t *testing.T) {
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
	old := "/.wh.binary"
	tmpdir := t.TempDir()
	d1 := []byte("hello\ngo\n")
	require.NoError(t, ioutil.WriteFile(filepath.Join(tmpdir, "test"), d1, 0644))

	add, err := addLayer(filepath.Join(tmpdir, "test"), "binary")
	require.NoError(t, err)
	delete, err := deleteLayer(old)
	require.NoError(t, err)

	o := &MirrorOptions{
		DestSkipTLS:   true,
		UserNamespace: "custom",
	}
	require.NoError(t, o.buildCatalogLayer(context.Background(), targetRef, targetRef, t.TempDir(), []v1.Layer{add, delete}...))
}

func TestBuildCatalogLayerCustomNS(t *testing.T) {

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
	old := "/.wh.binary"
	tmpdir := t.TempDir()
	d1 := []byte("hello\ngo\n")
	require.NoError(t, ioutil.WriteFile(filepath.Join(tmpdir, "test"), d1, 0644))

	add, err := addLayer(filepath.Join(tmpdir, "test"), "binary")
	require.NoError(t, err)
	delete, err := deleteLayer(old)
	require.NoError(t, err)

	o := &MirrorOptions{
		DestSkipTLS:   true,
		UserNamespace: "custom",
	}
	require.NoError(t, o.buildCatalogLayer(context.Background(), targetRef, targetRef, t.TempDir(), []v1.Layer{add, delete}...))

}
