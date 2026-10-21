package generate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/types"
)

func newTestRegistry(t *testing.T) (host string) {
	t.Helper()
	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	host = strings.TrimPrefix(srv.URL, "http://")

	push := func(ref string, labels map[string]string) {
		img, err := random.Image(64, 1)
		require.NoError(t, err)
		if labels != nil {
			cfg, err := img.ConfigFile()
			require.NoError(t, err)
			cfg.Config.Labels = labels
			img, err = mutate.ConfigFile(img, cfg)
			require.NoError(t, err)
		}
		r, err := name.ParseReference(host+"/"+ref, name.Insecure)
		require.NoError(t, err)
		require.NoError(t, remote.Write(r, img))
	}
	push("redhat/redhat-operator-index:v4.16", map[string]string{catalogLabel: "/configs"})
	push("ubi9/ubi:latest", nil)
	return host
}

func testSysCtx() *types.SystemContext {
	return &types.SystemContext{DockerInsecureSkipTLSVerify: types.OptionalBoolTrue}
}

func TestRegistrySource(t *testing.T) {
	host := newTestRegistry(t)
	s := &RegistrySource{Host: host, SysCtx: testSysCtx()}
	ctx := context.Background()

	repos, err := s.Repositories(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"redhat/redhat-operator-index", "ubi9/ubi"}, repos)

	tags, err := s.Tags(ctx, "ubi9/ubi")
	require.NoError(t, err)
	assert.Equal(t, []string{"latest"}, tags)

	ok, err := s.IsCatalog(ctx, "redhat/redhat-operator-index", "v4.16")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = s.IsCatalog(ctx, "ubi9/ubi", "latest")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRegistrySourceExplicitRepos(t *testing.T) {
	s := &RegistrySource{Host: "unused", Repos: []string{"a/b"}}
	repos, err := s.Repositories(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"a/b"}, repos)
}

func TestRegistrySourceCatalogDisabledHint(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	s := &RegistrySource{Host: strings.TrimPrefix(srv.URL, "http://"), SysCtx: testSysCtx()}
	_, err := s.Repositories(context.Background())
	assert.ErrorContains(t, err, "--repos")
}
