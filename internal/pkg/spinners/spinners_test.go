package spinners

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/openshift/oc-mirror/v2/internal/pkg/emoji"
)

func TestStatusDecorator(t *testing.T) {
	type testCase struct {
		name     string
		stats    decor.Statistics
		expected string
	}

	testCases := []testCase{
		{
			name:     "running spinner has no status mark",
			stats:    decor.Statistics{},
			expected: "",
		},
		{
			name:     "completed spinner shows check mark",
			stats:    decor.Statistics{Completed: true},
			expected: emoji.SpinnerCheckMark,
		},
		{
			name:     "aborted spinner shows cross mark",
			stats:    decor.Statistics{Aborted: true},
			expected: emoji.SpinnerCrossMark,
		},
		{
			name:     "completed and aborted prefers check mark",
			stats:    decor.Statistics{Completed: true, Aborted: true},
			expected: emoji.SpinnerCheckMark,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := statusDecorator().Decor(tc.stats)
			assert.Equal(t, tc.expected, got)
		})
	}
}
