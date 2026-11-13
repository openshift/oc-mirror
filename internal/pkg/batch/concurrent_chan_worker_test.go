package batch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
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

// TestOCPBUGS53455_RebuiltCatalogPreserveDigests tests that rebuilt operator catalogs
// have PreserveDigests set to false to allow manifest format conversions.
// This prevents the error: "Manifest list must be converted to type ... but we cannot modify it: Instructed to preserve digests"
func TestOCPBUGS53455_RebuiltCatalogPreserveDigests(t *testing.T) {
	log := clog.New("trace")

	global := &mirror.GlobalOptions{SecurePolicy: false, Quiet: false}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	tempDir := t.TempDir()
	timestampStr := time.Now().Format("20060102_150405")

	tests := []struct {
		name                     string
		catalogImage             v2alpha1.CopyImageSchema
		expectedPreserveDigests  bool
		expectedRemoveSignatures bool
		description              string
	}{
		{
			name: "Rebuilt catalog should have PreserveDigests=false",
			catalogImage: v2alpha1.CopyImageSchema{
				Source:      consts.DockerProtocol + "localhost:55000/redhat/redhat-operator-index@sha256:rebuilthash",
				Origin:      consts.DockerProtocol + "registry.redhat.io/redhat/redhat-operator-index:v4.17",
				Destination: consts.DockerProtocol + "nexus:8082/redhat/redhat-operator-index:v4.17",
				Type:        v2alpha1.TypeOperatorCatalog,
				RebuiltTag:  "sha256-rebuilthash.tag", // This indicates a rebuilt catalog
			},
			expectedPreserveDigests:  false, // CRITICAL: Must be false to allow format conversion
			expectedRemoveSignatures: true,
			description:              "Rebuilt catalogs need format conversion support for registries like Nexus",
		},
		{
			name: "Non-rebuilt catalog should have PreserveDigests=true (default)",
			catalogImage: v2alpha1.CopyImageSchema{
				Source:      consts.DockerProtocol + "registry.redhat.io/redhat/redhat-operator-index:v4.17",
				Origin:      consts.DockerProtocol + "registry.redhat.io/redhat/redhat-operator-index:v4.17",
				Destination: consts.DockerProtocol + "nexus:8082/redhat/redhat-operator-index:v4.17",
				Type:        v2alpha1.TypeOperatorCatalog,
				RebuiltTag:  "", // Empty RebuiltTag means it's not rebuilt
			},
			expectedPreserveDigests:  true, // Default behavior
			expectedRemoveSignatures: false,
			description:              "Non-rebuilt catalogs should preserve digests",
		},
		{
			name: "Operator bundle should always have PreserveDigests=true",
			catalogImage: v2alpha1.CopyImageSchema{
				Source:      consts.DockerProtocol + "registry.redhat.io/rhbk/keycloak-operator-bundle@sha256:somehash",
				Origin:      consts.DockerProtocol + "registry.redhat.io/rhbk/keycloak-operator-bundle@sha256:somehash",
				Destination: consts.DockerProtocol + "nexus:8082/rhbk/keycloak-operator-bundle@sha256:somehash",
				Type:        v2alpha1.TypeOperatorBundle,
				RebuiltTag:  "",
			},
			expectedPreserveDigests:  true,
			expectedRemoveSignatures: false,
			description:              "Operator bundles must preserve digests for signature verification",
		},
		{
			name: "Operator related image should always have PreserveDigests=true",
			catalogImage: v2alpha1.CopyImageSchema{
				Source:      consts.DockerProtocol + "registry.redhat.io/rhbk/keycloak@sha256:imagehash",
				Origin:      consts.DockerProtocol + "registry.redhat.io/rhbk/keycloak@sha256:imagehash",
				Destination: consts.DockerProtocol + "nexus:8082/rhbk/keycloak@sha256:imagehash",
				Type:        v2alpha1.TypeOperatorRelatedImage,
				RebuiltTag:  "",
			},
			expectedPreserveDigests:  true,
			expectedRemoveSignatures: false,
			description:              "Related images must preserve digests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock that captures the CopyOptions passed to Mirror.Run
			mirrorMock := new(MirrorMock)
			var capturedOpts *mirror.CopyOptions

			// Setup mock to capture the options passed to Run
			mirrorMock.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(opts *mirror.CopyOptions) bool {
				capturedOpts = opts
				return true
			})).Return(nil)

			opts := mirror.CopyOptions{
				Global:              global,
				DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
				SrcImage:            srcOpts,
				DestImage:           destOpts,
				RetryOpts:           retryOpts,
				Destination:         consts.DockerProtocol + "nexus:8082",
				Dev:                 false,
				Mode:                mirror.MirrorToMirror,
				Function:            "copy",
			}

			collectorSchema := v2alpha1.CollectorSchema{
				AllImages: []v2alpha1.CopyImageSchema{tt.catalogImage},
			}

			w := &ChannelConcurrentBatch{
				Log:              log,
				LogsDir:          tempDir,
				Mirror:           mirrorMock,
				MaxGoroutines:    1,
				SynchedTimeStamp: timestampStr,
			}

			// Execute the worker
			_, err := w.Worker(context.Background(), collectorSchema, opts)
			assert.NoError(t, err, tt.description)

			// Verify Mock was called
			mirrorMock.AssertExpectations(t)

			// CRITICAL ASSERTIONS: Verify PreserveDigests and RemoveSignatures flags
			assert.NotNil(t, capturedOpts, "CopyOptions should have been captured")
			assert.Equal(t, tt.expectedPreserveDigests, capturedOpts.PreserveDigests,
				"PreserveDigests flag mismatch for %s: %s", tt.catalogImage.Type, tt.description)
			assert.Equal(t, tt.expectedRemoveSignatures, capturedOpts.RemoveSignatures,
				"RemoveSignatures flag mismatch for %s: %s", tt.catalogImage.Type, tt.description)
		})
	}
}

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

		require.Len(t, copied.OperandPlatforms, 1)
		assert.Contains(t, copied.OperandPlatforms, operandRef)
		assert.Empty(t, copied.OperandPlatforms[operandRef], "no platforms means wholly skipped")
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
		assert.Empty(t, copied.OperandPlatforms)

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
		assert.Empty(t, copied.OperandPlatforms)

		// We only know the image has no matching instance, so the report must
		// not claim the operator lacks support for the platform.
		reported := readMirroringErrors(t, logsDir)
		assert.Contains(t, reported, "no instances found for platform")
		assert.NotContains(t, reported, "does not support",
			"an unreadable bundle proves nothing about the operator")
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
		assert.Empty(t, copied.OperandPlatforms)
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
		assert.Empty(t, copied.OperandPlatforms)
	})

	t.Run("no platform filter means the error is never downgraded", func(t *testing.T) {
		schema := operandSchema()
		schema.PlatformFilters = nil

		resolver := &resolverStub{operand: true}
		w, _ := newWorker(t, newMirror(operandRef), resolver)

		_, err := w.Worker(context.Background(), schema, opts)

		require.Error(t, err)
	})

	// Unclassified images must
	// fail without blaming the operator.
	for _, tt := range []struct {
		name     string
		withStub bool
		d2m      bool
		noBundle bool
	}{
		{name: "no resolver configured"},
		{name: "disk to mirror cannot reach the registry", withStub: true, d2m: true},
		{name: "no bundle lists the image", withStub: true, noBundle: true},
	} {
		t.Run("unclassified: "+tt.name, func(t *testing.T) {
			schema := operandSchema()
			if tt.noBundle {
				schema.CopyImageSchemaMap.BundlesByImage = nil
			}
			runOpts := opts
			if tt.d2m {
				runOpts = platformFilterOpts(t)
				runOpts.Mode = mirror.DiskToMirror
			}

			var resolver *resolverStub
			if tt.withStub {
				resolver = &resolverStub{operand: true}
			}
			w, logsDir := newWorker(t, newMirror(operandRef), resolver)

			copied, err := w.Worker(context.Background(), schema, runOpts)

			require.Error(t, err)
			assert.Empty(t, copied.OperandPlatforms, "nothing proved it is an operand")
			if resolver != nil {
				assert.Zero(t, resolver.callCount())
			}

			reported := readMirroringErrors(t, logsDir)
			assert.Contains(t, reported, "no instances found for platform")
			assert.NotContains(t, reported, "does not support",
				"nothing showed the operator runs this image")
		})
	}
}

