package operator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// countingBundleHandler records how many times a bundle was actually read, so
// the tests can assert the resolver is lazy and memoises.
type countingBundleHandler struct {
	controllers map[string]struct{}
	err         error
	mu          sync.Mutex
	calls       int
	block       chan struct{}
}

func (h *countingBundleHandler) ControllerImages(_ context.Context, _, _ string, _ mirror.CopyOptions) (map[string]struct{}, error) {
	h.mu.Lock()
	h.calls++
	h.mu.Unlock()
	if h.block != nil {
		<-h.block
	}
	return h.controllers, h.err
}

func (h *countingBundleHandler) callCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

const (
	controllerImg = "registry.redhat.io/ns/virt-operator-rhel9@sha256:aaa"
	operandImg    = "registry.redhat.io/ns/virt-launcher-4-nv-rhel10@sha256:bbb"
	bundleImg     = "registry.redhat.io/ns/hco-bundle@sha256:ccc"
)

func newResolver(t *testing.T, handler bundleHandlerInterface) *OperandResolver {
	t.Helper()
	return &OperandResolver{
		Log:     clog.New("trace"),
		Opts:    &mirror.CopyOptions{Global: &mirror.GlobalOptions{WorkingDir: t.TempDir()}},
		handler: handler,
		bundles: make(map[string]*bundleControllers),
	}
}

