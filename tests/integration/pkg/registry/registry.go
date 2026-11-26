package registry

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
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

// ocMirrorCachePort is the port oc-mirror binds its local cache registry to by default.
const ocMirrorCachePort = 55000

// Registry manages an in-process distribution/distribution registry instance.
type Registry struct {
	config      *configuration.Configuration
	service     *dregistry.Registry
	storageRoot string
	host        string
	port        int
	nameOptions []name.Option
	transport   http.RoundTripper
	certificate []byte
	client      name.Registry
	listenErr   chan error
}

// Start parses the config file, creates an in-process registry serving plain HTTP on localhost,
// and starts it in the background. Pass port 0 to let the OS assign a free port automatically.
func Start(ctx context.Context, configPath string, port int) (*Registry, error) {
	return start(ctx, configPath, port, false)
}

// StartTLS is like Start, but the registry serves HTTPS on 127.0.0.1 with a freshly generated
// self-signed certificate. Clients must trust Certificate to connect to it.
func StartTLS(ctx context.Context, configPath string, port int) (*Registry, error) {
	return start(ctx, configPath, port, true)
}

func start(ctx context.Context, configPath string, port int, useTLS bool) (*Registry, error) {
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
	config.Storage["filesystem"] = configuration.Parameters{"rootdirectory": filepath.Join(storageRoot, "storage")}
	config.HTTP.Addr = fmt.Sprintf(":%d", port)

	r := &Registry{
		config:      config,
		storageRoot: storageRoot,
		host:        "localhost",
		port:        port,
		nameOptions: []name.Option{name.Insecure},
		transport:   remote.DefaultTransport,
	}
	if useTLS {
		if err := r.configureTLS(); err != nil {
			return nil, errors.Join(err, os.RemoveAll(storageRoot))
		}
	}

	client, err := name.NewRegistry(r.Endpoint(), r.nameOptions...)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to set up registry client: %w", err), os.RemoveAll(storageRoot))
	}
	r.client = client

	if err := r.serve(ctx); err != nil {
		return nil, errors.Join(err, os.RemoveAll(storageRoot))
	}
	return r, nil
}

// configureTLS generates the registry certificate and a client transport that trusts it.
func (r *Registry) configureTLS() error {
	r.host = "127.0.0.1"
	certificate, key, err := selfSignedCertificate(r.host)
	if err != nil {
		return err
	}

	tlsDir := filepath.Join(r.storageRoot, "tls")
	if err := os.MkdirAll(tlsDir, 0o700); err != nil {
		return fmt.Errorf("failed to create TLS dir: %w", err)
	}
	r.config.HTTP.TLS.Certificate = filepath.Join(tlsDir, "registry.crt")
	r.config.HTTP.TLS.Key = filepath.Join(tlsDir, "registry.key")
	if err := os.WriteFile(r.config.HTTP.TLS.Certificate, certificate, 0o600); err != nil {
		return fmt.Errorf("failed to write registry certificate: %w", err)
	}
	if err := os.WriteFile(r.config.HTTP.TLS.Key, key, 0o600); err != nil {
		return fmt.Errorf("failed to write registry key: %w", err)
	}

	r.certificate = certificate
	clientTransport := remote.DefaultTransport.(*http.Transport).Clone()
	clientTransport.TLSClientConfig = r.ClientTLSConfig()
	r.nameOptions = nil
	r.transport = clientTransport
	return nil
}

// serve starts the registry service in the background, reusing the stored content.
func (r *Registry) serve(ctx context.Context) error {
	health.DefaultRegistry = health.NewRegistry()
	os.Setenv("OTEL_TRACES_EXPORTER", "none")
	logrus.SetOutput(io.Discard)

	service, err := dregistry.NewRegistry(ctx, r.config)
	if err != nil {
		return fmt.Errorf("failed to create registry: %w", err)
	}

	listenErr := make(chan error, 1)
	go func() {
		if err := service.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			listenErr <- err
		}
		close(listenErr)
	}()

	r.service = service
	r.listenErr = listenErr
	return nil
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

		_, err := remote.Catalog(ctx, r.client, r.remoteOptions(ctx)...)
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

// Suspend shuts the registry down but keeps its content, so clients get connection
// errors as if it were unreachable until Resume is called.
func (r *Registry) Suspend() error {
	return r.shutdown()
}

// Resume restarts a suspended registry on the same address, with the same content.
func (r *Registry) Resume(ctx context.Context) error {
	if err := r.serve(ctx); err != nil {
		return err
	}
	return r.WaitReady(ctx, 30*time.Second)
}

// Stop gracefully shuts down the registry and removes its storage.
func (r *Registry) Stop() error {
	var shutdownErr error
	if r.service != nil {
		shutdownErr = r.shutdown()
	}

	if err := os.RemoveAll(r.storageRoot); err != nil {
		return fmt.Errorf("failed to remove registry storage: %w (shutdown: %v)", err, shutdownErr)
	}

	return shutdownErr
}

