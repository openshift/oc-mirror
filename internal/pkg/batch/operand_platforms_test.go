package batch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func platformMiss(platform string) error {
	return fmt.Errorf("preparing instances for copy: no instances found for platform %s", platform)
}

func TestCopyAvailableOperandPlatforms(t *testing.T) {
	requested := []string{"linux/amd64", "linux/arm64", "linux/ppc64le"}
	tests := []struct {
		name      string
		initial   error
		filters   [][]string
		errors    []error
		available []string
		missing   []string
		wantErr   bool
	}{
		{
			name:      "one platform missing",
			initial:   platformMiss("linux/arm64"),
			filters:   [][]string{{"linux/amd64", "linux/ppc64le"}},
			errors:    []error{nil},
			available: []string{"linux/amd64", "linux/ppc64le"},
			missing:   []string{"linux/arm64"},
		},
		{
			name:      "multiple platforms missing",
			initial:   platformMiss("linux/arm64"),
			filters:   [][]string{{"linux/amd64", "linux/ppc64le"}, {"linux/amd64"}},
			errors:    []error{platformMiss("linux/ppc64le"), nil},
			available: []string{"linux/amd64"},
			missing:   []string{"linux/arm64", "linux/ppc64le"},
		},
		{
			name:    "none available never retries without filters",
			initial: platformMiss("linux/amd64"),
			filters: [][]string{{"linux/arm64", "linux/ppc64le"}, {"linux/ppc64le"}},
			errors:  []error{platformMiss("linux/arm64"), platformMiss("linux/ppc64le")},
			missing: requested,
		},
		{
			name:    "a retry failing for another reason stays fatal",
			initial: platformMiss("linux/arm64"),
			filters: [][]string{{"linux/amd64", "linux/ppc64le"}},
			errors:  []error{errors.New("unauthorized")},
			wantErr: true,
		},
		{
			name:    "an unrequested platform in the error is not removed",
			initial: platformMiss("linux/s390x"),
			wantErr: true,
		},
		{
			name:    "an unrecognized platform error stays fatal",
			initial: errors.New("no instances found for platform"),
			wantErr: true,
		},
		{
			name:    "a repeated platform error cannot loop",
			initial: platformMiss("linux/arm64"),
			filters: [][]string{{"linux/amd64", "linux/ppc64le"}},
			errors:  []error{platformMiss("linux/arm64")},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := new(MirrorMock)
			opts := platformFilterOpts(t)
			opts.InstancePlatforms = slices.Clone(requested)
			img := operandSchema().AllImages[0]
			for i, filters := range tt.filters {
				m.On("Run", mock.Anything, img.Source, img.Destination, mirror.CopyMode,
					mock.MatchedBy(func(o *mirror.CopyOptions) bool {
						return slices.Equal(o.InstancePlatforms, filters) && o.PreserveDigests == opts.PreserveDigests
					})).Return(tt.errors[i]).Once()
			}
			w := &ChannelConcurrentBatch{Mirror: m}
			available, missing, err := w.copyAvailableOperandPlatforms(context.Background(), img, opts, tt.initial)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, missing, "failed copies must not be recorded as skipped")
			} else {
				require.NoError(t, err)
				var got []string
				for _, p := range available {
					got = append(got, p.String())
				}
				assert.Equal(t, tt.available, got)
				assert.Equal(t, tt.missing, missing)
			}
			assert.Equal(t, requested, opts.InstancePlatforms, "do not mutate the caller's options")
			m.AssertExpectations(t)
			assert.Len(t, m.Calls, len(tt.filters))
		})
	}

	t.Run("cancellation prevents retries", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		m := new(MirrorMock)
		w := &ChannelConcurrentBatch{Mirror: m}
		opts := platformFilterOpts(t)
		opts.InstancePlatforms = requested
		_, _, err := w.copyAvailableOperandPlatforms(ctx, operandSchema().AllImages[0], opts, platformMiss("linux/arm64"))
		require.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, m.Calls)
	})
}

// OCPBUGS-132865: a bundle image that another bundle lists as a related image can be copied as
// a related image. It is still the bundle image of a bundle that references it, so its copy
// failure is kept: the operand resolver is not asked and no platform is removed.
func TestRetryOperandCopyKeepsBundleImagesStrict(t *testing.T) {
	const otherBundle = "registry/ns/other-bundle@sha256:c30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebed"
	bundleRefs := map[string]string{
		"digest":         bundleRef,
		"tag":            consts.DockerProtocol + "registry/ns/bundle:v1.0.0",
		"tag and digest": consts.DockerProtocol + "registry/ns/bundle:v1.0.0@sha256:a30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebeb",
	}
	for refName, ref := range bundleRefs {
		t.Run("bundle image by "+refName+" also listed as a related image", func(t *testing.T) {
			img := v2alpha1.CopyImageSchema{Source: ref, Origin: ref, Destination: "docker://dest/bundle", Type: v2alpha1.TypeOperatorRelatedImage}
			schema := v2alpha1.CollectorSchema{CopyImageSchemaMap: v2alpha1.CopyImageSchemaMap{BundlesByImage: map[string]map[string]string{
				img.Origin: {strings.TrimPrefix(ref, consts.DockerProtocol): "own.v1.0.0", otherBundle: "other.v1.0.0"},
			}}}
			opts := platformFilterOpts(t)
			opts.InstancePlatforms = []string{"linux/amd64", "linux/arm64"}
			copyErr := platformMiss("linux/arm64")
			m := new(MirrorMock)
			resolver := &resolverStub{operand: true}
			w := &ChannelConcurrentBatch{Log: clog.New("error"), Mirror: m, OperandResolver: resolver}

			platforms, skipped, err := w.retryOperandCopy(context.Background(), img, schema, opts, copyErr)

			require.ErrorIs(t, err, copyErr, "the copy failure is kept")
			assert.Empty(t, platforms)
			assert.False(t, skipped)
			assert.Zero(t, resolver.callCount(), "a bundle image is never handed to the operand resolver")
			assert.Empty(t, m.Calls, "no platform is removed and retried")
		})
	}

	t.Run("related image that is no bundle image", func(t *testing.T) {
		img := operandSchema().AllImages[0]
		schema := v2alpha1.CollectorSchema{CopyImageSchemaMap: v2alpha1.CopyImageSchemaMap{BundlesByImage: map[string]map[string]string{
			img.Origin: {strings.TrimPrefix(bundleRef, consts.DockerProtocol): "own.v1.0.0", otherBundle: "other.v1.0.0"},
		}}}
		opts := platformFilterOpts(t)
		opts.InstancePlatforms = []string{"linux/amd64", "linux/arm64"}
		m := new(MirrorMock)
		m.On("Run", mock.Anything, img.Source, img.Destination, mirror.CopyMode,
			mock.MatchedBy(func(o *mirror.CopyOptions) bool {
				return slices.Equal(o.InstancePlatforms, []string{"linux/amd64"})
			})).Return(nil).Once()
		resolver := &resolverStub{operand: true}
		w := &ChannelConcurrentBatch{Log: clog.New("error"), Mirror: m, OperandResolver: resolver}

		platforms, skipped, err := w.retryOperandCopy(context.Background(), img, schema, opts, platformMiss("linux/arm64"))

		require.NoError(t, err)
		assert.False(t, skipped)
		require.Len(t, platforms, 1)
		assert.Equal(t, "linux/amd64", platforms[0].String())
		assert.Equal(t, 1, resolver.callCount())
		m.AssertExpectations(t)
	})
}
