package registriesd

import (
	"os"
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
	assert.True(t, cfg.Docker["localhost:55000"].UseSigstoreAttachments)

	assert.FileExists(t, expectedDestRegistryFile)
	cfg, err = parser.ParseYamlFile[registryConfiguration](expectedDestRegistryFile)
	assert.NoError(t, err)
	assert.Contains(t, cfg.Docker, "mymirror.com")
	assert.True(t, cfg.Docker["mymirror.com"].UseSigstoreAttachments)
}

func TestGetCustomRegistrydConfigPath(t *testing.T) {
	testFolder := t.TempDir()

	registriesDirPath := GetWorkingDirRegistrydConfigPath(testFolder)

	expectedRegistriesD := filepath.Join(testFolder, "containers/registries.d")
	assert.Equal(t, expectedRegistriesD, registriesDirPath)
}

func TestGetDefaultRegistrydConfigPath(t *testing.T) {
	testHome := t.TempDir()
	t.Setenv("HOME", testHome)

	// When user registries.d doesn't exist, should return system path
	path, err := GetDefaultRegistrydConfigPath()
	assert.NoError(t, err)
	assert.Equal(t, systemRegistriesDirPath, path)

	// When user registries.d exists, should return user path
	expectedUserPath := filepath.Join(testHome, ".config/containers/registries.d")
	err = os.MkdirAll(expectedUserPath, 0755)
	assert.NoError(t, err)

	path, err = GetDefaultRegistrydConfigPath()
	assert.NoError(t, err)
	assert.Equal(t, expectedUserPath, path)
}
