package release

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type mockImageBuilder struct {
	Fail  bool
	Calls *int
}

// gatedDeadlineContext releases its deadline signal only after the test has
// observed the HTTP request in the server handler.
type gatedDeadlineContext struct {
	done chan struct{}
	once sync.Once
}

func (c *gatedDeadlineContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (c *gatedDeadlineContext) Done() <-chan struct{} {
	return c.done
}

func (c *gatedDeadlineContext) Err() error {
	select {
	case <-c.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func (c *gatedDeadlineContext) Value(key interface{}) interface{} {
	return nil
}

func (c *gatedDeadlineContext) expire() {
	c.once.Do(func() { close(c.done) })
}

func TestCreateGraphImage(t *testing.T) {

	log := clog.New("trace")
	globalM2D := &mirror.GlobalOptions{
		SecurePolicy: false,
		WorkingDir:   t.TempDir(),
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, retryOpts := mirror.RetryFlags()
	_, srcOptsM2D := mirror.ImageSrcFlags(globalM2D, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOptsM2D := mirror.ImageDestFlags(globalM2D, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")

	m2dOpts := mirror.CopyOptions{
		Global:              globalM2D,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOptsM2D,
		DestImage:           destOptsM2D,
		RetryOpts:           retryOpts,
		Destination:         consts.FileProtocol + "test",
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
		LocalStorageFQDN:    "localhost:9999",
	}

	cfgm2d := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Platform: v2alpha1.Platform{
					Graph: true,
					Channels: []v2alpha1.ReleaseChannel{
						{
							Name: "stable-4.7",
						},
						{
							Name:       "stable-4.6",
							MinVersion: "4.6.3",
							MaxVersion: "4.6.13",
						},
						{
							Name: "okd",
							Type: v2alpha1.TypeOKD,
						},
					},
				},
				Operators: []v2alpha1.Operator{
					{
						Catalog: "redhat-operators:v4.7",
						Full:    true,
					},
					{
						Catalog: "certified-operators:v4.7",
						Full:    true,
						IncludeConfig: v2alpha1.IncludeConfig{
							Packages: []v2alpha1.IncludePackage{
								{Name: "couchbase-operator"},
								{
									Name: "mongodb-operator",
									IncludeBundle: v2alpha1.IncludeBundle{
										MinVersion: "1.4.0",
									},
								},
								{
									Name: "crunchy-postgresql-operator",
									Channels: []v2alpha1.IncludeChannel{
										{Name: "stable"},
									},
								},
							},
						},
					},
					{
						Catalog: "community-operators:v4.7",
					},
				},
				AdditionalImages: []v2alpha1.AdditionalImage{
					{Name: "registry.redhat.io/ubi8/ubi:latest"},
				},
				Helm: v2alpha1.Helm{
					Repositories: []v2alpha1.Repository{
						{
							URL:  "https://stefanprodan.github.io/podinfo",
							Name: "podinfo",
							Charts: []v2alpha1.Chart{
								{Name: "podinfo", Version: "5.0.0"},
							},
						},
					},
					Local: []v2alpha1.Chart{
						{Name: "podinfo", Path: "/test/podinfo-5.0.0.tar.gz"},
					},
				},
				BlockedImages: []v2alpha1.BlockedImage{
					{Name: "alpine"},
					{Name: "redis"},
				},
				Samples: []v2alpha1.SampleImage{
					{Name: "ruby"},
					{Name: "python"},
					{Name: "nginx"},
				},
			},
		},
	}

	cincinnati := &MockCincinnati{Config: cfgm2d, Opts: m2dOpts}

	ctx := context.Background()

	// this test should cover over 80% M2D
	t.Run("Testing CreateGraphImage - Mirror to disk: should pass", func(t *testing.T) {
		manifest := &MockManifest{Log: log}
		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           &MockMirror{Fail: false},
			Config:           cfgm2d,
			Manifest:         manifest,
			Opts:             m2dOpts,
			Cincinnati:       cincinnati,
			LocalStorageFQDN: "localhost:9999",
			ImageBuilder:     &mockImageBuilder{},
		}

		// just to ensure we cover new.go
		_ = New(log, "nada", cfgm2d, m2dOpts, &MockMirror{}, &MockManifest{}, cincinnati, &mockImageBuilder{})

		_, err := ex.CreateGraphImage(ctx, graphURL)
		if err != nil {
			t.Fatalf("should not fail")
		}

	})

	t.Run("Testing CreateGraphImage - Mirror to disk: should fail", func(t *testing.T) {
		manifest := &MockManifest{Log: log}
		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           &MockMirror{Fail: false},
			Config:           cfgm2d,
			Manifest:         manifest,
			Opts:             m2dOpts,
			Cincinnati:       cincinnati,
			LocalStorageFQDN: "localhost:9999",
			ImageBuilder:     &mockImageBuilder{},
		}

		_, err := ex.CreateGraphImage(ctx, "nada")
		if err == nil {
			t.Fatalf("should fail")
		}

	})

	t.Run("Testing CreateGraphImage - Mirror to disk: should fail", func(t *testing.T) {
		manifest := &MockManifest{Log: log}
		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           &MockMirror{Fail: false},
			Config:           cfgm2d,
			Manifest:         manifest,
			Opts:             m2dOpts,
			Cincinnati:       cincinnati,
			LocalStorageFQDN: "localhost:9999",
			ImageBuilder:     &mockImageBuilder{Fail: true},
		}

		_, err := ex.CreateGraphImage(ctx, graphURL)
		if err == nil {
			t.Fatalf("should fail")
		}

	})

}