func TestWorkerMirrorsAvailableOperandPlatforms(t *testing.T) {
	for _, mode := range []string{mirror.MirrorToMirror, mirror.MirrorToDisk} {
		t.Run(mode, func(t *testing.T) {
			schema := operandSchema()
			requested := []v2alpha1.InstancePlatformFilter{
				{OS: "linux", Architecture: "amd64"},
				{OS: "linux", Architecture: "arm64"},
			}
			schema.PlatformFilters[operandRef] = requested
			opts := platformFilterOpts(t)
			opts.Mode = mode
			m := new(MirrorMock)
			m.On("Run", mock.Anything, operandRef, mock.Anything, mirror.CopyMode,
				mock.MatchedBy(func(o *mirror.CopyOptions) bool {
					return slices.Equal(o.InstancePlatforms, []string{"linux/amd64", "linux/arm64"})
				})).Return(platformMiss("linux/arm64")).Once()
			m.On("Run", mock.Anything, operandRef, mock.Anything, mirror.CopyMode,
				mock.MatchedBy(func(o *mirror.CopyOptions) bool {
					return slices.Equal(o.InstancePlatforms, []string{"linux/amd64"}) && o.PreserveDigests
				})).Return(nil).Once()
			for _, ref := range []string{bundleRef, controlRef} {
				m.On("Run", mock.Anything, ref, mock.Anything, mirror.CopyMode, mock.Anything).Return(nil).Once()
			}
			resolver := &resolverStub{operand: true}
			w := New(ChannelConcurrentWorker, clog.New("trace"), t.TempDir(), m, 8, "test", resolver)
			copied, err := w.Worker(context.Background(), schema, opts)
			require.NoError(t, err)
			assert.Len(t, copied.AllImages, 3, "partial operands and their bundles must remain mirrored")
			assert.Equal(t, 3, copied.TotalOperatorImages)
			require.Len(t, copied.OperandPlatforms, 1)
			assert.Equal(t, requested[:1], copied.OperandPlatforms[operandRef])
			assert.Equal(t, requested[:1], copied.PlatformFilters[operandRef], "archive only the selected platforms")
			assert.Equal(t, requested, schema.PlatformFilters[operandRef], "do not modify shared collector filters")
			assert.Equal(t, 1, resolver.callCount())
			m.AssertExpectations(t)
		})
	}
}

