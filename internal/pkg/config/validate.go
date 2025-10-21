package config

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type (
	validationFunc       func(cfg *v2alpha1.ImageSetConfiguration) []error
	validationDeleteFunc func(cfg *v2alpha1.DeleteImageSetConfiguration) error
)

var (
	validationChecks       = []validationFunc{validateOperatorOptions, validateReleaseChannels}
	validationDeleteChecks = []validationDeleteFunc{validateOperatorOptionsDelete, validateReleaseChannelsDelete}
)

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

		if opErrs := validateOperator(ctlg); len(opErrs) > 0 {
			errs = append(errs, opErrs...)
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func validateOperator(ctlg v2alpha1.Operator) []error {
	errs := []error{}
	packages := sets.New[string]()
	for _, pkg := range ctlg.Packages {
		if packages.Has(pkg.Name) {
			errs = append(errs, fmt.Errorf("catalog %q: duplicate package entry %q", ctlg.Catalog, pkg.Name))
		}
		packages.Insert(pkg.Name)

		if pkg.MaxVersion != "" {
			if _, err := semver.NewVersion(pkg.MaxVersion); err != nil {
				errs = append(errs, fmt.Errorf(
					"catalog %q: operator %q: maxVersion %q must respect semantic versioning notation",
					ctlg.Catalog, pkg.Name, pkg.MaxVersion,
				))
			}
		}
		if pkg.MinVersion != "" {
			if _, err := semver.NewVersion(pkg.MinVersion); err != nil {
				errs = append(errs, fmt.Errorf(
					"catalog %q: operator %q: minVersion %q must respect semantic versioning notation",
					ctlg.Catalog, pkg.Name, pkg.MinVersion,
				))
			}
		}

		errs = append(errs, validatePackageChannels(ctlg.Catalog, &pkg)...)
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func validatePackageChannels(ctlgName string, pkg *v2alpha1.IncludePackage) []error {
	errs := []error{}
	channels := sets.New[string]()
	for _, chFilter := range pkg.Channels {
		if channels.Has(chFilter.Name) {
			errs = append(errs, fmt.Errorf("catalog %q: operator %q: duplicate channel entry %q", ctlgName, pkg.Name, chFilter.Name))
		}
		channels.Insert(chFilter.Name)

		errs = append(errs, validatePackageChannel(ctlgName, pkg, &chFilter)...)
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func validatePackageChannel(ctlgName string, pkg *v2alpha1.IncludePackage, ch *v2alpha1.IncludeChannel) []error {
	errs := []error{}
	if ch.MaxVersion != "" {
		if _, err := semver.NewVersion(ch.MaxVersion); err != nil {
			errs = append(errs, fmt.Errorf(
				"catalog %q: operator %q: channel %q: maxVersion %q must respect semantic versioning notation",
				ctlgName, pkg.Name, ch.Name, ch.MaxVersion,
			))
		}
	}
	if ch.MinVersion != "" {
		if _, err := semver.NewVersion(ch.MinVersion); err != nil {
			errs = append(errs, fmt.Errorf(
				"catalog %q: operator %q: channel %q: minVersion %q must respect semantic versioning notation",
				ctlgName, pkg.Name, ch.Name, ch.MinVersion,
			))
		}
	}
	if (ch.MinVersion != "" || ch.MaxVersion != "") && (pkg.MinVersion != "" || pkg.MaxVersion != "") {
		errs = append(errs, fmt.Errorf(
			"catalog %q: operator %q: mixing both filtering by minVersion/maxVersion and filtering by channel minVersion/maxVersion is not allowed",
			ctlgName, pkg.Name,
		))
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
