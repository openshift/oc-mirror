// Package proxy provides a minimal forward proxy used by integration tests to
// verify that oc-mirror routes its network traffic through HTTP_PROXY/HTTPS_PROXY.
// It tunnels HTTPS traffic (CONNECT) and forwards plain HTTP requests, and records
// every host it has been asked to forward traffic to.
package proxy

import (
	"context"
	"errors"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Proxy is a minimal forward proxy for testing purposes.
type Proxy struct {
	listener net.Listener
	server   *http.Server
	routes   map[string]string

	mu       sync.Mutex
	hosts    map[string]struct{}
	serveErr error
}

// Start starts a forward proxy listening on an OS-assigned local port.
func Start() (*Proxy, error) {
	return StartWithRoutes(nil)
}

// StartWithRoutes redirects requested host:port pairs to test-owned endpoints.
// A non-nil map rejects destinations without a route, preventing live fallback.
// Passing nil preserves the normal forward-proxy behavior of Start.
func StartWithRoutes(routes map[string]string) (*Proxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	p := &Proxy{listener: ln, hosts: make(map[string]struct{}), routes: maps.Clone(routes)}
	p.server = &http.Server{Handler: http.HandlerFunc(p.handle)}
	go func() {
		// Save any error other than the expected one from Stop closing the server.
		if err := p.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			p.mu.Lock()
			p.serveErr = err
			p.mu.Unlock()
		}
	}()

	return p, nil
}

// Err returns any unexpected error from serving proxy connections, or nil.
func (p *Proxy) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.serveErr
}

// URL returns the proxy's base URL, suitable for HTTP_PROXY/HTTPS_PROXY.
func (p *Proxy) URL() string {
	return "http://" + p.listener.Addr().String()
}

// Stop closes the proxy listener and any open connections.
func (p *Proxy) Stop() error {
	return p.server.Close()
}

// SawHost reports whether any host the proxy forwarded traffic to contains substr.
func (p *Proxy) SawHost(substr string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for h := range p.hosts {
		if strings.Contains(h, substr) {
			return true
		}
	}
	return false
}

// Hosts returns every distinct host the proxy has forwarded traffic to, for
// diagnostics when an assertion fails.
func (p *Proxy) Hosts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	hosts := make([]string, 0, len(p.hosts))
	for h := range p.hosts {
		hosts = append(hosts, h)
	}
	return hosts
}

func (p *Proxy) recordHost(hostport string) {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	p.mu.Lock()
	p.hosts[host] = struct{}{}
	p.mu.Unlock()
}

func (p *Proxy) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handleHTTP(w, r)
}

// destination returns where to forward traffic for hostport (host:port), applying
// the routes, if any.
func (p *Proxy) destination(hostport string) (string, bool) {
	if p.routes == nil {
		return hostport, true
	}
	destination, ok := p.routes[hostport]
	return destination, ok
}

// handleHTTP forwards a plain HTTP request, whose URL is absolute.
func (p *Proxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Host == "" {
		http.Error(w, "not a proxy request", http.StatusBadRequest)
		return
	}
	hostport := r.URL.Host
	if r.URL.Port() == "" {
		hostport = net.JoinHostPort(r.URL.Hostname(), "80")
	}
	p.recordHost(hostport)
	destination, ok := p.destination(hostport)
	if !ok {
		http.Error(w, "no test route for "+hostport, http.StatusBadGateway)
		return
	}

	target := &url.URL{Scheme: "http", Host: destination}
	(&httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = r.Host
		},
	}).ServeHTTP(w, r)
}

// handleConnect handles HTTPS tunneling: it dials the requested host and
// pipes bytes between the client and destination once the tunnel is established.
func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	p.recordHost(r.Host)
	destination, ok := p.destination(r.Host)
	if !ok {
		http.Error(w, "no test route for "+r.Host, http.StatusBadGateway)
		return
	}

	dialCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var dialer net.Dialer
	destConn, err := dialer.DialContext(dialCtx, "tcp", destination)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	closeDest := func() { _ = destConn.Close() }
	defer closeDest()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	closeClient := func() { _ = clientConn.Close() }
	defer closeClient()

	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return
	}

	// Close both connections once either side finishes, so a stuck peer
	// transfer can't block the handler indefinitely.
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(destConn, clientConn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(clientConn, destConn)
		done <- struct{}{}
	}()

	<-done
	closeDest()
	closeClient()
	<-done
}

// UnusedAddr returns a local "host:port" address that is not currently being
// listened on, useful for simulating an unreachable proxy in negative-path tests.
func UnusedAddr() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		return "", err
	}
	return addr, nil
}
