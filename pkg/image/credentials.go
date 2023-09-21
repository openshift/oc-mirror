package image

import (
	"errors"
	"os"
	"path/filepath"

	dockercfg "github.com/docker/cli/cli/config"
	"github.com/openshift/library-go/pkg/image/registryclient"
	"github.com/openshift/oc/pkg/cli/image/manifest/dockercredentials"
	"k8s.io/client-go/rest"
)

// NewContext creates a context for the registryClient of `oc mirror`
func NewContext(skipVerification bool) (*registryclient.Context, error) {
	userAgent := rest.DefaultKubernetesUserAgent()
	rt, err := rest.TransportFor(&rest.Config{UserAgent: userAgent})
	if err != nil {
		return nil, err
	}
	insecureRT, err := rest.TransportFor(&rest.Config{TLSClientConfig: rest.TLSClientConfig{Insecure: true}, UserAgent: userAgent})
	if err != nil {
		return nil, err
	}

	ctx := registryclient.NewContext(rt, insecureRT)

	// Set default options
	var registryConfig string
	dockerConfigJSON := filepath.Join(dockercfg.Dir(), dockercfg.ConfigFileName)
	switch _, err := os.Stat(dockerConfigJSON); {
	case err == nil:
		registryConfig = dockerConfigJSON
	case errors.Is(err, os.ErrNotExist):
		podmanConfig := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "containers/auth.json")
		if _, err := os.Stat(podmanConfig); err == nil {
			registryConfig = podmanConfig
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}

	if len(registryConfig) != 0 {
		creds, err := dockercredentials.NewCredentialStoreFactory(registryConfig, true)
		if err != nil {
			return nil, err
		}
		ctx.WithCredentialsFactory(creds)
	}
	ctx.Retries = 3
	ctx.DisableDigestVerification = skipVerification
	return ctx, nil
}
