package config

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type validationFunc func(cfg *v2alpha1.ImageSetConfiguration) []error
type validationDeleteFunc func(cfg *v2alpha1.DeleteImageSetConfiguration) error

var validationChecks = []validationFunc{validateOperatorOptions, validateReleaseChannels}
var validationDeleteChecks = []validationDeleteFunc{validateOperatorOptionsDelete, validateReleaseChannelsDelete}

// Validate will check an ImagesetConfiguration for input errors.
func Validate(cfg *v2alpha1.ImageSetConfiguration) error {
	var errs []error
	for _, check := range validationChecks {
		if validationErrs := check(cfg); len(validationErrs) > 0 {
			errs = append(errs, fmt.Errorf("invalid configuration: %v", utilerrors.NewAggregate(validationErrs)))
		}
	}
	return utilerrors.NewAggregate(errs)
}

func validateOperatorOptions(cfg *v2alpha1.ImageSetConfiguration) []error {
	catalogs := sets.New[string]()
	errs := []error{}
	for _, ctlg := range cfg.Mirror.Operators {
		ctlgName, err := ctlg.GetUniqueName()
		if err != nil {
			errs = append(errs, err)
		}
		if catalogs.Has(ctlgName) {
			errs = append(errs, fmt.Errorf(
				"catalog %q: duplicate found in configuration", ctlgName,
			))
		}
		catalogs.Insert(ctlgName)

		if filterErrs := validateOperatorFiltering(ctlg); len(filterErrs) > 0 {
			errs = append(errs, filterErrs...)
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func validateOperatorFiltering(ctlg v2alpha1.Operator) []error {
	errs := []error{}
	if len(ctlg.Packages) > 0 {
		for _, pkg := range ctlg.Packages {
			if pkg.MaxVersion != "" || pkg.MinVersion != "" {
				if pkg.MaxVersion != "" {
					if _, err := semver.NewVersion(pkg.MaxVersion); err != nil {
						errs = append(errs, fmt.Errorf("catalog %q: operator %q: maxVersion %q must respect semantic versioning notation", ctlg.Catalog, pkg.Name, pkg.MaxVersion))
					}
				}
				if pkg.MinVersion != "" {
					if _, err := semver.NewVersion(pkg.MinVersion); err != nil {
						errs = append(errs, fmt.Errorf("catalog %q: operator %q: minVersion %q must respect semantic versioning notation", ctlg.Catalog, pkg.Name, pkg.MinVersion))
					}
				}

				if len(pkg.Channels) > 0 {
					for _, chFilter := range pkg.Channels {
						if chFilter.MaxVersion != "" || chFilter.MinVersion != "" {
							errs = append(errs, fmt.Errorf("catalog %q: operator %q: mixing both filtering by minVersion/maxVersion and filtering by channel minVersion/maxVersion is not allowed", ctlg.Catalog, pkg.Name))
						}
					}
				}
			}
			if len(pkg.Channels) > 0 {
				for _, chFilter := range pkg.Channels {
					if chFilter.MaxVersion != "" {
						if _, err := semver.NewVersion(chFilter.MaxVersion); err != nil {
							errs = append(errs, fmt.Errorf("catalog %q: operator %q: channel %q: maxVersion %q must respect semantic versioning notation", ctlg.Catalog, pkg.Name, chFilter.Name, chFilter.MaxVersion))
						}
					}
					if chFilter.MinVersion != "" {
						if _, err := semver.NewVersion(chFilter.MinVersion); err != nil {
							errs = append(errs, fmt.Errorf("catalog %q: operator %q: channel %q: minVersion %q must respect semantic versioning notation", ctlg.Catalog, pkg.Name, chFilter.Name, chFilter.MinVersion))
						}
					}
				}
			}
		}
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func validateReleaseChannels(cfg *v2alpha1.ImageSetConfiguration) []error {
	channels := sets.New[string]()
	for _, channel := range cfg.Mirror.Platform.Channels {
		if channels.Has(channel.Name) {
			return []error{fmt.Errorf(
				"release channel %q: duplicate found in configuration", channel.Name,
			)}
		}
		channels.Insert(channel.Name)
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
	catalogs := sets.New[string]()
	for _, ctlg := range cfg.Delete.Operators {
		ctlgName, err := ctlg.GetUniqueName()
		if err != nil {
			return err
		}
		if catalogs.Has(ctlgName) {
			return fmt.Errorf(
				"catalog %q: duplicate found in configuration", ctlgName,
			)
		}
		catalogs.Insert(ctlgName)
	}
	return nil
}

func validateReleaseChannelsDelete(cfg *v2alpha1.DeleteImageSetConfiguration) error {
	channels := sets.New[string]()
	for _, channel := range cfg.Delete.Platform.Channels {
		if channels.Has(channel.Name) {
			return fmt.Errorf(
				"release channel %q: duplicate found in configuration", channel.Name,
			)
		}
		channels.Insert(channel.Name)
	}
	return nil
}
