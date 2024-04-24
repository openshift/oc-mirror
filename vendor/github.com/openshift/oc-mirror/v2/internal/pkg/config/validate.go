package config

import (
	"fmt"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type validationFunc func(cfg *v2alpha1.ImageSetConfiguration) error
type validationDeleteFunc func(cfg *v2alpha1.DeleteImageSetConfiguration) error

var validationChecks = []validationFunc{validateOperatorOptions, validateReleaseChannels}
var validationDeleteChecks = []validationDeleteFunc{validateOperatorOptionsDelete, validateReleaseChannelsDelete}

// Validate will check an ImagesetConfiguration for input errors.
func Validate(cfg *v2alpha1.ImageSetConfiguration) error {
	var errs []error
	for _, check := range validationChecks {
		if err := check(cfg); err != nil {
			errs = append(errs, fmt.Errorf("invalid configuration: %v", err))
		}
	}
	return utilerrors.NewAggregate(errs)
}

func validateOperatorOptions(cfg *v2alpha1.ImageSetConfiguration) error {
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
		if err := validateOperatorFiltering(ctlg); err != nil {
			return err
		}

		seen[ctlgName] = true
	}
	return nil
}
func validateOperatorFiltering(ctlg v2alpha1.Operator) error {
	if len(ctlg.Packages) > 0 {
		for _, pkg := range ctlg.Packages {
			if len(pkg.SelectedBundles) > 0 && (len(pkg.Channels) > 0 || pkg.MaxVersion != "" || pkg.MinVersion != "") {
				return fmt.Errorf("catalog %q: operator %q: mixing both filtering by bundles and filtering by channels or minVersion/maxVersion is not allowed", ctlg.Catalog, pkg.Name)
			}
		}
	}
	return nil
}
func validateReleaseChannels(cfg *v2alpha1.ImageSetConfiguration) error {
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

// ValidateDelete will check an DeleteImagesetConfiguration for input errors.
func ValidateDelete(cfg *v2alpha1.DeleteImageSetConfiguration) error {
	var errs []error
	for _, check := range validationDeleteChecks {
		if err := check(cfg); err != nil {
			errs = append(errs, fmt.Errorf("invalid configuration: %v", err))
		}
	}
	return utilerrors.NewAggregate(errs)
}

func validateOperatorOptionsDelete(cfg *v2alpha1.DeleteImageSetConfiguration) error {
	seen := map[string]bool{}
	for _, ctlg := range cfg.Delete.Operators {
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

func validateReleaseChannelsDelete(cfg *v2alpha1.DeleteImageSetConfiguration) error {
	seen := map[string]bool{}
	for _, channel := range cfg.Delete.Platform.Channels {
		if seen[channel.Name] {
			return fmt.Errorf(
				"release channel %q: duplicate found in configuration", channel.Name,
			)
		}
		seen[channel.Name] = true
	}
	return nil
}
