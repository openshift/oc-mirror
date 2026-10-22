package registry

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
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
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/sirupsen/logrus"
)

// Registry manages an in-process distribution/distribution registry instance.
type Registry struct {
	service     *dregistry.Registry
	storagePath string
	port        int
	client      name.Registry
	listenErr   chan error
	tls         *selfSignedTLS
	tlsDir      string
}

// Option configures a Registry started by Start.
type Option func(*options)

type options struct {
	selfSignedTLS bool
	tlsHosts      []string
}

// WithSelfSignedTLS serves the registry over HTTPS with a certificate for localhost
// and extraHosts, signed by a CA generated for this registry. Clients other than
// the Registry methods must trust the CA in CACertPath.
func WithSelfSignedTLS(extraHosts ...string) Option {
	return func(o *options) {
		o.selfSignedTLS = true
		o.tlsHosts = extraHosts
	}
}

// Start parses the config file, creates an in-process registry, and starts it in the background.
// Pass port 0 to let the OS assign a free port automatically.
// Without options, the registry serves plain HTTP.
func Start(ctx context.Context, configPath string, port int, opts ...Option) (*Registry, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	if port == 0 {
		p, err := freePort()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate a free port: %w", err)
		}
		port = p
	}

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

	var tlsFiles *selfSignedTLS
	var tlsDir string
	if o.selfSignedTLS {
		tlsDir, err = os.MkdirTemp("", "oc-mirror-registry-tls-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp TLS dir: %w", err)
		}
		tlsFiles, err = generateSelfSignedTLS(tlsDir, o.tlsHosts)
		if err != nil {
			return nil, fmt.Errorf("failed to generate TLS certificates: %w", err)
		}
		config.HTTP.TLS.Certificate = tlsFiles.certPath
		config.HTTP.TLS.Key = tlsFiles.keyPath
	}

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

	r := &Registry{
		service:     service,
		storagePath: storagePath,
		port:        port,
		listenErr:   listenErr,
		tls:         tlsFiles,
		tlsDir:      tlsDir,
	}
	r.client, err = name.NewRegistry(r.Endpoint(), r.nameOptions()...)
	if err != nil {
		return nil, fmt.Errorf("failed to set up registry client: %w", err)
	}
	return r, nil
}

// CACertPath returns the path of the PEM-encoded CA certificate that signed the
// registry's certificate, or "" when the registry serves plain HTTP.
func (r *Registry) CACertPath() string {
	if r.tls == nil {
		return ""
	}
	return r.tls.caCertPath
}

// RootCAs returns the pool containing the CA that signed the registry's
// certificate, or nil when the registry serves plain HTTP.
func (r *Registry) RootCAs() *x509.CertPool {
	if r.tls == nil {
		return nil
	}
	return r.tls.caPool
}

// nameOptions returns the options to parse references to images in this registry.
func (r *Registry) nameOptions() []name.Option {
	if r.tls == nil {
		return []name.Option{name.Insecure}
	}
	return nil
}

// RemoteOptions returns the go-containerregistry options to read from or write
// to this registry: anonymous access, and trust of its CA when it serves HTTPS.
func (r *Registry) RemoteOptions(ctx context.Context) []remote.Option {
	opts := []remote.Option{remote.WithAuth(authn.Anonymous), remote.WithContext(ctx)}
	if r.tls != nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{RootCAs: r.tls.caPool, MinVersion: tls.VersionTLS12}
		opts = append(opts, remote.WithTransport(transport))
	}
	return opts
}

// Reference parses a reference ("<repository>:<tag>" or "<repository>@<digest>")
// to an image in this registry.
func (r *Registry) Reference(repoRef string) (name.Reference, error) {
	return name.ParseReference(r.Endpoint()+"/"+repoRef, r.nameOptions()...)
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

		_, err := remote.Catalog(ctx, r.client, r.RemoteOptions(ctx)...)
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
	if r.tlsDir != "" {
		if err := os.RemoveAll(r.tlsDir); err != nil {
			return fmt.Errorf("failed to remove registry TLS files: %w (shutdown: %v)", err, shutdownErr)
		}
	}

	return shutdownErr
}

// Endpoint returns the registry endpoint address.
func (r *Registry) Endpoint() string {
	return fmt.Sprintf("localhost:%d", r.port)
}

// ListRepositories returns all repositories in the registry.
func (r *Registry) ListRepositories(ctx context.Context) ([]string, error) {
	return remote.Catalog(ctx, r.client, r.RemoteOptions(ctx)...)
}

// ListTags returns all the tags for a given repository in the registry.
func (r *Registry) ListTags(ctx context.Context, repo string) ([]string, error) {
	ref, err := name.NewRepository(fmt.Sprintf("%s/%s", r.Endpoint(), repo), r.nameOptions()...)
	if err != nil {
		return nil, fmt.Errorf("couldn't list tags for repo %s: %w", repo, err)
	}

	return remote.List(ref, r.RemoteOptions(ctx)...)
}

// ManifestExistsByDigest reports whether a manifest with the given digest can be
// resolved in the repository. It resolves the image strictly by digest (repo@sha256:...),
// so it detects images that were pushed by revision even when no tag points at them.
// A missing manifest (HTTP 404 / MANIFEST_UNKNOWN) returns (false, nil); any other
// error is returned to the caller.
func (r *Registry) ManifestExistsByDigest(ctx context.Context, repo, digest string) (bool, error) {
	ref, err := name.NewDigest(fmt.Sprintf("%s/%s@%s", r.Endpoint(), repo, digest), r.nameOptions()...)
	if err != nil {
		return false, fmt.Errorf("couldn't create digest reference for %s@%s: %w", repo, digest, err)
	}

	_, err = remote.Head(ref, r.RemoteOptions(ctx)...)
	if err != nil {
		var terr *transport.Error
		if errors.As(err, &terr) && terr.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, fmt.Errorf("couldn't resolve manifest %s@%s: %w", repo, digest, err)
	}
	return true, nil
}

// TagDigest resolves the manifest digest that the given repository tag points to
// (e.g. "sha256:abcd..."). It is used to assert that distinct tags reference the same
// underlying digest.
func (r *Registry) TagDigest(ctx context.Context, repo, tag string) (string, error) {
	ref, err := name.NewTag(fmt.Sprintf("%s/%s:%s", r.Endpoint(), repo, tag), r.nameOptions()...)
	if err != nil {
		return "", fmt.Errorf("couldn't create tag reference for %s:%s: %w", repo, tag, err)
	}

	desc, err := remote.Head(ref, r.RemoteOptions(ctx)...)
	if err != nil {
		return "", fmt.Errorf("couldn't resolve tag %s:%s: %w", repo, tag, err)
	}
	return desc.Digest.String(), nil
}

// IsCatalog returns true if the given repository and tag is an OLM operator catalog image.
func (r *Registry) IsCatalog(ctx context.Context, repo, tag string) (bool, error) {
	ref, err := name.NewTag(fmt.Sprintf("%s/%s:%s", r.Endpoint(), repo, tag), r.nameOptions()...)
	if err != nil {
		return false, fmt.Errorf("couldn't create tag reference for %s:%s: %w", repo, tag, err)
	}

	img, err := remote.Image(ref, r.RemoteOptions(ctx)...)
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

func freePort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
