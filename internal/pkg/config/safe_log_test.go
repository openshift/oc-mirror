package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

func TestDeleteImageSetConfigurationLogSummaryDoesNotExposeBlockedPatterns(t *testing.T) {
	sensitivePattern := "secret-internal-registry.example.com/private-image"
	cfg := v2alpha1.DeleteImageSetConfiguration{
		DeleteImageSetConfigurationSpec: v2alpha1.DeleteImageSetConfigurationSpec{
			Delete: v2alpha1.Delete{
				Platform: v2alpha1.Platform{
					Channels: []v2alpha1.ReleaseChannel{{Name: "stable-4.22"}},
				},
				BlockedImages: []v2alpha1.BlockedImage{
					{Name: sensitivePattern},
				},
			},
		},
	}

	summary := DeleteImageSetConfigurationLogSummary(cfg)
	require.Contains(t, summary, "blockedImages=1")
	require.NotContains(t, summary, sensitivePattern)
}

func TestImageSetConfigurationLogSummaryDoesNotExposeBlockedPatterns(t *testing.T) {
	sensitivePattern := "secret-internal-registry.example.com/private-image"
	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				BlockedImages: []v2alpha1.BlockedImage{
					{Name: sensitivePattern},
				},
			},
		},
	}

	summary := ImageSetConfigurationLogSummary(cfg)
	require.Contains(t, summary, "blockedImages=1")
	require.NotContains(t, summary, sensitivePattern)
	require.True(t, strings.HasPrefix(summary, "platformChannels="))
}
