package config

import (
	"fmt"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
)

type validationFunc func(cfg *v1alpha2.ImageSetConfiguration) error

var validationChecks = []validationFunc{validateOperatorOptions, validateReleaseChannels}

// Validate will check an ImagesetConfiguration for input errors.
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
	seen := map[string]bool{}
	for _, ctlg := range cfg.Mirror.Operators {
		ctlgName, err := ctlg.GetUniqueName()
		if err != nil {
			return err
		}
		if seen[ctlgName] {
			return fmt.Errorf(
				"catalog %q: duplicate found in configuration", ctlgName,
			)
		}
		seen[ctlgName] = true
	}
	return nil
}

func validateReleaseChannels(cfg *v1alpha2.ImageSetConfiguration) error {
	seen := map[string]bool{}
	for _, channel := range cfg.Mirror.Platform.Channels {
		if seen[channel.Name] {
			return fmt.Errorf(
				"release channel %q: duplicate found in configuration", channel.Name,
			)
		}
		seen[channel.Name] = true
	}
	return nil
}
