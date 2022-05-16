package operator

import (
	"sort"

	"github.com/blang/semver/v4"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/model"
)

// IncludeConfigManager defines methods for IncludeConfig creation and manipulation.
type IncludeConfigManager interface {
	ConvertDCToIncludeConfig(declcfg.DeclarativeConfig) (v1alpha2.IncludeConfig, error)
	UpdateIncludeConfig(declcfg.DeclarativeConfig, v1alpha2.IncludeConfig) (v1alpha2.IncludeConfig, error)
}

var _ IncludeConfigManager = &catalogStrategy{}

type catalogStrategy struct{}

// NewCatalogStrategy will return the catalog implementation for the IncludeConfigManager.
func NewCatalogStrategy() IncludeConfigManager {
	return &catalogStrategy{}
}

// ConvertDCToIncludeConfig converts a heads-only rendered declarative config to an IncludeConfig
// with all of the package channels with the lowest bundle version.
func (s *catalogStrategy) ConvertDCToIncludeConfig(dc declcfg.DeclarativeConfig) (ic v1alpha2.IncludeConfig, err error) {
	inputModel, err := declcfg.ConvertToModel(dc)
	if err != nil {
		return ic, err
	}
	for _, mpkg := range inputModel {
		icPkg := v1alpha2.IncludePackage{
			Name: mpkg.Name,
		}
		// Pass in the empty includePackage here for
		// catalog strategy as there is in incoming
		// include config to process.
		icPkg.Channels = getFirstChannelBundles(*mpkg, icPkg)
		sortChannels(icPkg.Channels)
		ic.Packages = append(ic.Packages, icPkg)
	}
	sortPackages(ic.Packages)
	return ic, nil
}

// UpdateIncludeConfig will process the current IncludeConfig to add any new packages or channels. Starting versions are
// also validated and incremented if the version no longer exists in the catalog.
func (s *catalogStrategy) UpdateIncludeConfig(dc declcfg.DeclarativeConfig, curr v1alpha2.IncludeConfig) (ic v1alpha2.IncludeConfig, err error) {
	inputModel, err := declcfg.ConvertToModel(dc)
	if err != nil {
		return ic, err
	}

	currPackages := make(map[string]v1alpha2.IncludePackage, len(curr.Packages))
	for _, pkg := range curr.Packages {
		currPackages[pkg.Name] = pkg
	}

	// If there is a new package get the channel head.
	// If existing validate the starting bundle is
	// still in the catalog and iterate, if needed.
	for _, mpkg := range inputModel {
		icPkg := v1alpha2.IncludePackage{
			Name: mpkg.Name,
		}
		currPkg, found := currPackages[mpkg.Name]
		if !found {
			chWithHeads, err := getChannelHeads(*mpkg)
			icPkg.Channels = chWithHeads
			if err != nil {
				return ic, err
			}
		} else {
			// Pass in the empty includePackage here for
			// catalog strategy as there is in incoming
			// include config to process.
			ch, err := getCurrentChannelBundles(*mpkg, currPkg, icPkg)
			if err != nil {
				return ic, err
			}
			icPkg.Channels = ch
		}

		sortChannels(icPkg.Channels)
		ic.Packages = append(ic.Packages, icPkg)
	}
	sortPackages(ic.Packages)
	return ic, nil
}

var _ IncludeConfigManager = &packageStrategy{}

type packageStrategy struct {
	curr v1alpha2.IncludeConfig
}

// NewPackageStrategy will return the package implementation for the IncludeConfigManager.
// The current IncludeConfig specified through user-configuration is used to determine
// what packages should be managed and what packages have configuration information set.
func NewPackageStrategy(curr v1alpha2.IncludeConfig) IncludeConfigManager {
	return &packageStrategy{curr}
}

// ConvertDCToIncludeConfig converts a heads-only rendered declarative config to an IncludeConfig
// with all of the package channels with the lowest bundle version.
func (s *packageStrategy) ConvertDCToIncludeConfig(dc declcfg.DeclarativeConfig) (ic v1alpha2.IncludeConfig, err error) {
	inputModel, err := declcfg.ConvertToModel(dc)
	if err != nil {
		return ic, err
	}

	currPackages := make(map[string]v1alpha2.IncludePackage, len(s.curr.Packages))
	for _, pkg := range s.curr.Packages {
		currPackages[pkg.Name] = pkg
	}

	// Check if the package is new or has
	// version configuration set at the package level
	// if not process at the channel level.
	for _, mpkg := range inputModel {
		icPkg, found := currPackages[mpkg.Name]
		if !found || !includePackageVersionsSet(icPkg) {
			icPkg = v1alpha2.IncludePackage{
				Name:     mpkg.Name,
				Channels: getFirstChannelBundles(*mpkg, icPkg),
			}
			sortChannels(icPkg.Channels)
		}
		ic.Packages = append(ic.Packages, icPkg)
	}
	sortPackages(ic.Packages)
	return ic, nil
}

