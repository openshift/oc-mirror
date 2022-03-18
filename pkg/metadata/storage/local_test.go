package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
)

func TestLocalBackend(t *testing.T) {

	underlyingFS := afero.NewMemMapFs()
	backend := localDirBackend{
		fs:  underlyingFS,
		dir: filepath.Join("foo", config.SourceDir),
	}
	require.NoError(t, backend.init())

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

	ctx := context.Background()

	require.NoError(t, backend.WriteMetadata(ctx, m, config.MetadataBasePath))

	info, metadataErr := underlyingFS.Stat("foo/src/publish/.metadata.json")
	require.NoError(t, metadataErr)
	require.True(t, info.Mode().IsRegular())
	info, metadataErr = backend.fs.Stat("publish/.metadata.json")
	require.NoError(t, metadataErr)
	require.True(t, info.Mode().IsRegular())
	info, metadataErr = backend.Stat(ctx, "publish/.metadata.json")
	require.NoError(t, metadataErr)
	require.True(t, info.Mode().IsRegular())
	_, metadataErr = backend.Open(ctx, "publish/.metadata.json")
	require.NoError(t, metadataErr)

	readMeta := &v1alpha2.Metadata{}
	require.NoError(t, backend.ReadMetadata(ctx, readMeta, config.MetadataBasePath))
	require.Equal(t, m, readMeta)

	metadataErr = backend.Cleanup(ctx, config.MetadataBasePath)
	require.NoError(t, metadataErr)
	require.ErrorIs(t, backend.ReadMetadata(ctx, readMeta, config.MetadataBasePath), ErrMetadataNotExist)

	type object struct {
		SomeData string
	}

	inObj := object{
		SomeData: "bar",
	}
	require.NoError(t, backend.WriteObject(ctx, "bar-obj.json", inObj))

	info, objErr := underlyingFS.Stat("foo/src/bar-obj.json")
	require.NoError(t, objErr)
	require.True(t, info.Mode().IsRegular())
	info, objErr = backend.fs.Stat("bar-obj.json")
	require.NoError(t, objErr)
	require.True(t, info.Mode().IsRegular())

	var outObj object
	require.NoError(t, backend.ReadObject(ctx, "bar-obj.json", &outObj))
	require.Equal(t, inObj, outObj)
}
