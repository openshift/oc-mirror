package config

import (
	"errors"
	"os"
	"path/filepath"

	dockercfg "github.com/docker/cli/cli/config"
	"github.com/openshift/library-go/pkg/image/registryclient"
	imagemanifest "github.com/openshift/oc/pkg/cli/image/manifest"
)

// TODO: create a context based on user provided
// pull secret

// CreateDefaultContext a default context for the registryClient of `oc mirror`
func CreateDefaultContext(skipTLS bool) (*registryclient.Context, error) {
	opts := &imagemanifest.SecurityOptions{}
	opts.Insecure = skipTLS

	dockerConfigJSON := filepath.Join(dockercfg.Dir(), dockercfg.ConfigFileName)
	switch _, err := os.Stat(dockerConfigJSON); {
	case err == nil:
		opts.RegistryConfig = dockerConfigJSON
	case errors.Is(err, os.ErrNotExist):
		podmanConfig := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "containers/auth.json")
		if _, err := os.Stat(podmanConfig); err == nil {
			opts.RegistryConfig = podmanConfig
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return opts.Context()
}
