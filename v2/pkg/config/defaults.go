package config

import (
	"github.com/openshift/oc-mirror/v2/pkg/api/v2alpha1"
)

// Complete set default values in the ImageSetConfiguration
// when applicable
func Complete(cfg *v2alpha1.ImageSetConfiguration) {
	completeReleaseArchitectures(cfg)
}

func completeReleaseArchitectures(cfg *v2alpha1.ImageSetConfiguration) {
	if len(cfg.Mirror.Platform.Channels) != 0 && len(cfg.Mirror.Platform.Architectures) == 0 {
		cfg.Mirror.Platform.Architectures = []string{v2alpha1.DefaultPlatformArchitecture}
	}
}

// Complete set default values in the DeleteImageSetConfiguration
// when applicable
func CompleteDelete(cfg *v2alpha1.DeleteImageSetConfiguration) {
	completeReleaseArchitecturesDelete(cfg)
}

func completeReleaseArchitecturesDelete(cfg *v2alpha1.DeleteImageSetConfiguration) {
	if len(cfg.Delete.Platform.Channels) != 0 && len(cfg.Delete.Platform.Architectures) == 0 {
		cfg.Delete.Platform.Architectures = []string{v2alpha1.DefaultPlatformArchitecture}
	}
}
