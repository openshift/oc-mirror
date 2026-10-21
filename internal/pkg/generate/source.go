package generate

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/operator-framework/operator-registry/alpha/model"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/manifest"
	dockerconfig "go.podman.io/image/v5/pkg/docker/config"
	"go.podman.io/image/v5/types"
)

// catalogLabel is set by OLM on operator catalog (file-based catalog) images.
const catalogLabel = "operators.operatorframework.io.index.configs.v1"

// CatalogLoader loads the declarative config of a catalog image reference.
type CatalogLoader func(ctx context.Context, catalogRef string) (model.Model, error)

// RegistrySource reads a live registry. Auth and TLS settings come from SysCtx,
// the same system context oc-mirror uses for source registries.
type RegistrySource struct {
	Host string
	// Repos, when set, replaces /v2/_catalog enumeration.
	Repos       []string
	SysCtx      *types.SystemContext
	LoadCatalog CatalogLoader
}

var _ Source = (*RegistrySource)(nil)

// insecure reports whether TLS verification is disabled in SysCtx.
func (s *RegistrySource) insecure() bool {
	return s.SysCtx != nil && s.SysCtx.DockerInsecureSkipTLSVerify == types.OptionalBoolTrue
}

// Repositories returns Repos when set, otherwise lists /v2/_catalog.
func (s *RegistrySource) Repositories(ctx context.Context) ([]string, error) {
	if len(s.Repos) > 0 {
		return s.Repos, nil
	}
	var nameOpts []name.Option
	if s.insecure() {
		nameOpts = append(nameOpts, name.Insecure)
	}
	reg, err := name.NewRegistry(s.Host, nameOpts...)
	if err != nil {
		return nil, fmt.Errorf("parsing registry %q: %w", s.Host, err)
	}
	opts := []remote.Option{remote.WithContext(ctx), remote.WithAuth(s.auth())}
	if s.insecure() {
		tr, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, errors.New("default HTTP transport is not an *http.Transport")
		}
		tr = tr.Clone()
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user passed --src-tls-verify=false
		opts = append(opts, remote.WithTransport(tr))
	}
	repos, err := remote.Catalog(ctx, reg, opts...)
	if err != nil {
		return nil, fmt.Errorf("listing repositories on %s (is /v2/_catalog enabled? otherwise pass --repos): %w", s.Host, err)
	}
	return repos, nil
}

// auth returns the stored credentials for Host, or anonymous when none exist.
func (s *RegistrySource) auth() authn.Authenticator { //nolint:ireturn // remote.WithAuth requires the interface
	creds, err := dockerconfig.GetCredentials(s.SysCtx, s.Host)
	if err != nil || creds == (types.DockerAuthConfig{}) {
		return authn.Anonymous
	}
	return authn.FromConfig(authn.AuthConfig{
		Username: creds.Username, Password: creds.Password, IdentityToken: creds.IdentityToken,
	})
}

// Tags lists the tags of repo.
func (s *RegistrySource) Tags(ctx context.Context, repo string) ([]string, error) {
	ref, err := docker.ParseReference(fmt.Sprintf("//%s/%s", s.Host, repo))
	if err != nil {
		return nil, fmt.Errorf("parsing reference for %s/%s: %w", s.Host, repo, err)
	}
	tags, err := docker.GetRepositoryTags(ctx, s.SysCtx, ref)
	if err != nil {
		return nil, fmt.Errorf("listing tags of %s/%s: %w", s.Host, repo, err)
	}
	return tags, nil
}

// inspectCtx returns a copy of SysCtx whose OSChoice defaults to linux, so
// inspecting multi-arch indexes does not depend on the host OS. The caller's
// struct is not modified.
func (s *RegistrySource) inspectCtx() *types.SystemContext {
	var sc types.SystemContext
	if s.SysCtx != nil {
		sc = *s.SysCtx
	}
	if sc.OSChoice == "" {
		sc.OSChoice = "linux"
	}
	return &sc
}

// IsCatalog reports whether repo:tag carries the OLM catalog label.
func (s *RegistrySource) IsCatalog(ctx context.Context, repo, tag string) (bool, error) {
	ref, err := docker.ParseReference(fmt.Sprintf("//%s/%s:%s", s.Host, repo, tag))
	if err != nil {
		return false, fmt.Errorf("parsing reference for %s/%s:%s: %w", s.Host, repo, tag, err)
	}
	img, err := ref.NewImage(ctx, s.inspectCtx())
	if err != nil {
		return false, fmt.Errorf("opening %s/%s:%s: %w", s.Host, repo, tag, err)
	}
	defer img.Close()
	info, err := img.Inspect(ctx)
	if err != nil {
		var nonImage manifest.NonImageArtifactError
		if errors.As(err, &nonImage) {
			return false, nil // OCI artifact (Helm chart, signature, ...), not a catalog
		}
		return false, fmt.Errorf("inspecting %s/%s:%s: %w", s.Host, repo, tag, err)
	}
	_, ok := info.Labels[catalogLabel]
	return ok, nil
}

// CatalogModel loads the declarative config of repo:tag through LoadCatalog.
func (s *RegistrySource) CatalogModel(ctx context.Context, repo, tag string) (model.Model, error) {
	if s.LoadCatalog == nil {
		return nil, errors.New("no catalog loader configured")
	}
	m, err := s.LoadCatalog(ctx, fmt.Sprintf("%s/%s:%s", s.Host, repo, tag))
	if err != nil {
		return nil, fmt.Errorf("loading catalog %s/%s:%s: %w", s.Host, repo, tag, err)
	}
	return m, nil
}