func TestOperandResolver(t *testing.T) {
	bundles := map[string]string{bundleImg: "hco.v4.22.12"}

	t.Run("an image no deployment runs is an operand", func(t *testing.T) {
		handler := &countingBundleHandler{controllers: map[string]struct{}{controllerImg: {}}}
		got, err := newResolver(t, handler).IsOperand(context.Background(), "docker://"+operandImg, bundles)

		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("an image the operator runs is not an operand", func(t *testing.T) {
		handler := &countingBundleHandler{controllers: map[string]struct{}{controllerImg: {}}}
		got, err := newResolver(t, handler).IsOperand(context.Background(), "docker://"+controllerImg, bundles)

		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("an image no bundle lists is never an operand", func(t *testing.T) {
		handler := &countingBundleHandler{controllers: map[string]struct{}{controllerImg: {}}}
		got, err := newResolver(t, handler).IsOperand(context.Background(), "docker://"+operandImg, nil)

		require.NoError(t, err)
		assert.False(t, got)
		assert.Zero(t, handler.callCount(), "no bundle to read means no pull")
	})

	t.Run("a controller in any listing bundle wins", func(t *testing.T) {
		// Shared image: an operand of one bundle, but run by another operator.
		other := "registry.redhat.io/ns/other-bundle@sha256:ddd"
		handler := &countingBundleHandler{controllers: map[string]struct{}{operandImg: {}}}
		got, err := newResolver(t, handler).IsOperand(context.Background(), "docker://"+operandImg,
			map[string]string{bundleImg: "hco.v4.22.12", other: "other.v1"})

		require.NoError(t, err)
		assert.False(t, got, "an operator needs it, so it cannot be skipped")
	})

	t.Run("an unreadable bundle is an error, never a skip", func(t *testing.T) {
		handler := &countingBundleHandler{err: errors.New("pull failed")}
		got, err := newResolver(t, handler).IsOperand(context.Background(), "docker://"+operandImg, bundles)

		require.Error(t, err)
		assert.False(t, got)
	})

	t.Run("each bundle is read once, however many images ask", func(t *testing.T) {
		handler := &countingBundleHandler{controllers: map[string]struct{}{controllerImg: {}}}
		resolver := newResolver(t, handler)

		for range 5 {
			_, err := resolver.IsOperand(context.Background(), "docker://"+operandImg, bundles)
			require.NoError(t, err)
		}

		assert.Equal(t, 1, handler.callCount(), "the result must be memoised per bundle")
	})

	t.Run("concurrent callers share a single read", func(t *testing.T) {
		// The handler blocks until released, so every goroutine is inside the
		// resolver at once: without single-flight they would each pull.
		handler := &countingBundleHandler{
			controllers: map[string]struct{}{controllerImg: {}},
			block:       make(chan struct{}),
		}
		resolver := newResolver(t, handler)

		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = resolver.IsOperand(context.Background(), "docker://"+operandImg, bundles)
			}()
		}

		time.Sleep(50 * time.Millisecond)
		close(handler.block)
		wg.Wait()

		assert.Equal(t, 1, handler.callCount(), "concurrent askers must wait on one pull, not stampede")
	})
}

func TestIsOperandReferenceMatching(t *testing.T) {
	const (
		digestA = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
		digestB = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
		repo    = "registry.redhat.io/ns/virt-operator-rhel9"
	)
	bundles := map[string]string{bundleImg: "hco.v4.22.12"}

	tests := []struct {
		name       string
		controller string
		candidate  string
		operand    bool
	}{
		{"same digest", repo + "@" + digestA, repo + "@" + digestA, false},
		{"different digest", repo + "@" + digestA, repo + "@" + digestB, true},
		{"same tag", repo + ":v4.22", repo + ":v4.22", false},
		{"different tag", repo + ":v4.22", repo + ":v4.21", true},
		// Cannot be compared without resolving the tag, so it must not be skipped.
		{"tag against digest", repo + ":v4.22", repo + "@" + digestA, false},
		{"digest against tag", repo + "@" + digestA, repo + ":v4.22", false},
		// Same path, different registry: must not be treated as the same image.
		{"different registry", "quay.io/ns/virt-operator-rhel9@" + digestA, repo + "@" + digestA, true},
		{"unrelated repository", repo + "@" + digestA, "registry.redhat.io/ns/virt-launcher@" + digestB, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &countingBundleHandler{controllers: map[string]struct{}{tt.controller: {}}}
			got, err := newResolver(t, handler).IsOperand(context.Background(), "docker://"+tt.candidate, bundles)

			require.NoError(t, err)
			assert.Equal(t, tt.operand, got)
		})
	}

	t.Run("a reference that cannot be parsed still matches exactly", func(t *testing.T) {
		const malformed = "not a valid@@reference"
		handler := &countingBundleHandler{controllers: map[string]struct{}{malformed: {}}}
		got, err := newResolver(t, handler).IsOperand(context.Background(), malformed, bundles)

		require.NoError(t, err)
		assert.False(t, got, "an exact string match is still a controller")
	})
}

func TestReferenceWithoutTransport(t *testing.T) {
	assert.Equal(t, "registry.io/ns/img@sha256:aaa", referenceWithoutTransport("docker://registry.io/ns/img@sha256:aaa"))
	assert.Equal(t, "registry.io/ns/img@sha256:aaa", referenceWithoutTransport("registry.io/ns/img@sha256:aaa"))
}

func TestFindClusterServiceVersion(t *testing.T) {
	csv := `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: test-operator.v1.0.0
spec:
  install:
    spec:
      deployments:
        - name: controller
          spec:
            template:
              spec:
                containers:
                  - name: manager
                    image: registry.example.com/controller@sha256:aaa
                initContainers:
                  - name: init
                    image: registry.example.com/init@sha256:bbb
  relatedImages:
    - name: operand
      image: registry.example.com/operand@sha256:ccc
`

	t.Run("parses the CSV and finds every container image", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.clusterserviceversion.yaml"), []byte(csv), 0o600))

		parsed, err := findClusterServiceVersion(dir)
		require.NoError(t, err)

		images := map[string]struct{}{}
		for _, dep := range parsed.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
			for _, c := range dep.Spec.Template.Spec.Containers {
				images[c.Image] = struct{}{}
			}
			for _, c := range dep.Spec.Template.Spec.InitContainers {
				images[c.Image] = struct{}{}
			}
		}
		assert.Contains(t, images, "registry.example.com/controller@sha256:aaa")
		assert.Contains(t, images, "registry.example.com/init@sha256:bbb")
		assert.NotContains(t, images, "registry.example.com/operand@sha256:ccc",
			"a related image that no deployment runs is an operand")
	})

	t.Run("reports when there is no CSV", func(t *testing.T) {
		_, err := findClusterServiceVersion(t.TempDir())
		assert.Error(t, err)
	})
}
