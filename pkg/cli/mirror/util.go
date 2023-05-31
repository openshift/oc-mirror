package mirror

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"k8s.io/klog/v2"
)

const mappingFile = "mapping.txt"

func getRemoteOpts(ctx context.Context, insecure bool) []remote.Option {
	return []remote.Option{
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithTransport(createRT(insecure)),
		remote.WithContext(ctx),
	}
}

func getCraneOpts(ctx context.Context, insecure bool) []crane.Option {
	opts := []crane.Option{
		crane.WithAuthFromKeychain(authn.DefaultKeychain),
		crane.WithTransport(createRT(insecure)),
		crane.WithContext(ctx),
	}
	if insecure {
		opts = append(opts, crane.Insecure)
	}
	return opts
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
			MinVersion:         tls.VersionTLS12,
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

func (o *MirrorOptions) checkErr(err error, acceptableErr func(error) bool, logMessage func(error) string) error {

	if err == nil {
		return nil
	}

	var skip, skipAllTypes bool
	if acceptableErr != nil {
		skip = acceptableErr(err)
	} else {
		skipAllTypes = true
	}

	message := err.Error()
	if logMessage != nil {
		message = logMessage(err)
	}

	// Instead of returning an error, just log it.
	if o.ContinueOnError && (skip || skipAllTypes) {
		klog.Errorf("error: %v", message)
		o.continuedOnError = true
	} else {
		return fmt.Errorf("%v", message)
	}

	return nil
}