// shutdown gracefully shuts the registry service down.
func (r *Registry) shutdown() error {
	// The HTTP server waits up to 5 seconds for the connections that have not sent a
	// request yet, like the unused connections a client opens for parallel requests.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return r.service.Shutdown(ctx)
}

// Endpoint returns the registry endpoint address.
func (r *Registry) Endpoint() string {
	return fmt.Sprintf("%s:%d", r.host, r.port)
}

// Certificate returns the PEM encoded certificate of a registry started with StartTLS.
func (r *Registry) Certificate() []byte {
	return r.certificate
}

// Transport returns an HTTP transport that can connect to the registry.
func (r *Registry) Transport() http.RoundTripper {
	return r.transport
}

// ClientTLSConfig returns a new client TLS configuration that trusts the certificate of a
// registry started with StartTLS, in addition to the system certificate authorities.
func (r *Registry) ClientTLSConfig() *tls.Config {
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	pool.AppendCertsFromPEM(r.certificate)
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

// InstallCertificate makes the container tools of the user whose home directory is home, like
// oc-mirror, trust the certificate of a registry started with StartTLS. Those tools read the
// certificate authorities of a registry from ~/.config/containers/certs.d/<host:port>/.
func (r *Registry) InstallCertificate(home string) error {
	if r.certificate == nil {
		return errors.New("the registry does not serve HTTPS")
	}
	certsDir := filepath.Join(home, ".config", "containers", "certs.d", r.Endpoint())
	if err := os.MkdirAll(certsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create %s: %w", certsDir, err)
	}
	return os.WriteFile(filepath.Join(certsDir, "ca.crt"), r.certificate, 0o644)
}

func (r *Registry) remoteOptions(ctx context.Context) []remote.Option {
	return []remote.Option{remote.WithAuth(authn.Anonymous), remote.WithTransport(r.transport), remote.WithContext(ctx)}
}

// ListRepositories returns all repositories in the registry.
func (r *Registry) ListRepositories(ctx context.Context) ([]string, error) {
	return remote.Catalog(ctx, r.client, r.remoteOptions(ctx)...)
}

// ListTags returns all the tags for a given repository in the registry.
func (r *Registry) ListTags(ctx context.Context, repo string) ([]string, error) {
	ref, err := name.NewRepository(fmt.Sprintf("%s/%s", r.Endpoint(), repo), r.nameOptions...)
	if err != nil {
		return nil, fmt.Errorf("couldn't list tags for repo %s: %w", repo, err)
	}

	return remote.List(ref, r.remoteOptions(ctx)...)
}

// ManifestExistsByDigest reports whether a manifest with the given digest can be
// resolved in the repository. It resolves the image strictly by digest (repo@sha256:...),
// so it detects images that were pushed by revision even when no tag points at them.
// A missing manifest (HTTP 404 / MANIFEST_UNKNOWN) returns (false, nil); any other
// error is returned to the caller.
func (r *Registry) ManifestExistsByDigest(ctx context.Context, repo, digest string) (bool, error) {
	ref, err := name.NewDigest(fmt.Sprintf("%s/%s@%s", r.Endpoint(), repo, digest), r.nameOptions...)
	if err != nil {
		return false, fmt.Errorf("couldn't create digest reference for %s@%s: %w", repo, digest, err)
	}

	_, err = remote.Head(ref, r.remoteOptions(ctx)...)
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
	ref, err := name.NewTag(fmt.Sprintf("%s/%s:%s", r.Endpoint(), repo, tag), r.nameOptions...)
	if err != nil {
		return "", fmt.Errorf("couldn't create tag reference for %s:%s: %w", repo, tag, err)
	}

	desc, err := remote.Head(ref, r.remoteOptions(ctx)...)
	if err != nil {
		return "", fmt.Errorf("couldn't resolve tag %s:%s: %w", repo, tag, err)
	}
	return desc.Digest.String(), nil
}

// IsCatalog returns true if the given repository and tag is an OLM operator catalog image.
func (r *Registry) IsCatalog(ctx context.Context, repo, tag string) (bool, error) {
	ref, err := name.NewTag(fmt.Sprintf("%s/%s:%s", r.Endpoint(), repo, tag), r.nameOptions...)
	if err != nil {
		return false, fmt.Errorf("couldn't create tag reference for %s:%s: %w", repo, tag, err)
	}

	img, err := remote.Image(ref, r.remoteOptions(ctx)...)
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

// freePort returns a free local port, other than the one oc-mirror uses for its local cache.
func freePort() (int, error) {
	for {
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			return 0, err
		}
		port := l.Addr().(*net.TCPAddr).Port
		if err := l.Close(); err != nil {
			return 0, err
		}
		if port != ocMirrorCachePort {
			return port, nil
		}
	}
}

// selfSignedCertificate returns a PEM encoded certificate and key for host, valid for a day.
func selfSignedCertificate(host string) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate registry key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate certificate serial number: %w", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: host},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP(host)},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create registry certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal registry key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), nil
}
