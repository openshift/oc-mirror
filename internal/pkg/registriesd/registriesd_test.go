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

func TestPrepareLookasideOnlyDir(t *testing.T) {
	testFolder := t.TempDir()
	regs := map[string]struct{}{
		"mymirror.com": {},
	}
	err := PrepareRegistrydCustomDir(testFolder, "", regs)
	assert.NoError(t, err)

	lookasideFile := filepath.Join(GetWorkingDirRegistrydConfigPath(testFolder), "mymirror.com.yaml")
	assert.NoError(t, os.WriteFile(lookasideFile, []byte("docker:\n  mymirror.com:\n    lookaside: file:///var/lib/oc-mirror/signatures\n    use-sigstore-attachments: true\n"), 0644))

	dir, err := PrepareLookasideOnlyDir(testFolder)
	assert.NoError(t, err)
	assert.Equal(t, GetWorkingDirLookasideOnlyRegistrydConfigPath(testFolder), dir)

	cloned, err := parser.ParseYamlFile[registryConfiguration](filepath.Join(dir, "mymirror.com.yaml"))
	assert.NoError(t, err)
	assert.Equal(t, "file:///var/lib/oc-mirror/signatures", cloned.Docker["mymirror.com"].Lookaside)
	assert.Equal(t, boolPtr(false), cloned.Docker["mymirror.com"].UseSigstoreAttachments)

	empty, err := PrepareLookasideOnlyDir(filepath.Join(testFolder, "missing-workdir"))
	assert.NoError(t, err)
	assert.DirExists(t, empty)
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
	err = os.MkdirAll(expectedUserPath, 0o755)
	assert.NoError(t, err)

	path, err = GetDefaultRegistrydConfigPath()
	assert.NoError(t, err)
	assert.Equal(t, expectedUserPath, path)
}
