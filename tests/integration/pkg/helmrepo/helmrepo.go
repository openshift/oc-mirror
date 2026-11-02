// Package helmrepo serves local Helm charts as a Helm chart repository over HTTP.
package helmrepo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/repo"
)

// Server is a Helm chart repository serving packaged charts and their index.
type Server struct {
	server *http.Server
	dir    string
	addr   string
}

// Start packages every chart directory under chartsDir, like `helm package`, indexes
// them, like `helm repo index`, and serves the repository on localhost:<port>.
func Start(chartsDir string, port int) (*Server, error) {
	dir, err := os.MkdirTemp("", "oc-mirror-helm-repo-*")
	if err != nil {
		return nil, err
	}

	chartDirs, err := os.ReadDir(chartsDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range chartDirs {
		if !entry.IsDir() {
			continue
		}
		chart, err := loader.LoadDir(filepath.Join(chartsDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("loading chart %s: %w", entry.Name(), err)
		}
		if _, err := chartutil.Save(chart, dir); err != nil {
			return nil, fmt.Errorf("packaging chart %s: %w", entry.Name(), err)
		}
	}

	addr := fmt.Sprintf("localhost:%d", port)
	// Chart URLs are relative, so the repository works under any host name.
	index, err := repo.IndexDirectory(dir, "")
	if err != nil {
		return nil, fmt.Errorf("indexing charts: %w", err)
	}
	if err := index.WriteFile(filepath.Join(dir, "index.yaml"), 0o644); err != nil {
		return nil, fmt.Errorf("writing index: %w", err)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &Server{
		server: &http.Server{Handler: http.FileServer(http.Dir(dir)), ReadHeaderTimeout: 10 * time.Second},
		dir:    dir,
		addr:   addr,
	}
	go func() {
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "helm repository server: %v\n", err)
		}
	}()
	return s, nil
}

// URL returns the repository URL.
func (s *Server) URL() string {
	return "http://" + s.addr
}

// Stop stops the server and removes the packaged charts.
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdownErr := s.server.Shutdown(ctx)
	return errors.Join(shutdownErr, os.RemoveAll(s.dir))
}
