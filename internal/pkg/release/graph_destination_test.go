package release

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func TestGraphImageDestination(t *testing.T) {
	for _, tc := range []struct {
		name, mode, destination, expected string
		limit                             int
	}{
		{"direct unlimited", mirror.MirrorToMirror, "mirror.example.com/team", "mirror.example.com/team/openshift/graph-image:latest", 0},
		{"direct flattened", mirror.MirrorToMirror, "mirror.example.com/team", "mirror.example.com/team/openshift-graph-image:latest", 2},
		{"restore flattened", mirror.DiskToMirror, "mirror.example.com/team", "mirror.example.com/team/openshift-graph-image:latest", 2},
		{"cache unchanged", mirror.MirrorToDisk, "localhost:55000", "localhost:55000/openshift/graph-image:latest", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := GraphImageDestination(tc.destination, mirror.CopyOptions{
				Mode: tc.mode, Global: &mirror.GlobalOptions{MaxNestedPaths: tc.limit},
			})
			require.NoError(t, err)
			require.Equal(t, tc.expected, ref)
		})
	}
	t.Run("invalid destination", func(t *testing.T) {
		_, err := GraphImageDestination("mirror.example.com/team@invalid", mirror.CopyOptions{
			Mode: mirror.MirrorToMirror, Global: &mirror.GlobalOptions{MaxNestedPaths: 2},
		})
		require.ErrorContains(t, err, "invalid digest")
	})
}

func TestHandleCachedGraphImageDestinationNesting(t *testing.T) {
	t.Setenv("UPDATE_URL_OVERRIDE", "https://cincinnati.example.com/graph")
	for _, mode := range []string{mirror.MirrorToMirror, mirror.MirrorToDisk} {
		for _, inCache := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/inCache=%t", mode, inCache), func(t *testing.T) {
				o := setupCollector_DiskToMirror(t.TempDir(), clog.New("error"))
				o.Opts.Mode = mode
				o.Opts.Destination = "docker://mirror.example.com/team"
				o.Opts.Global.MaxNestedPaths = 2
				if mode == mirror.MirrorToDisk {
					o.Opts.Destination = "file://" + o.Opts.Global.WorkingDir
					o.Opts.Global.MaxNestedPaths = 1
				}
				cacheRef := "docker://" + GraphImageReference(o.LocalStorageFQDN)
				source := cacheRef
				manifest := new(ManifestMock)
				if inCache {
					manifest.On("ImageDigest", mock.Anything, mock.Anything, cacheRef).Return("digest", nil)
				} else {
					source = "oci://" + filepath.Join(o.Opts.Global.WorkingDir, graphPreparationDir)
					manifest.On("ImageDigest", mock.Anything, mock.Anything, cacheRef).Return("", nil)
					manifest.On("ImageDigest", mock.Anything, mock.Anything, source).Return("digest", nil)
				}
				o.Manifest = manifest
				graph, err := o.handleGraphImage(context.Background())
				require.NoError(t, err)
				expected := "docker://mirror.example.com/team/openshift-graph-image:latest"
				if mode == mirror.MirrorToDisk {
					expected = cacheRef
				}
				require.Equal(t, expected, graph.Destination)
				require.Equal(t, source, graph.Source)
				require.Equal(t, source, graph.Origin)
				manifest.AssertExpectations(t)
			})
		}
	}
}
