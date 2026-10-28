package registriesd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/otiai10/copy"
	"go.podman.io/storage/pkg/fileutils"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/v2/internal/pkg/parser"
)

const (
	systemRegistriesDirPath string = "/etc/containers/registries.d"
	containersSubPath       string = "containers"
	registriesDSubPath      string = "registries.d"
)

var userRegistriesDir = filepath.FromSlash(".config/containers/registries.d")

func PrepareRegistrydCustomDir(workingDir, registriesDirPath string, registryHosts map[string]struct{}) error {
	var defaultRegistrydConfigPath, customRegistrydConfigPath string
	var err error

	if registriesDirPath != "" {
		defaultRegistrydConfigPath = registriesDirPath
	} else {
		if defaultRegistrydConfigPath, err = GetDefaultRegistrydConfigPath(); err != nil {
			return fmt.Errorf("error getting the default registryd config path : %w", err)
		}
	}

	customRegistrydConfigPath = GetWorkingDirRegistrydConfigPath(workingDir)

	if err := copyDefaultConfigsToWorkingDir(defaultRegistrydConfigPath, customRegistrydConfigPath); err != nil {
		return fmt.Errorf("error copying default registryd configs to custom registryd config path : %w", err)
	}

	if err := addRegistriesd(customRegistrydConfigPath, registryHosts); err != nil {
		return err
	}

	return nil
}

func GetDefaultRegistrydConfigPath() (string, error) {
	// Use os.UserHomeDir() instead of user.Current() to support containerized
	// environments with arbitrary UIDs that don't exist in /etc/passwd.
	// os.UserHomeDir() reads $HOME env var first, avoiding passwd lookups.
	homeDir, err := os.UserHomeDir()
	if err == nil {
		return registriesDirPathWithHomeDir(homeDir), nil
	}

	// Fall back to system default if home directory cannot be determined.
	return systemRegistriesDirPath, nil
}

func registriesDirPathWithHomeDir(homeDir string) string {
	userRegistriesDirPath := filepath.Join(homeDir, userRegistriesDir)
	if err := fileutils.Exists(userRegistriesDirPath); err == nil {
		return userRegistriesDirPath
	}
	return systemRegistriesDirPath
}

func GetWorkingDirRegistrydConfigPath(workingDir string) string {
	return filepath.Join(workingDir, containersSubPath, registriesDSubPath)
}

// GetWorkingDirLookasideOnlyRegistrydConfigPath is a sibling of the working-dir
// registries.d that keeps lookaside/GPG settings but turns off sigstore
// attachment handling.
func GetWorkingDirLookasideOnlyRegistrydConfigPath(workingDir string) string {
	return filepath.Join(workingDir, containersSubPath, registriesDSubPath+"-lookaside")
}

// PrepareLookasideOnlyDir writes a registries.d clone of the working-dir config
// with use-sigstore-attachments forced off so copy.Image will not rebuild
// sha256-*.sig tags via PutSignatures. Lookaside (GPG) entries are preserved.
func PrepareLookasideOnlyDir(workingDir string) (string, error) {
	src := GetWorkingDirRegistrydConfigPath(workingDir)
	dst := GetWorkingDirLookasideOnlyRegistrydConfigPath(workingDir)
	if err := os.MkdirAll(dst, 0755); err != nil {
		return "", fmt.Errorf("error creating lookaside-only registries.d: %w", err)
	}
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return dst, nil
	} else if err != nil {
		return "", fmt.Errorf("error reading registries.d %s: %w", src, err)
	}

	files, err := os.ReadDir(src)
	if err != nil {
		return "", fmt.Errorf("error reading registries.d %s: %w", src, err)
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if err := writeLookasideOnlyFile(src, dst, file.Name()); err != nil {
			return "", err
		}
	}
	return dst, nil
}

func writeLookasideOnlyFile(srcDir, dstDir, name string) error {
	if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
		return nil
	}
	srcPath := filepath.Join(srcDir, name)
	cfg, err := parser.ParseYamlFile[registryConfiguration](srcPath)
	if err != nil {
		return fmt.Errorf("error parsing %s: %w", srcPath, err)
	}
	disableSigstoreAttachments(&cfg)
	body, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("error marshaling %s: %w", name, err)
	}
	return writeFileAtomic(filepath.Join(dstDir, name), body)
}

func writeFileAtomic(path string, body []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("error creating temp file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if rmErr := os.Remove(tmpName); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("error removing temp file %s: %w", tmpName, rmErr))
		}
	}()
	if chmodErr := tmp.Chmod(0o600); chmodErr != nil {
		return errors.Join(fmt.Errorf("error setting permissions on %s: %w", path, chmodErr), tmp.Close())
	}
	if _, writeErr := tmp.Write(body); writeErr != nil {
		return errors.Join(fmt.Errorf("error writing %s: %w", path, writeErr), tmp.Close())
	}
	if closeErr := tmp.Close(); closeErr != nil {
		return fmt.Errorf("error closing %s: %w", path, closeErr)
	}
	if renameErr := os.Rename(tmpName, path); renameErr != nil {
		return fmt.Errorf("error writing %s: %w", path, renameErr)
	}
	return nil
}

