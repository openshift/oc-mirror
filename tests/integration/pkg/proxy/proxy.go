// Package proxy provides a minimal HTTPS forward proxy used by integration
// tests to verify that oc-mirror routes its network traffic through
// HTTP_PROXY/HTTPS_PROXY. It only supports CONNECT tunneling: oc-mirror's
// only plain-HTTP traffic targets its own local cache on localhost, which
// Go's net/http never routes through a configured proxy, so there is no
// plain-HTTP destination for this proxy to ever handle. It records every
// host it has been asked to tunnel traffic to.
package proxy

import (
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

	mu    sync.Mutex
	hosts map[string]struct{}
}

// Start starts a forward proxy listening on an OS-assigned local port.
func Start() (*Proxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	p := &Proxy{listener: ln, hosts: make(map[string]struct{})}
	p.server = &http.Server{Handler: http.HandlerFunc(p.handleConnect)}
	go func() { _ = p.server.Serve(ln) }()

	return p, nil
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

	destConn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer destConn.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()

	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(destConn, clientConn) }()
	go func() { defer wg.Done(); _, _ = io.Copy(clientConn, destConn) }()
	wg.Wait()
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
