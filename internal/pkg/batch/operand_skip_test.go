package batch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// errNoMatchingPlatform is what the copy library returns for a manifest list
// that has no instance for any of the requested platforms.
var errNoMatchingPlatform = errors.New("preparing instances for copy: no instances found for platform linux/amd64")

const (
	operandRef = consts.DockerProtocol + "registry/ns/arm64-only-operand@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"
	bundleRef  = consts.DockerProtocol + "registry/ns/bundle@sha256:a30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebeb"
	controlRef = consts.DockerProtocol + "registry/ns/controller@sha256:b30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebec"
)

// resolverStub answers a fixed verdict and counts how often it was asked, so
// the tests can assert the worker only pays for resolution on a platform miss.
type resolverStub struct {
	operand bool
	err     error
	mu      sync.Mutex
	calls   int
}

func (r *resolverStub) IsOperand(_ context.Context, _ string, _ map[string]string) (bool, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	return r.operand, r.err
}

func (r *resolverStub) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func platformFilterOpts(t *testing.T) mirror.CopyOptions {
	t.Helper()
	global := &mirror.GlobalOptions{SecurePolicy: false, Quiet: false}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	return mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "oci:test",
		Mode:                mirror.MirrorToMirror,
		Function:            "copy",
	}
}

// operandSchema builds a collector schema holding one image that will fail on
// platform, the bundle that lists it, and one other operator image. Everything
// is typed as the collector leaves it: classification now happens at copy time.
func operandSchema() v2alpha1.CollectorSchema {
	images := []v2alpha1.CopyImageSchema{
		{Source: operandRef, Origin: operandRef, Destination: "docker://dest/operand", Type: v2alpha1.TypeOperatorRelatedImage},
		{Source: controlRef, Origin: controlRef, Destination: "docker://dest/controller", Type: v2alpha1.TypeOperatorRelatedImage},
		{Source: bundleRef, Origin: bundleRef, Destination: "docker://dest/bundle", Type: v2alpha1.TypeOperatorBundle},
	}

	return v2alpha1.CollectorSchema{
		AllImages:           images,
		TotalOperatorImages: len(images),
		PlatformFilters: map[string][]v2alpha1.InstancePlatformFilter{
			operandRef: {{OS: "linux", Architecture: "amd64"}},
		},
		CopyImageSchemaMap: v2alpha1.CopyImageSchemaMap{
			OperatorsByImage: map[string]map[string]struct{}{
				operandRef: {"some-operator": struct{}{}},
			},
			// Links the image to its bundle: this is what the worker hands the
			// resolver, and without the skip it is what drops the bundle.
			BundlesByImage: map[string]map[string]string{
				operandRef: {bundleRef: "some-operator.v1.0.0"},
			},
		},
	}
}

