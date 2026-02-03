package registriesd

import (
	"path/filepath"
	"testing"

	"github.com/openshift/oc-mirror/v2/internal/pkg/parser"

	"github.com/stretchr/testify/assert"
)

func TestPrepareRegistrydCustomDir(t *testing.T) {
	testFolder := t.TempDir()
	expectedCacheRegistryFile := filepath.Join(GetWorkingDirRegistrydConfigPath(testFolder), "localhost:55000.yaml")
	expectedDestRegistryFile := filepath.Join(GetWorkingDirRegistrydConfigPath(testFolder), "mymirror.com.yaml")

	regs := map[string]struct{}{
		"localhost:55000": {},
		"mymirror.com":    {},
	}

	err := PrepareRegistrydCustomDir(testFolder, "", regs)

	assert.NoError(t, err)

	assert.FileExists(t, expectedCacheRegistryFile)
	cfg, err := parser.ParseYamlFile[registryConfiguration](expectedCacheRegistryFile)
	assert.NoError(t, err)
	assert.Contains(t, cfg.Docker, "localhost:55000")
	assert.Equal(t, boolPtr(true), cfg.Docker["localhost:55000"].UseSigstoreAttachments)

	assert.FileExists(t, expectedDestRegistryFile)
	cfg, err = parser.ParseYamlFile[registryConfiguration](expectedDestRegistryFile)
	assert.NoError(t, err)
	assert.Contains(t, cfg.Docker, "mymirror.com")
	assert.Equal(t, boolPtr(true), cfg.Docker["mymirror.com"].UseSigstoreAttachments)
}

func TestGetCustomRegistrydConfigPath(t *testing.T) {
	testFolder := t.TempDir()

	registriesDirPath := GetWorkingDirRegistrydConfigPath(testFolder)

	expectedRegistriesD := filepath.Join(testFolder, "containers/registries.d")
	assert.Equal(t, expectedRegistriesD, registriesDirPath)
}
