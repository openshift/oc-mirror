package storage

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/uuid"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestRegistryBackend(t *testing.T) {

	tests := []struct {
		name        string
		image       string
		closeServer bool
	}{{
		name:  "top level image with a tag",
		image: "metadata:latest",
	}, {
		name:  "namespace",
		image: "demo/metadata:test",
	}, {
		name:  "nested namespaces",
		image: "demo/another/metadata:latest",
	}, {
		name:  "no tag",
		image: "metadata",
	}, {
		name:        "force error",
		image:       "metadata",
		closeServer: true,
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			server := httptest.NewServer(registry.New())
			t.Cleanup(server.Close)
			u, err := url.Parse(server.URL)
			if err != nil {
				t.Error(err)
			}

			image := fmt.Sprintf("%s/%s", u.Host, test.image)
			cfg := v1alpha2.RegistryConfig{
				ImageURL: image,
				SkipTLS:  true,
			}
			ctx := context.Background()
			backend, err := NewRegistryBackend(&cfg, filepath.Join("foo", config.SourceDir))
			require.NoError(t, err)

			m := &v1alpha2.Metadata{}
			m.Uid = uuid.New()
			m.PastMirror = v1alpha2.PastMirror{
				Timestamp: int(time.Now().Unix()),
				Sequence:  1,
				Mirror: v1alpha2.Mirror{
					OCP: v1alpha2.OCP{
						Channels: []v1alpha2.ReleaseChannel{
							{Name: "stable-4.7", MinVersion: "4.7.13"},
						},
					},
					Operators: []v1alpha2.Operator{
						{Catalog: "registry.redhat.io/openshift/redhat-operators:v4.7"},
					},
				},
				Operators: []v1alpha2.OperatorMetadata{
					{
						Catalog:  "registry.redhat.io/openshift/redhat-operators:v4.7",
						ImagePin: "registry.redhat.io/openshift/redhat-operators@sha256:a05ed1726b3cdc16e694b8ba3e26e834428a0fa58bc220bb0e07a30a76a595a6",
					},
				},
			}

			require.NoError(t, backend.WriteMetadata(ctx, m, config.MetadataBasePath))

			info, metadataErr := os.Stat("foo/src/publish/.metadata.json")
			require.NoError(t, metadataErr)
			require.True(t, info.Mode().IsRegular())
			info, metadataErr = backend.Stat(ctx, config.MetadataBasePath)
			require.NoError(t, metadataErr)
			require.True(t, info.Mode().IsRegular())
			_, metadataErr = backend.Open(ctx, config.MetadataBasePath)
			require.NoError(t, metadataErr)

			readMeta := &v1alpha2.Metadata{}
			require.NoError(t, backend.ReadMetadata(ctx, readMeta, config.MetadataBasePath))
			require.Equal(t, m, readMeta)

			metadataErr = backend.Cleanup(ctx, config.MetadataBasePath)
			require.NoError(t, metadataErr)
			require.ErrorIs(t, backend.ReadMetadata(ctx, readMeta, config.MetadataBasePath), ErrMetadataNotExist)

			// Ensure when the server is close the metadata error is not thrown
			if test.closeServer {
				server.Close()
				require.Error(t, backend.ReadMetadata(ctx, readMeta, config.MetadataBasePath))
				require.NotErrorIs(t, backend.ReadMetadata(ctx, readMeta, config.MetadataBasePath), ErrMetadataNotExist)
			}
		})
	}
}