func TestOCPBUGS128840_CanceledBeforeSchedulingReturnsCancellation(t *testing.T) {
	for _, mode := range []string{mirror.MirrorToDisk, mirror.MirrorToMirror, mirror.DiskToMirror} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			collectorSchema := v2alpha1.CollectorSchema{
				AllImages: []v2alpha1.CopyImageSchema{
					{
						Source:      "docker://registry.example/repro:latest",
						Origin:      "docker://registry.example/repro:latest",
						Destination: "docker://mirror.example/repro:latest",
						Type:        v2alpha1.TypeGeneric,
					},
				},
			}
			worker := &ChannelConcurrentBatch{
				Log:              clog.New("trace"),
				LogsDir:          t.TempDir(),
				Mirror:           new(MirrorMock),
				MaxGoroutines:    1,
				SynchedTimeStamp: time.Now().Format("20060102_150405"),
			}
			opts := mirror.CopyOptions{
				Global:   &mirror.GlobalOptions{SecurePolicy: false},
				Mode:     mode,
				Function: "copy",
			}

			copiedImages, err := worker.Worker(ctx, collectorSchema, opts)

			assert.ErrorIs(t, err, context.Canceled)
			assert.Empty(t, copiedImages.AllImages)
		})
	}
}

func TestWorkerNeverSkipsBundlePlatforms(t *testing.T) {
	schema := operandSchema()
	schema.PlatformFilters[bundleRef] = schema.PlatformFilters[operandRef]
	schema.CopyImageSchemaMap.BundlesByImage[bundleRef] = map[string]string{bundleRef: "some-operator.v1.0.0"}
	m := new(MirrorMock)
	for _, ref := range []string{operandRef, controlRef} {
		m.On("Run", mock.Anything, ref, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	}
	m.On("Run", mock.Anything, bundleRef, mock.Anything, mock.Anything, mock.Anything).Return(errNoMatchingPlatform).Once()
	resolver := &resolverStub{operand: true}
	w := New(ChannelConcurrentWorker, clog.New("trace"), t.TempDir(), m, 8, "test", resolver)
	copied, err := w.Worker(context.Background(), schema, platformFilterOpts(t))
	require.Error(t, err)
	assert.Empty(t, copied.OperandPlatforms)
	assert.Zero(t, resolver.callCount(), "a bundle must never be classified as an operand")
	m.AssertExpectations(t)
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

func TestOCPBUGS128840_PartialCancellationDoesNotFabricateResult(t *testing.T) {
	for _, mode := range []string{mirror.MirrorToDisk, mirror.MirrorToMirror, mirror.DiskToMirror} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			started := make(chan struct{})
			release := make(chan struct{})
			mirrorMock := new(MirrorMock)
			mirrorMock.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Run(func(mock.Arguments) {
					close(started)
					<-release
				}).Return(nil).Once()

			worker := &ChannelConcurrentBatch{
				Log:              clog.New("trace"),
				LogsDir:          t.TempDir(),
				Mirror:           mirrorMock,
				MaxGoroutines:    1,
				SynchedTimeStamp: time.Now().Format("20060102_150405"),
			}
			opts := mirror.CopyOptions{
				Global:   &mirror.GlobalOptions{SecurePolicy: false},
				Mode:     mode,
				Function: "copy",
			}
			collectorSchema := v2alpha1.CollectorSchema{AllImages: []v2alpha1.CopyImageSchema{
				{Source: "docker://registry.example/first:latest", Origin: "docker://registry.example/first:latest", Destination: "docker://mirror.example/first:latest", Type: v2alpha1.TypeGeneric},
				{Source: "docker://registry.example/second:latest", Origin: "docker://registry.example/second:latest", Destination: "docker://mirror.example/second:latest", Type: v2alpha1.TypeGeneric},
			}}

			type workerResult struct {
				copiedImages v2alpha1.CollectorSchema
				err          error
			}
			done := make(chan workerResult, 1)
			go func() {
				copiedImages, err := worker.Worker(ctx, collectorSchema, opts)
				done <- workerResult{copiedImages: copiedImages, err: err}
			}()

			<-started
			cancel()
			close(release)

			select {
			case result := <-done:
				assert.ErrorIs(t, result.err, context.Canceled)
				assert.Len(t, result.copiedImages.AllImages, 1)
			case <-time.After(time.Second):
				t.Fatal("worker did not return after cancellation")
			}
		})
	}
}