func (o mockImageBuilder) BuildAndPush(ctx context.Context, targetRef string, layoutPath layout.Path, cmd []string, layers ...v1.Layer) (string, error) {
	if o.Calls != nil {
		(*o.Calls)++
	}
	if o.Fail {
		return "", fmt.Errorf("forced error")
	}
	return "sha256:12345", nil
}

func (o mockImageBuilder) SaveImageLayoutToDir(ctx context.Context, imgRef string, layoutDir string) (layout.Path, error) {
	if o.Calls != nil {
		(*o.Calls)++
	}
	if o.Fail {
		return layout.Path(""), fmt.Errorf("forced error")
	}
	p, err := layout.FromPath(consts.TestFolder + "test-untar")
	if err != nil {
		return layout.Path(""), fmt.Errorf("getting layout from path: %w", err)
	}
	return p, nil
}

func (o mockImageBuilder) ProcessImageIndex(ctx context.Context, idx v1.ImageIndex, v2format *bool, cmd []string, targetRef string, layers ...v1.Layer) (v1.ImageIndex, error) {
	return nil, nil
}

func (o mockImageBuilder) RebuildCatalogs(ctx context.Context, collectorSchema v2alpha1.CollectorSchema) ([]v2alpha1.CopyImageSchema, error) {
	return []v2alpha1.CopyImageSchema{}, nil
}

func TestCreateGraphImageHTTPStatus(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusPartialContent, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			builderCalls := 0
			ex := newTestGraphCollector(t, mirror.MirrorToDisk)
			ex.ImageBuilder = &mockImageBuilder{Calls: &builderCalls}
			_, err := ex.CreateGraphImage(context.Background(), server.URL)
			if err == nil {
				t.Fatal("expected non-success HTTP status to fail")
			}
			if !strings.Contains(err.Error(), "unexpected HTTP status") {
				t.Errorf("expected status-aware error, got %v", err)
			}
			if !strings.Contains(err.Error(), http.StatusText(status)) {
				t.Errorf("expected response status in error, got %v", err)
			}
			if strings.Contains(err.Error(), "gzip") {
				t.Errorf("expected status error before gzip decoding, got %v", err)
			}
			if builderCalls != 0 {
				t.Errorf("expected image builder not to be called, got %d calls", builderCalls)
			}
		})
	}
}

func TestCreateGraphImageSuccess(t *testing.T) {
	graphData, err := os.ReadFile(filepath.Join(consts.TestFolder, "graph-assets", "graph-data.tar.gz"))
	if err != nil {
		t.Fatalf("read graph fixture: %v", err)
	}

	for _, mode := range []string{mirror.MirrorToDisk, mirror.MirrorToMirror} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if _, err := w.Write(graphData); err != nil {
					t.Errorf("write graph fixture: %v", err)
				}
			}))
			defer server.Close()

			ex := newTestGraphCollector(t, mode)
			imageRef, err := ex.CreateGraphImage(context.Background(), server.URL)
			if err != nil {
				t.Fatalf("create graph image: %v", err)
			}
			if !strings.HasPrefix(imageRef, consts.DockerProtocol) {
				t.Errorf("expected Docker image reference, got %q", imageRef)
			}
		})
	}
}

