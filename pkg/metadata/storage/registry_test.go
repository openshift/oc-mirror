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
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/stretchr/testify/require"
)

func Test_RegistryBackend(t *testing.T) {

	tests := []struct {
		name  string
		image string
		err   string
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
			cfg := v1alpha1.RegistryConfig{
				ImageURL: image,
				SkipTLS:  true,
			}
			ctx := context.Background()
			backend, err := NewRegistryBackend(&cfg, filepath.Join("foo", config.SourceDir))
			require.NoError(t, err)

			m := &v1alpha1.Metadata{}
			m.Uid = uuid.New()
			m.PastMirrors = []v1alpha1.PastMirror{
				{
					Timestamp: int(time.Now().Unix()),
					Sequence:  1,
					Mirror: v1alpha1.Mirror{
						OCP: v1alpha1.OCP{
							Channels: []v1alpha1.ReleaseChannel{
								{Name: "stable-4.7", Versions: []string{"4.7.13"}},
							},
						},
						Operators: []v1alpha1.Operator{
							{Catalog: "registry.redhat.io/openshift/redhat-operators:v4.7", HeadsOnly: true},
						},
					},
					Operators: []v1alpha1.OperatorMetadata{
						{
							Catalog:  "registry.redhat.io/openshift/redhat-operators:v4.7",
							ImagePin: "registry.redhat.io/openshift/redhat-operators@sha256:a05ed1726b3cdc16e694b8ba3e26e834428a0fa58bc220bb0e07a30a76a595a6",
						},
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

			readMeta := &v1alpha1.Metadata{}
			require.NoError(t, backend.ReadMetadata(ctx, readMeta, config.MetadataBasePath))
			require.Equal(t, m, readMeta)

			metadataErr = backend.Cleanup(ctx, config.MetadataBasePath)
			require.NoError(t, metadataErr)
			require.ErrorIs(t, backend.ReadMetadata(ctx, readMeta, config.MetadataBasePath), ErrMetadataNotExist)
		})
	}
}
