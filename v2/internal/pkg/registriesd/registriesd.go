package registriesd

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/containers/storage/pkg/fileutils"
	"github.com/otiai10/copy"
	"sigs.k8s.io/yaml"
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
	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("unable to determine the current user : %w", err)
	}

	return registriesDirPathWithHomeDir(usr.HomeDir), nil
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

	// TODO the default file is generated when using skopeo, podman or installing the containers-common rpm, so only checking if the file exists is not enough, because it will always exist, also the customer can define a file with another name with the field default-docker inside.
	// it is needed to check the content of the registries.d files to see if there is a default-docker, if there is a default-docker it is needed to check if something was changed from the auto-generated default content.
	if _, err := os.Stat(registryFileAbsPath); errors.Is(err, os.ErrNotExist) {
		return createRegistryConfigFile(registryFileAbsPath, registryHost)
	}

	return nil
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
					UseSigstoreAttachments: true,
				},
			},
			DefaultDocker: nil,
		}
	} else {
		registryConfigStruct = registryConfiguration{
			DefaultDocker: &registryNamespace{UseSigstoreAttachments: true},
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
