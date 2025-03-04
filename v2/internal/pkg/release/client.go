package release

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/url"
	"os"

	"github.com/google/uuid"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

var (
	_ Client = &ocpClient{}
	_ Client = &okdClient{}
)

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

// Client is a Cincinnati client which can be used to fetch update graphs from
// an upstream Cincinnati stack.
type Client interface {
	GetURL() *url.URL
	SetQueryParams(arch, channel, version string)
	GetID() uuid.UUID
	GetTransport() *http.Transport
}

type ocpClient struct {
	id        uuid.UUID
	transport *http.Transport
	url       url.URL
}

type okdClient struct {
	id        uuid.UUID
	transport *http.Transport
	url       url.URL
}

// NewOCPClient creates a new OCP Cincinnati client with the given client identifier.
func NewOCPClient(id uuid.UUID, log clog.PluggableLoggerInterface) (Client, error) {
	var updateGraphURL string
	if updateURLOverride := os.Getenv("UPDATE_URL_OVERRIDE"); len(updateURLOverride) != 0 {
		log.Info("Using the UPDATE_URL_OVERRIDE environment variable")
		updateGraphURL = updateURLOverride
	} else {
		updateGraphURL = UpdateURL
	}
	upstream, err := url.Parse(updateGraphURL)
	if err != nil {
		return &ocpClient{}, err
	}

	tls, err := getTLSConfig()
	if err != nil {
		return &ocpClient{}, err
	}

	transport := &http.Transport{
		TLSClientConfig: tls,
		Proxy:           http.ProxyFromEnvironment,
	}
	return &ocpClient{id: id, transport: transport, url: *upstream}, nil
}

func (o *ocpClient) GetURL() *url.URL {
	return &o.url
}

func (o *ocpClient) GetTransport() *http.Transport {
	return o.transport
}

func (o *ocpClient) GetID() uuid.UUID {
	return o.id
}

func (o *ocpClient) SetQueryParams(arch, channel, version string) {
	queryParams := o.url.Query()

	queryParams.Set("id", o.id.String())
	params := map[string]string{
		"arch":    arch,
		"channel": channel,
		"version": version,
	}
	for key, value := range params {
		if value != "" {
			queryParams.Set(key, value)
		}
	}
	o.url.RawQuery = queryParams.Encode()
}

// NewOKDClient creates a new OKD Cincinnati client with the given client identifier.
func NewOKDClient(id uuid.UUID) (Client, error) {
	upstream, err := url.Parse(OkdUpdateURL)
	if err != nil {
		return &okdClient{}, err
	}

	tls, err := getTLSConfig()
	if err != nil {
		return &okdClient{}, err
	}

	transport := &http.Transport{
		TLSClientConfig: tls,
		Proxy:           http.ProxyFromEnvironment,
	}
	return &okdClient{id: id, transport: transport, url: *upstream}, nil
}

func (o *okdClient) GetURL() *url.URL {
	return &o.url
}

func (o *okdClient) GetID() uuid.UUID {
	return o.id
}

func (o *okdClient) GetTransport() *http.Transport {
	return o.transport
}

func (o *okdClient) SetQueryParams(_, _, _ string) {
	// Do nothing
}

func getTLSConfig() (*tls.Config, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	config := &tls.Config{
		RootCAs:    certPool,
		MinVersion: tls.VersionTLS12,
	}
	return config, nil
}
