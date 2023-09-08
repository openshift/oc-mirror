package config

import (
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
)

// Complete set default values in the ImageSetConfiguration
// when applicable
func Complete(cfg *v1alpha2.ImageSetConfiguration) {
	completeReleaseArchitectures(cfg)
}

func completeReleaseArchitectures(cfg *v1alpha2.ImageSetConfiguration) {
	if len(cfg.Mirror.Platform.Channels) != 0 && len(cfg.Mirror.Platform.Architectures) == 0 {
		cfg.Mirror.Platform.Architectures = []string{v1alpha2.DefaultPlatformArchitecture}
	}
}
