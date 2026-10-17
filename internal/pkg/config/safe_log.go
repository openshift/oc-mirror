package config

import (
	"fmt"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

// DeleteImageSetConfigurationLogSummary returns a log-safe summary of delete configuration
// content without emitting user-controlled blocked image patterns.
func DeleteImageSetConfigurationLogSummary(cfg v2alpha1.DeleteImageSetConfiguration) string {
	return fmt.Sprintf(
		"platformChannels=%d operators=%d additionalImages=%d helmLocalCharts=%d helmRepos=%d blockedImages=%d",
		len(cfg.Delete.Platform.Channels),
		len(cfg.Delete.Operators),
		len(cfg.Delete.AdditionalImages),
		len(cfg.Delete.Helm.Local),
		len(cfg.Delete.Helm.Repositories),
		len(cfg.Delete.BlockedImages),
	)
}

// ImageSetConfigurationLogSummary returns a log-safe summary of mirror configuration
// content used by delete workflows without emitting user-controlled blocked image patterns.
func ImageSetConfigurationLogSummary(cfg v2alpha1.ImageSetConfiguration) string {
	return fmt.Sprintf(
		"platformChannels=%d operators=%d additionalImages=%d helmLocalCharts=%d helmRepos=%d blockedImages=%d",
		len(cfg.Mirror.Platform.Channels),
		len(cfg.Mirror.Operators),
		len(cfg.Mirror.AdditionalImages),
		len(cfg.Mirror.Helm.Local),
		len(cfg.Mirror.Helm.Repositories),
		len(cfg.Mirror.BlockedImages),
	)
}
