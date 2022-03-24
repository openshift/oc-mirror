package config

import (
	"fmt"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
)

type validationFunc func(cfg *v1alpha2.ImageSetConfiguration) error

var validationChecks = []validationFunc{validateOperatorOptions, validateReleaseChannels}

func Validate(cfg *v1alpha2.ImageSetConfiguration) error {
	var errs []error
	for _, check := range validationChecks {
		if err := check(cfg); err != nil {
			errs = append(errs, fmt.Errorf("invalid configuration: %v", err))
		}
	}
	return utilerrors.NewAggregate(errs)
}

func validateOperatorOptions(cfg *v1alpha2.ImageSetConfiguration) error {
	for _, ctlg := range cfg.Mirror.Operators {
		if len(ctlg.IncludeConfig.Packages) != 0 && ctlg.IsHeadsOnly() {
			return fmt.Errorf("catalog %q: cannot define packages with full key set to false", ctlg.Catalog)
		}
	}
	return nil
}

func validateReleaseChannels(cfg *v1alpha2.ImageSetConfiguration) error {
	seen := map[string]bool{}
	for _, channel := range cfg.Mirror.OCP.Channels {
		if seen[channel.Name] {
			return fmt.Errorf(
				"release channel %q: duplicate found in configuration", channel.Name,
			)
		}
		seen[channel.Name] = true
	}
	return nil
}
