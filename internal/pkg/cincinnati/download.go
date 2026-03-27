package cincinnati

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

const (
	// Timeout when calling upstream Cincinnati stack.
	getUpdatesTimeout = time.Minute * 60
	// UpdateURL is the Cincinnati endpoint for the OpenShift platform.
	OcpUpdateURL = "https://api.openshift.com/api/upgrades_info/v1/graph"
	// OkdUpdateURL is the Cincinnati endpoint for the OKD platform.
	OkdUpdateURL = "https://origin-release.ci.openshift.org/graph"
)

type Option func(*options) error

type options struct {
	id            uuid.UUID
	cincinnatiURL *url.URL
	arch          string
	channel       string
	version       string
	transport     *http.Transport
}

func makeOptions(opts ...Option) (*options, error) {
	o := &options{
		id: uuid.New(),
	}

	for _, option := range opts {
		if err := option(o); err != nil {
			return nil, err
		}
	}

	if len(o.channel) == 0 {
		return nil, errors.New("channel value must be set")
	}

	if o.transport == nil {
		t, err := defaultTransport()
		if err != nil {
			return nil, err
		}
		o.transport = t
	}

	if o.cincinnatiURL == nil {
		u, err := url.Parse(OcpUpdateURL)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", OcpUpdateURL, err)
		}
		o.cincinnatiURL = u
	}

	return o, nil
}

func WithTransport(t *http.Transport) Option {
	return func(o *options) error {
		o.transport = t
		return nil
	}
}

func WithArch(arch string) Option {
	return func(o *options) error {
		o.arch = arch
		return nil
	}
}

func WithChannel(channel string) Option {
	return func(o *options) error {
		o.channel = channel
		return nil
	}
}

func WithVersion(version string) Option {
	return func(o *options) error {
		o.version = version
		return nil
	}
}

func WithURL(addr string) Option {
	return func(o *options) error {
		u, err := url.Parse(addr)
		if err != nil {
			return fmt.Errorf("failed to parse url: %w", err)
		}
		o.cincinnatiURL = u
		return nil
	}
}

func WithID(id uuid.UUID) Option {
	return func(o *options) error {
		o.id = id
		return nil
	}
}

func defaultTransport() (*http.Transport, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("failed to get system cert pool: %w", err)
	}

	return &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    certPool,
			MinVersion: tls.VersionTLS12,
		},
		Proxy: http.ProxyFromEnvironment,
	}, nil
}

func (o options) buildURL() (*url.URL, error) {
	upstream := *o.cincinnatiURL

	query := upstream.Query()
	query.Set("id", o.id.String())

	if len(o.arch) > 0 {
		query.Set("arch", o.arch)
	}

	if len(o.channel) > 0 {
		query.Set("channel", o.channel)
	}

	if len(o.version) > 0 {
		query.Set("version", o.version)
	}

	upstream.RawQuery = query.Encode()

	return &upstream, nil
}

func (o options) buildRequest(ctx context.Context, log clog.PluggableLoggerInterface) (*http.Request, error) {
	upstream, err := o.buildURL()
	if err != nil {
		return nil, err
	}
	uri := upstream.String()

	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}
	req.Header.Add("Accept", "application/json")

	if o.transport.TLSClientConfig != nil && o.transport.TLSClientConfig.ClientCAs == nil {
		log.Debug("Using a root CA pool with 0 root CA subjects to request updates from %s", uri)
	}

	if o.transport.Proxy != nil {
		if proxy, err := o.transport.Proxy(req); err == nil && proxy != nil {
			log.Debug("Using proxy %s to request updates from %s", proxy.Host, uri)
		}
	}

	return req, nil
}

// DownloadGraphData downloads the Cincinnati graph data.
func DownloadGraphData(ctx context.Context, log clog.PluggableLoggerInterface, opts ...Option) ([]byte, error) {
	o, err := makeOptions(opts...)
	if err != nil {
		return nil, err
	}

	req, err := o.buildRequest(ctx, log)
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, getUpdatesTimeout)
	defer cancel()

	client := http.Client{Transport: o.transport}
	resp, err := client.Do(req.WithContext(timeoutCtx)) //nolint:gosec // G704: URL validated at construction time
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return body, nil
}
