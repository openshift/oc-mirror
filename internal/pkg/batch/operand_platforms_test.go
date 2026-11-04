package batch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

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
