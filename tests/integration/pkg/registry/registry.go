package registry

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/health"
	dregistry "github.com/distribution/distribution/v3/registry"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sirupsen/logrus"
)

// Registry manages an in-process distribution/distribution registry instance.
type Registry struct {
	service     *dregistry.Registry
	storagePath string
	port        int
	client      name.Registry
	listenErr   chan error
}

// Start parses the config file, creates an in-process registry, and starts it in the background.
func Start(ctx context.Context, configPath string, port int) (*Registry, error) {
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read registry config: %w", err)
	}

	config, err := configuration.Parse(bytes.NewReader(configData))
	if err != nil {
		return nil, fmt.Errorf("failed to parse registry config: %w", err)
	}

	storageRoot, err := os.MkdirTemp("", "oc-mirror-registry-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp storage dir: %w", err)
	}
	config.Storage["filesystem"] = configuration.Parameters{"rootdirectory": storageRoot}
	config.HTTP.Addr = fmt.Sprintf(":%d", port)
	storagePath := filepath.Join(storageRoot, "docker")

	health.DefaultRegistry = health.NewRegistry()
	os.Setenv("OTEL_TRACES_EXPORTER", "none")
	logrus.SetOutput(io.Discard)

	service, err := dregistry.NewRegistry(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create registry: %w", err)
	}

	listenErr := make(chan error, 1)
	go func() {
		if err := service.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			listenErr <- err
		}
		close(listenErr)
	}()

	client, err := name.NewRegistry(fmt.Sprintf("localhost:%d", port), name.Insecure)
	if err != nil {
		return nil, fmt.Errorf("failed to set up registry client: %w", err)
	}

	return &Registry{
		service:     service,
		storagePath: storagePath,
		port:        port,
		client:      client,
		listenErr:   listenErr,
	}, nil
}

// WaitReady polls the registry until it responds or timeout is reached.
func (r *Registry) WaitReady(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-r.listenErr:
			if err != nil {
				return fmt.Errorf("registry failed to start: %w", err)
			}
		default:
		}

		_, err := remote.Catalog(ctx, r.client, remote.WithAuth(authn.Anonymous))
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("registry not ready after %v", timeout)
		case <-ticker.C:
		}
	}
}

// Stop gracefully shuts down the registry and removes its storage.
func (r *Registry) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var shutdownErr error
	if r.service != nil {
		shutdownErr = r.service.Shutdown(ctx)
	}

	if err := os.RemoveAll(r.storagePath); err != nil {
		return fmt.Errorf("failed to remove registry storage: %w (shutdown: %v)", err, shutdownErr)
	}

	return shutdownErr
}

// Endpoint returns the registry endpoint address.
func (r *Registry) Endpoint() string {
	return fmt.Sprintf("localhost:%d", r.port)
}

// ListRepositories returns all repositories in the registry.
func (r *Registry) ListRepositories(ctx context.Context) ([]string, error) {
	return remote.Catalog(ctx, r.client, remote.WithAuth(authn.Anonymous))
}

// ListTags returns all the tags for a given repository in the registry.
func (r *Registry) ListTags(ctx context.Context, repo string) ([]string, error) {
	ref, err := name.NewRepository(fmt.Sprintf("%s/%s", r.Endpoint(), repo), name.Insecure)
	if err != nil {
		return nil, fmt.Errorf("couldn't list tags for repo %s: %w", repo, err)
	}

	return remote.List(ref, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))
}

// IsCatalog returns true if the given repository and tag is an OLM operator catalog image.
func (r *Registry) IsCatalog(ctx context.Context, repo, tag string) (bool, error) {
	ref, err := name.NewTag(fmt.Sprintf("%s/%s:%s", r.Endpoint(), repo, tag), name.Insecure)
	if err != nil {
		return false, fmt.Errorf("couldn't create tag reference for %s:%s: %w", repo, tag, err)
	}

	img, err := remote.Image(ref, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("couldn't fetch image %s:%s: %w", repo, tag, err)
	}

	cf, err := img.ConfigFile()
	if err != nil {
		return false, fmt.Errorf("couldn't get config file for %s:%s: %w", repo, tag, err)
	}

	_, ok := cf.Config.Labels["operators.operatorframework.io.index.configs.v1"]
	return ok, nil
}
