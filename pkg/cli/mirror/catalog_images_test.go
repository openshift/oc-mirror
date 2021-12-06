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
func Test_BuildCatalogLayer(t *testing.T) {

	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Error(err)
	}
	c := map[string][]byte{
		"/binary": []byte("binary contents"),
	}
	targetRef := fmt.Sprintf("%s/nginx:foo", u.Host)
	tag, err := name.NewTag(targetRef)
	if err != nil {
		require.NoError(t, err)
	}
	i, _ := crane.Image(c)
	err = crane.Push(i, tag.String())
	if err != nil {
		require.NoError(t, err)
	}

	old := "/.wh.binary"
	tmpdir := t.TempDir()
	d1 := []byte("hello\ngo\n")
	if err := ioutil.WriteFile(filepath.Join(tmpdir, "test"), d1, 0644); err != nil {
		require.NoError(t, err)
	}

	add, err := addLayer(filepath.Join(tmpdir, "test"), "binary")
	if err != nil {
		t.Fatal(err)
	}
	delete, err := deleteLayer(old)
	if err != nil {
		t.Fatal(err)
	}

	o := &MirrorOptions{
		DestSkipTLS: true,
	}

	err = o.buildCatalogLayer(context.Background(), targetRef, targetRef, t.TempDir(), []v1.Layer{add, delete}...)
	if err != nil {
		t.Error(err)
	}
}