func disableSigstoreAttachments(cfg *registryConfiguration) {
	if cfg.DefaultDocker != nil {
		ns := *cfg.DefaultDocker
		ns.UseSigstoreAttachments = boolPtr(false)
		cfg.DefaultDocker = &ns
	}
	for name, ns := range cfg.Docker {
		ns.UseSigstoreAttachments = boolPtr(false)
		cfg.Docker[name] = ns
	}
}

func copyDefaultConfigsToWorkingDir(defaultRegistrydConfigPath, customRegistrydConfigPath string) error {
	if err := os.MkdirAll(filepath.Dir(customRegistrydConfigPath), 0755); err != nil {
		return fmt.Errorf("error creating folder %s %w", filepath.Dir(customRegistrydConfigPath), err)
	}

	if _, err := os.Stat(defaultRegistrydConfigPath); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err := copy.Copy(defaultRegistrydConfigPath, customRegistrydConfigPath); err != nil {
		return fmt.Errorf("error copying from dir %s to %s %w", defaultRegistrydConfigPath, filepath.Dir(customRegistrydConfigPath), err)
	}

	return nil
}

func addRegistriesd(customizableRegistriesDir string, registries map[string]struct{}) error {

	configs, err := loadRegistryConfigFiles(customizableRegistriesDir)
	if err != nil && errors.Is(err, configUnmarshalError{}) {
		return err
	}

	mandatoryRegistriesWithoutConfig(configs, registries)

	for reg := range registries {
		if err := addRegistryd(customizableRegistriesDir, reg); err != nil {
			return err
		}
	}
	return nil
}

func addRegistryd(customizableRegistriesDir, registryHost string) error {
	registryFileName := fileName(registryHost)
	registryFileAbsPath := filepath.Join(customizableRegistriesDir, registryFileName)

	return createRegistryConfigFile(registryFileAbsPath, registryHost)
}

func fileName(registryURL string) string {
	return registryURL + ".yaml"
}

func createRegistryConfigFile(registryFileAbsPath, registryHost string) error {
	err := os.MkdirAll(filepath.Dir(registryFileAbsPath), 0755)
	if err != nil {
		return fmt.Errorf("error creating cache")
	}
	registryConfigFile, err := os.Create(registryFileAbsPath)
	if err != nil {
		return fmt.Errorf("error creating registry config file %w", err)
	}
	defer registryConfigFile.Close()

	var registryConfigStruct registryConfiguration
	if registryHost != "default" {
		registryConfigStruct = registryConfiguration{
			Docker: map[string]registryNamespace{
				registryHost: {
					UseSigstoreAttachments: boolPtr(true),
				},
			},
			DefaultDocker: nil,
		}
	} else {
		registryConfigStruct = registryConfiguration{
			DefaultDocker: &registryNamespace{UseSigstoreAttachments: boolPtr(true)},
		}
	}

	ccBytes, err := yaml.Marshal(registryConfigStruct)
	if err != nil {
		return fmt.Errorf("error marshaling registry config struct %w", err)
	}
	_, err = registryConfigFile.Write(ccBytes)
	if err != nil {
		return fmt.Errorf("error wring the registry config file %w", err)
	}

	return nil
}

func loadRegistryConfigFiles(customizableRegistriesDir string) ([]registryConfiguration, error) {
	configFiles := []registryConfiguration{}

	files, err := os.ReadDir(customizableRegistriesDir)
	if err != nil {
		return nil, fmt.Errorf("error reading custom registriesd directory %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filePath := filepath.Join(customizableRegistriesDir, file.Name())

		var registryConfigStruct registryConfiguration
		if registryConfigStruct, err = parser.ParseYamlFile[registryConfiguration](filePath); err != nil {
			return nil, configUnmarshalError{err: err}
		}
		configFiles = append(configFiles, registryConfigStruct)
	}

	return configFiles, nil
}

func mandatoryRegistriesWithoutConfig(configFiles []registryConfiguration, registries map[string]struct{}) {
	zeroStruct := registryNamespace{}
	for _, configFile := range configFiles {
		if configFile.DefaultDocker != nil && *configFile.DefaultDocker != zeroStruct {
			delete(registries, "default")
		}

		if len(configFile.Docker) == 0 {
			continue
		}

		for dockerReg, dockerRegValue := range configFile.Docker {
			for registry := range registries {
				if strings.Contains(dockerReg, registry) && dockerRegValue != zeroStruct {
					delete(registries, registry)
				}
			}
		}

	}
}