func TestOCPBUGS128840_ReleaseErrorRemainsBatchError(t *testing.T) {
	mirrorMock := new(MirrorMock)
	mirrorMock.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(assert.AnError).Once()
	mirrorMock.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	worker := &ChannelConcurrentBatch{
		Log:              clog.New("trace"),
		LogsDir:          t.TempDir(),
		Mirror:           mirrorMock,
		MaxGoroutines:    1,
		SynchedTimeStamp: time.Now().Format("20060102_150405"),
	}
	opts := mirror.CopyOptions{
		Global:   &mirror.GlobalOptions{SecurePolicy: false},
		Mode:     mirror.MirrorToDisk,
		Function: "copy",
	}
	collectorSchema := v2alpha1.CollectorSchema{AllImages: []v2alpha1.CopyImageSchema{
		{Source: "docker://registry.example/release:latest", Origin: "docker://registry.example/release:latest", Destination: "docker://mirror.example/release:latest", Type: v2alpha1.TypeOCPRelease},
		{Source: "docker://registry.example/content:latest", Origin: "docker://registry.example/content:latest", Destination: "docker://mirror.example/content:latest", Type: v2alpha1.TypeOCPReleaseContent},
	}}

	copiedImages, err := worker.Worker(context.Background(), collectorSchema, opts)

	assert.Error(t, err)
	assert.IsType(t, &BatchError{}, err)
	assert.Empty(t, copiedImages.AllImages)
}
