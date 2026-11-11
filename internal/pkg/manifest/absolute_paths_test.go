package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

func TestExtractOCILayersHonorsAbsoluteWhiteout(t *testing.T) {
	log := clog.New("error")
	manifest := &Manifest{Log: log}

	oldCfg := []byte("{\"schema\":\"olm.channel\",\"name\":\"stable-4.20\",\"package\":\"recipe\"}\n")
	newCfg := []byte("{\"schema\":\"olm.channel\",\"name\":\"stable-4.20\",\"package\":\"recipe\",\"entries\":[{\"name\":\"recipe.v4.20.21\"}]}\n")

	extractAndCheck := func(t *testing.T, oldFiles, whiteout, newFiles map[string][]byte) {
		t.Helper()
		oldLayer, err := crane.Layer(oldFiles)
		require.NoError(t, err)
		whiteoutLayer, err := crane.Layer(whiteout)
		require.NoError(t, err)
		newLayer, err := crane.Layer(newFiles)
		require.NoError(t, err)

		// Match oc-mirror's catalog rebuild: whiteout layer, then the replacement configs.
		img, err := mutate.AppendLayers(empty.Image, oldLayer, whiteoutLayer, newLayer)
		require.NoError(t, err)

		dest := t.TempDir()
		require.NoError(t, manifest.ExtractOCILayers(img, dest, "/configs"))

		_, err = os.Stat(filepath.Join(dest, "configs", "recipe", "index.json"))
		assert.ErrorIs(t, err, os.ErrNotExist)
		got, err := os.ReadFile(filepath.Join(dest, "configs", "recipe", "catalog.json"))
		require.NoError(t, err)
		assert.Equal(t, newCfg, got)
	}

	t.Run("absolute whiteout and relative files", func(t *testing.T) {
		extractAndCheck(t,
			map[string][]byte{"configs/recipe/index.json": oldCfg},
			map[string][]byte{"/.wh.configs": {}},
			map[string][]byte{"configs/recipe/catalog.json": newCfg},
		)
	})

	t.Run("absolute whiteout and absolute files", func(t *testing.T) {
		extractAndCheck(t,
			map[string][]byte{"/configs/recipe/index.json": oldCfg},
			map[string][]byte{"/.wh.configs": {}},
			map[string][]byte{"/configs/recipe/catalog.json": newCfg},
		)
	})

	t.Run("relative whiteout still deletes the old config", func(t *testing.T) {
		extractAndCheck(t,
			map[string][]byte{"configs/recipe/index.json": oldCfg},
			map[string][]byte{".wh.configs": {}},
			map[string][]byte{"configs/recipe/catalog.json": newCfg},
		)
	})
}
