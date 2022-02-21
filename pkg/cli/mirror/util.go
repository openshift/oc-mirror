package mirror

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func getRemoteOpts(ctx context.Context, insecure bool) []remote.Option {
	return []remote.Option{
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithTransport(createRT(insecure)),
		remote.WithContext(ctx),
	}
}

func getNameOpts(insecure bool) (options []name.Option) {
	if insecure {
		options = append(options, name.Insecure)
	}
	return options
}

func createRT(insecure bool) http.RoundTripper {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			// By default, we wrap the transport in retries, so reduce the
			// default dial timeout to 5s to avoid 5x 30s of connection
			// timeouts when doing the "ping" on certain http registries.
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure,
		},
	}
}

func (o *MirrorOptions) createResultsDir() (resultsDir string, err error) {
	resultsDir = filepath.Join(
		o.Dir,
		fmt.Sprintf("results-%v", time.Now().Unix()),
	)
	if err := os.MkdirAll(resultsDir, os.ModePerm); err != nil {
		return resultsDir, err
	}
	return resultsDir, nil
}

func (o *MirrorOptions) newMetadataImage(uid string) string {
	repo := path.Join(o.ToMirror, o.UserNamespace, "oc-mirror")
	return fmt.Sprintf("%s:%s", repo, uid)
}
