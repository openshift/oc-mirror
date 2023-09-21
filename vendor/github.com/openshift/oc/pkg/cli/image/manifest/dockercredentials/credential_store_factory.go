package dockercredentials

import (
	"fmt"
	"net/url"
	"sync"

	"github.com/containers/image/v5/docker/reference"
	"github.com/distribution/distribution/v3/registry/client/auth"
	"github.com/docker/docker/api/types"
	dockerregistry "github.com/docker/docker/registry"

	"github.com/openshift/library-go/pkg/image/registryclient"
)

// NewCredentialStoreFactory returns an entity capable of creating a CredentialStore
func NewCredentialStoreFactory(path string, refreshToken bool) (registryclient.CredentialStoreFactory, error) {
	fmt.Println("Code based on PR #1540 https://github.com/openshift/oc/pull/1540")
	authResolver, err := NewAuthResolver(path)
	if err != nil {
		return nil, err
	}

	return &credentialStoreFactory{authResolver, refreshToken}, nil
}

type credentialStoreFactory struct {
	authResolver *AuthResolver
	refreshToken bool
}

func (c *credentialStoreFactory) CredentialStoreFor(image string) auth.CredentialStore {
	nocreds := registryclient.NoCredentials
	if c.authResolver == nil {
		return nocreds
	}

	ref, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return nocreds
	}

	authCfg, err := c.authResolver.findAuthentication(ref, reference.Domain(ref))
	if err != nil {
		return nocreds
	}

	if c.refreshToken {
		return NewDynamicCredentialStore(&types.AuthConfig{
			Username:      authCfg.Username,
			Password:      authCfg.Password,
			IdentityToken: authCfg.IdentityToken,
		})
	}
	return dockerregistry.NewStaticCredentialStore(&types.AuthConfig{
		Username:      authCfg.Username,
		Password:      authCfg.Password,
		IdentityToken: authCfg.IdentityToken,
	})
}

func NewDynamicCredentialStore(auth *types.AuthConfig) auth.CredentialStore {
	return &DynamicCredentialStore{authConfig: auth}
}

type DynamicCredentialStore struct {
	authConfig *types.AuthConfig
	mutex      sync.Mutex
}

func (dcs *DynamicCredentialStore) Basic(*url.URL) (string, string) {
	if dcs.authConfig == nil {
		return "", ""
	}
	dcs.mutex.Lock()
	defer dcs.mutex.Unlock()

	return dcs.authConfig.Username, dcs.authConfig.Password
}

func (dcs *DynamicCredentialStore) RefreshToken(*url.URL, string) string {
	if dcs.authConfig == nil {
		return ""
	}
	dcs.mutex.Lock()
	defer dcs.mutex.Unlock()

	return dcs.authConfig.IdentityToken
}

func (dcs *DynamicCredentialStore) SetRefreshToken(u *url.URL, service, token string) {
	if dcs.authConfig != nil {
		dcs.mutex.Lock()
		defer dcs.mutex.Unlock()

		dcs.authConfig.IdentityToken = token
	}
}