// UpdateIncludeConfig will process the currently managed IncludeConfig to add any new packages or channels. Starting versions are
// also validated and incremented if the version no longer exists in the catalog.
func (s *packageStrategy) UpdateIncludeConfig(dc declcfg.DeclarativeConfig, prev v1alpha2.IncludeConfig) (ic v1alpha2.IncludeConfig, err error) {
	inputModel, err := declcfg.ConvertToModel(dc)
	if err != nil {
		return ic, err
	}

	// currPackages are the package from the user-defined IncludeConfig
	currPackages := make(map[string]v1alpha2.IncludePackage, len(s.curr.Packages))
	for _, pkg := range s.curr.Packages {
		currPackages[pkg.Name] = pkg
	}

	// prevPackages is the most recent version of managed IncludeConfig values
	prevPackages := make(map[string]v1alpha2.IncludePackage, len(prev.Packages))
	for _, pkg := range prev.Packages {
		prevPackages[pkg.Name] = pkg
	}

	// If there is a new package get the channel head.
	// If existing validate the starting bundle is
	// still in the catalog and iterate, if needed.
	for _, mpkg := range inputModel {
		// Check if the package is new or has
		// version configuration set at the package level
		// if not process at the channel level.
		icPkg, found := currPackages[mpkg.Name]
		if !found || !includePackageVersionsSet(icPkg) {
			prevPkg, found := prevPackages[mpkg.Name]
			if !found {
				chWithHeads, err := getChannelHeads(*mpkg)
				if err != nil {
					return ic, err
				}
				icPkg = v1alpha2.IncludePackage{
					Name:     mpkg.Name,
					Channels: chWithHeads,
				}
			} else {
				chs, err := getCurrentChannelBundles(*mpkg, prevPkg, icPkg)
				if err != nil {
					return ic, err
				}
				icPkg = v1alpha2.IncludePackage{
					Name:     mpkg.Name,
					Channels: chs,
				}
			}
			sortChannels(icPkg.Channels)
		}
		ic.Packages = append(ic.Packages, icPkg)
	}
	sortPackages(ic.Packages)
	return ic, nil
}

// getFirstBundleByChannel will get the first bundle available in the channel in semver order
// for the model package for any channels that are not configured for the current
// include config package.
func getFirstChannelBundles(mpkg model.Package, icPkg v1alpha2.IncludePackage) []v1alpha2.IncludeChannel {
	currIncludeChannels := make(map[string]v1alpha2.IncludeChannel, len(icPkg.Channels))
	for _, ch := range icPkg.Channels {
		currIncludeChannels[ch.Name] = ch
	}

	channels := []v1alpha2.IncludeChannel{}
	for _, ch := range mpkg.Channels {
		newCh, found := currIncludeChannels[ch.Name]
		if !found || !includeChannelVersionsSet(newCh) {
			newCh = v1alpha2.IncludeChannel{
				Name: ch.Name,
			}

			keys := make([]string, 0, len(ch.Bundles))
			for k := range ch.Bundles {
				keys = append(keys, k)
			}
			sort.Slice(keys, func(i, j int) bool {
				return ch.Bundles[keys[i]].Version.GT(ch.Bundles[keys[j]].Version)
			})

			firstBundle := ch.Bundles[keys[len(keys)-1]].Version
			b := v1alpha2.IncludeBundle{
				MinVersion: firstBundle.String(),
			}
			newCh.IncludeBundle = b

		}
		channels = append(channels, newCh)
	}

	return channels
}