func TestWorkerSkipsOperandWithNoMatchingPlatform(t *testing.T) {
	log := clog.New("trace")
	timestampStr := time.Now().Format("20060102_150405")
	opts := platformFilterOpts(t)

	// Fails the named image on platform, succeeds for everything else.
	newMirror := func(failing string) *MirrorMock {
		m := new(MirrorMock)
		m.On("Run", mock.Anything, failing, mock.Anything, mock.Anything, mock.Anything).Return(errNoMatchingPlatform)
		m.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		return m
	}

	newWorker := func(t *testing.T, m *MirrorMock, r *resolverStub) (BatchInterface, string) {
		t.Helper()
		logsDir := t.TempDir()
		// A nil *resolverStub must be passed as a nil interface, not as a
		// non-nil interface holding a nil pointer.
		var resolver OperandResolver
		if r != nil {
			resolver = r
		}
		return New(ChannelConcurrentWorker, log, logsDir, m, uint(8), timestampStr, resolver), logsDir
	}

	t.Run("a proven operand is skipped and its bundle still mirrored", func(t *testing.T) {
		resolver := &resolverStub{operand: true}
		w, _ := newWorker(t, newMirror(operandRef), resolver)

		copied, err := w.Worker(context.Background(), operandSchema(), opts)

		require.NoError(t, err, "a skipped operand must not fail the batch")

		origins := make([]string, 0, len(copied.AllImages))
		for _, img := range copied.AllImages {
			origins = append(origins, img.Origin)
		}
		assert.NotContains(t, origins, operandRef, "the operand was not mirrored")
		assert.Contains(t, origins, bundleRef, "the bundle must still be mirrored")
		assert.Contains(t, origins, controlRef)

		// 3 operator images in, 1 skipped, so 2 counted as copied.
		assert.Equal(t, 2, copied.TotalOperatorImages)

		if assert.Len(t, copied.SkippedImages, 1) {
			assert.Equal(t, operandRef, copied.SkippedImages[0].Origin)
			assert.Equal(t, v2alpha1.TypeOperatorOperand, copied.SkippedImages[0].Type,
				"the record must say what the image turned out to be")
			assert.Equal(t, noMatchingPlatformReason, copied.SkippedImages[0].Reason)
			assert.Equal(t, []string{"linux/amd64"}, copied.SkippedImages[0].Platforms)
		}
	})

	t.Run("the resolver is only consulted on a platform miss", func(t *testing.T) {
		resolver := &resolverStub{operand: true}
		m := new(MirrorMock)
		m.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		w, _ := newWorker(t, m, resolver)

		_, err := w.Worker(context.Background(), operandSchema(), opts)

		require.NoError(t, err)
		assert.Zero(t, resolver.callCount(),
			"a clean mirror must never pay for a bundle pull")
	})

	t.Run("an image the operator runs fails, naming the operator", func(t *testing.T) {
		resolver := &resolverStub{operand: false}
		w, logsDir := newWorker(t, newMirror(operandRef), resolver)

		copied, err := w.Worker(context.Background(), operandSchema(), opts)

		require.Error(t, err, "a controller image with no matching platform is a real failure")
		assert.Empty(t, copied.SkippedImages)

		reported := readMirroringErrors(t, logsDir)
		assert.Contains(t, reported, `operator "some-operator" does not support linux/amd64`)
		assert.Contains(t, reported, "no instances found for platform",
			"the original cause must stay in the chain")
	})

	t.Run("a resolver failure never becomes a skip", func(t *testing.T) {
		resolver := &resolverStub{err: errors.New("bundle pull failed")}
		w, logsDir := newWorker(t, newMirror(operandRef), resolver)

		copied, err := w.Worker(context.Background(), operandSchema(), opts)

		require.Error(t, err, "we could not prove it is an operand, so it must fail")
		assert.Empty(t, copied.SkippedImages)

		// We only know the image has no matching instance, so the report must
		// not claim the operator lacks support for the platform.
		reported := readMirroringErrors(t, logsDir)
		assert.Contains(t, reported, "no instances found for platform")
		assert.NotContains(t, reported, "does not support",
			"an unreadable bundle proves nothing about the operator")
	})

	t.Run("without a resolver nothing is skipped", func(t *testing.T) {
		w, _ := newWorker(t, newMirror(operandRef), nil)

		copied, err := w.Worker(context.Background(), operandSchema(), opts)

		require.Error(t, err)
		assert.Empty(t, copied.SkippedImages)
	})

	t.Run("the same error on an additional image still fails the batch", func(t *testing.T) {
		schema := v2alpha1.CollectorSchema{
			AllImages: []v2alpha1.CopyImageSchema{
				{Source: operandRef, Origin: operandRef, Destination: "docker://dest/additional", Type: v2alpha1.TypeGeneric},
			},
			TotalAdditionalImages: 1,
			PlatformFilters: map[string][]v2alpha1.InstancePlatformFilter{
				operandRef: {{OS: "linux", Architecture: "amd64"}},
			},
		}

		resolver := &resolverStub{operand: true}
		w, _ := newWorker(t, newMirror(operandRef), resolver)

		copied, err := w.Worker(context.Background(), schema, opts)

		require.Error(t, err, "the user named this image explicitly, so a platform miss is a config error")
		assert.Empty(t, copied.SkippedImages)
		assert.Zero(t, resolver.callCount(), "an additional image is never an operand")
	})

	t.Run("an operand failing for any other reason still fails the batch", func(t *testing.T) {
		resolver := &resolverStub{operand: true}
		m := new(MirrorMock)
		m.On("Run", mock.Anything, operandRef, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("manifest unknown"))
		m.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		w, _ := newWorker(t, m, resolver)

		copied, err := w.Worker(context.Background(), operandSchema(), opts)

		require.Error(t, err, "only a platform mismatch is skippable")
		assert.Empty(t, copied.SkippedImages)
	})

	t.Run("no platform filter means the error is never downgraded", func(t *testing.T) {
		schema := operandSchema()
		schema.PlatformFilters = nil

		resolver := &resolverStub{operand: true}
		w, _ := newWorker(t, newMirror(operandRef), resolver)

		_, err := w.Worker(context.Background(), schema, opts)

		require.Error(t, err)
	})

	t.Run("disk to mirror does not reach for the registry", func(t *testing.T) {
		d2mOpts := platformFilterOpts(t)
		d2mOpts.Mode = mirror.DiskToMirror

		resolver := &resolverStub{operand: true}
		w, _ := newWorker(t, newMirror(operandRef), resolver)

		_, err := w.Worker(context.Background(), operandSchema(), d2mOpts)

		require.Error(t, err)
		assert.Zero(t, resolver.callCount(),
			"the bundle lives in a registry a disconnected run cannot reach")
	})
}

// readMirroringErrors returns the contents of the error report the worker
// writes when a batch fails.
func readMirroringErrors(t *testing.T, logsDir string) string {
	t.Helper()
	entries, err := os.ReadDir(logsDir)
	require.NoError(t, err)

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "mirroring_errors_") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(logsDir, entry.Name()))
		require.NoError(t, err)
		return string(data)
	}

	t.Fatalf("no mirroring errors file written to %s", logsDir)
	return ""
}
