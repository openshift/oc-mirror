package image

import (
	"testing"

	"github.com/openshift/library-go/pkg/image/registryclient"
	"github.com/stretchr/testify/require"
)

func TestNewContext(t *testing.T) {

	tests := []struct {
		name             string
		skipVerification bool
		expected         func(registryclient.Context) bool
		err              string
	}{{
		name:             "Valid/WithRetries",
		skipVerification: true,
		expected: func(ctx registryclient.Context) bool {
			return ctx.DisableDigestVerification
		},
	}, {
		name:             "Valid/WithSkipVerification",
		skipVerification: false,
		expected: func(ctx registryclient.Context) bool {
			return !ctx.DisableDigestVerification
		},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			regctx, err := NewContext(test.skipVerification)
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				require.True(t, test.expected(*regctx))
			}
		})
	}
}
