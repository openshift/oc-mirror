// Package proxy provides a minimal HTTPS forward proxy used by integration
// tests to verify that oc-mirror routes its network traffic through
// HTTP_PROXY/HTTPS_PROXY. It only supports CONNECT tunneling: oc-mirror's
// only plain-HTTP traffic targets its own local cache on localhost, which
// Go's net/http never routes through a configured proxy, so there is no
// plain-HTTP destination for this proxy to ever handle. It records every
// host it has been asked to tunnel traffic to.
package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Proxy is a minimal forward proxy for testing purposes.
type Proxy struct {
	listener net.Listener
	server   *http.Server

	mu       sync.Mutex
	hosts    map[string]struct{}
	serveErr error
}

// Start starts a forward proxy listening on an OS-assigned local port.
func Start() (*Proxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	p := &Proxy{listener: ln, hosts: make(map[string]struct{})}
	p.server = &http.Server{Handler: http.HandlerFunc(p.handleConnect)}
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

// handleConnect handles HTTPS tunneling: it dials the requested host and
// pipes bytes between the client and destination once the tunnel is established.
func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		http.Error(w, "only CONNECT is supported", http.StatusMethodNotAllowed)
		return
	}

	p.recordHost(r.Host)

	dialCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var dialer net.Dialer
	destConn, err := dialer.DialContext(dialCtx, "tcp", r.Host)
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