// getCurrBundles will get the current bundle available in the channel in semver order
// for the model package.
// The channel heads are added for new channels
// Configuration channels from the current are added as is
// Channel bundles from the previously managed package are checked
// against the bundles in the model package.
func getCurrentChannelBundles(mpkg model.Package, prevPkg, currPkg v1alpha2.IncludePackage) ([]v1alpha2.IncludeChannel, error) {

	// Add every bundle with a specified bundle name or
	// directly satisfying a bundle version to bundles.
	prevBundleByChannel := make(map[string]v1alpha2.IncludeBundle, len(prevPkg.Channels))
	for _, ch := range prevPkg.Channels {
		prevBundleByChannel[ch.Name] = ch.IncludeBundle
	}

	currIncludeChannels := make(map[string]v1alpha2.IncludeChannel, len(currPkg.Channels))
	for _, ch := range currPkg.Channels {
		currIncludeChannels[ch.Name] = ch
	}

	channels := []v1alpha2.IncludeChannel{}

	for _, ch := range mpkg.Channels {

		newCh, found := currIncludeChannels[ch.Name]
		if !found || !includeChannelVersionsSet(newCh) {
			newCh = v1alpha2.IncludeChannel{
				Name: ch.Name,
			}

			// Gather bundle information from
			// channel in declarative config to verify
			// currently available bundles.
			bundleSet := make(map[string]struct{}, len(ch.Bundles))
			versionsToInclude := []semver.Version{}
			for _, b := range ch.Bundles {
				bundleSet[b.Version.String()] = struct{}{}
				versionsToInclude = append(versionsToInclude, b.Version)
			}

			// If the channel is new, return the channel head.
			// If the channel is found and the bundle is found,
			// keep the current include bundle. If the target version
			// does not exist in the bundle set, sort by version and
			// find the next version using binary search.
			existsInBundleSet := func(v string) bool {
				_, found := bundleSet[v]
				return found
			}

			var startingBundle v1alpha2.IncludeBundle
			var err error
			icBundle, found := prevBundleByChannel[ch.Name]
			switch {
			case !found:
				startingBundle, err = getHeadBundle(*ch)
			case existsInBundleSet(icBundle.MinVersion):
				startingBundle = icBundle
			default:
				minVer, merr := semver.Parse(icBundle.MinVersion)
				if merr != nil {
					return nil, merr
				}
				startingBundle, err = findNextBundle(versionsToInclude, minVer)
			}
			if err != nil {
				return nil, err
			}
			newCh.IncludeBundle = startingBundle
		}
		channels = append(channels, newCh)
	}
	return channels, nil
}

// findNextBundle will find the next highest bundle in a set of versions in relation
// to the target.
func findNextBundle(versions []semver.Version, target semver.Version) (v1alpha2.IncludeBundle, error) {
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].LT(versions[j])
	})
	nextVersion := search(versions, target, 0, len(versions)-1)
	return v1alpha2.IncludeBundle{MinVersion: nextVersion.String()}, nil
}

// search perform a binary search to find the next highest version in relation
// to the target.
func search(versions []semver.Version, target semver.Version, low, high int) semver.Version {
	// If the target is the highest version, there is no next
	// version so return
	if versions[len(versions)-1].EQ(target) {
		return semver.Version{}
	}

	if high <= low {
		return versions[low]
	}

	mid := low + (high-low)/2
	if versions[mid].EQ(target) {
		return versions[mid+1]
	}

	if target.GT(versions[mid]) {
		return search(versions, target, mid+1, high)
	}

	return search(versions, target, low, mid-1)
}

// getChannelHeads proccess each channel in package and sets the starting
// bundle to the channel head.
func getChannelHeads(mpkg model.Package) ([]v1alpha2.IncludeChannel, error) {
	channels := []v1alpha2.IncludeChannel{}

	for _, ch := range mpkg.Channels {
		// initialize channel
		c := v1alpha2.IncludeChannel{
			Name: ch.Name,
		}

		b, err := getHeadBundle(*ch)
		if err != nil {
			return nil, err
		}
		c.IncludeBundle = b
		channels = append(channels, c)
	}
	return channels, nil
}

// getHeadBundle return the channel head bundle for the current channel.
func getHeadBundle(mch model.Channel) (v1alpha2.IncludeBundle, error) {
	bundle, err := mch.Head()
	if err != nil {
		return v1alpha2.IncludeBundle{}, err
	}

	return v1alpha2.IncludeBundle{MinVersion: bundle.Version.String()}, nil
}

// includePackageVersionsSet will verify if user-set version
// information is set on an Include Config for an IncludePackage.
func includePackageVersionsSet(pkg v1alpha2.IncludePackage) bool {
	switch {
	case pkg.MinVersion != "":
		return true
	case pkg.MinBundle != "":
		return true
	case pkg.MaxVersion != "":
		return true
	}
	return false
}

// includeChannelVersionsSet will verify if user-set version
// information is set on an Include Config in an IncludeChannel.
func includeChannelVersionsSet(ch v1alpha2.IncludeChannel) bool {
	switch {
	case ch.MinVersion != "":
		return true
	case ch.MinBundle != "":
		return true
	case ch.MaxVersion != "":
		return true
	}
	return false
}

func sortPackages(pkgs []v1alpha2.IncludePackage) {
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].Name < pkgs[j].Name
	})
}

func sortChannels(chs []v1alpha2.IncludeChannel) {
	sort.Slice(chs, func(i, j int) bool {
		return chs[i].Name < chs[j].Name
	})
}
