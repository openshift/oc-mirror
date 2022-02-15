package storage

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha2"
	"github.com/stretchr/testify/require"
)

func TestByConfig(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	customDir := t.TempDir()

	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Error(err)
	}

	tests := []struct {
		name        string
		cfg         v1alpha2.StorageConfig
		expected    Backend
		expectedDir string
	}{{
		name: "local-backend",
		cfg: v1alpha2.StorageConfig{
			Local: &v1alpha2.LocalConfig{
				Path: filepath.Join(customDir, "local-backend"),
			},
		},
		expected:    &localDirBackend{},
		expectedDir: customDir,
	}, {
		name: "registry-backend",
		cfg: v1alpha2.StorageConfig{
			Registry: &v1alpha2.RegistryConfig{
				ImageURL: fmt.Sprintf("%s/test-meta", u.Host),
				SkipTLS:  true,
			},
		},
		expected:    &registryBackend{},
		expectedDir: dir,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			backend, err := ByConfig(filepath.Join(dir, test.name), test.cfg)
			require.NoError(t, err)

			switch v := backend.(type) {
			case *localDirBackend:
				if !reflect.DeepEqual(test.expected, &localDirBackend{}) {
					t.Errorf("Test %s, expected %v, got %v", test.name, test.expected, v)
				}
			case *registryBackend:
				if !reflect.DeepEqual(test.expected, &registryBackend{}) {
					t.Errorf("Test %s: expected %v, got %v", test.name, test.expected, v)
				}
			default:
				t.Fail()
			}

			meta := v1alpha2.Metadata{}
			err = backend.WriteMetadata(ctx, &meta, config.MetadataBasePath)
			require.NoError(t, err)

			_, err = backend.Stat(ctx, config.MetadataBasePath)
			require.NoError(t, err)

			_, err = os.Stat(filepath.Join(test.expectedDir, test.name, config.MetadataBasePath))
			require.NoError(t, err)
		})
	}
}
