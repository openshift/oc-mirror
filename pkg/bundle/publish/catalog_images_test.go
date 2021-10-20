package publish

import (
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
)

func Test_BuildCatalogLayer(t *testing.T) {

	server := httptest.NewServer(registry.New())
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

	err = buildCatalogLayer(old, filepath.Join(tmpdir, "test"), "binary", targetRef, targetRef)
	if err != nil {
		t.Error(err)
	}
}