func TestCreateGraphImageHonorsContextCancellation(t *testing.T) { //nolint:cyclop // test covers request cancellation and bounded cleanup paths
	requestStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseHandler) })
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		select {
		case <-r.Context().Done():
		case <-releaseHandler:
		}
	}))
	defer func() {
		release()
		server.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ex := newTestGraphCollector(t, mirror.MirrorToDisk)
	builderCalls := 0
	ex.ImageBuilder = &mockImageBuilder{Calls: &builderCalls}
	done := make(chan error, 1)
	go func() {
		_, err := ex.CreateGraphImage(ctx, server.URL)
		done <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(5 * time.Second):
		release()
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		t.Fatal("graph request did not reach the test server")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context cancellation, got %v", err)
		}
		if builderCalls != 0 {
			t.Errorf("expected image builder not to be called, got %d calls", builderCalls)
		}
	case <-time.After(time.Second):
		release()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		t.Fatal("graph request did not stop after context cancellation")
	}
}

func TestCreateGraphImageHonorsContextDeadline(t *testing.T) { //nolint:cyclop // test covers gated deadline and bounded cleanup paths
	requestStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseHandler) })
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		select {
		case <-r.Context().Done():
		case <-releaseHandler:
		}
	}))
	defer func() {
		release()
		server.Close()
	}()

	ctx := &gatedDeadlineContext{done: make(chan struct{})}
	defer ctx.expire()
	ex := newTestGraphCollector(t, mirror.MirrorToMirror)
	builderCalls := 0
	ex.ImageBuilder = &mockImageBuilder{Calls: &builderCalls}
	done := make(chan error, 1)
	go func() {
		_, err := ex.CreateGraphImage(ctx, server.URL)
		done <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(5 * time.Second):
		release()
		ctx.expire()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		t.Fatal("graph request did not reach the test server")
	}
	ctx.expire()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context deadline, got %v", err)
		}
		if builderCalls != 0 {
			t.Errorf("expected image builder not to be called, got %d calls", builderCalls)
		}
	case <-time.After(5 * time.Second):
		release()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		t.Fatal("graph request did not stop after context deadline")
	}
}

func TestCreateGraphImageRejectsInvalidURL(t *testing.T) {
	ex := newTestGraphCollector(t, mirror.MirrorToMirror)
	builderCalls := 0
	ex.ImageBuilder = &mockImageBuilder{Calls: &builderCalls}
	if _, err := ex.CreateGraphImage(context.Background(), "://"); err == nil {
		t.Fatal("expected invalid graph URL to fail")
	} else if !strings.Contains(err.Error(), "missing protocol scheme") {
		t.Errorf("expected URL parsing error, got %v", err)
	}
	if builderCalls != 0 {
		t.Errorf("expected image builder not to be called, got %d calls", builderCalls)
	}
}

func TestReleaseImageCollectorSkipsGraphWhenDisabled(t *testing.T) {
	ex := newTestGraphCollector(t, mirror.MirrorToDisk)
	ex.Config.Mirror.Platform.Graph = false
	ex.Cincinnati = emptyCincinnati{}

	result, err := ex.ReleaseImageCollector(context.Background())
	if err != nil {
		t.Fatalf("release collection should not fail: %v", err)
	}
	if len(result.AllImages) != 0 {
		t.Errorf("expected no graph or release images, got %d images", len(result.AllImages))
	}
}

func newTestGraphCollector(t *testing.T, mode string) *LocalStorageCollector {
	t.Helper()
	global := &mirror.GlobalOptions{
		SecurePolicy: false,
		WorkingDir:   t.TempDir(),
	}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, retryOpts := mirror.RetryFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")

	destination := consts.FileProtocol + "test"
	if mode == mirror.MirrorToMirror {
		destination = consts.DockerProtocol + "mymirror"
	}
	copyOpts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         destination,
		Dev:                 false,
		Mode:                mode,
		LocalStorageFQDN:    "localhost:9999",
	}
	return &LocalStorageCollector{
		Log:              clog.New("trace"),
		Opts:             copyOpts,
		LocalStorageFQDN: "localhost:9999",
		ImageBuilder:     &mockImageBuilder{},
	}
}

type emptyCincinnati struct{}

func (emptyCincinnati) GetReleaseReferenceImages(context.Context) ([]v2alpha1.CopyImageSchema, error) {
	return nil, nil
}
