package batch

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
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
