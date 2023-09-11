package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
)

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

func toTar(img v1.Image, filename string) error {
	tr := mutate.Extract(img)
	outFile, err := os.Create(filename)
	// handle err
	if err != nil {
		return err
	}
	defer outFile.Close()
	_, err = io.Copy(outFile, tr)
	return err
}

func main() {
	argsWithoutProg := os.Args[1:]
	var img v1.Image
	var err error

	remoteOpts := getCraneOpts(context.TODO(), true)
	img, err = crane.Pull(argsWithoutProg[0], remoteOpts...)
	if err != nil {
		fmt.Printf("unable to pull image from %s: %v", argsWithoutProg[0], err)
	}

	// if we get here and no image was found bail out
	if img == nil {
		fmt.Printf("unable to obtain image for %v", argsWithoutProg[0])
	}
	err = toTar(img, argsWithoutProg[1])
	if err != nil {
		fmt.Printf("%v", err)
	}
}
