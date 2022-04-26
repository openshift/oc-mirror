package operator

import (
	"sort"

	"github.com/blang/semver/v4"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/model"
)

// ConvertDCToIncludeConfig converts a heads-only rendered declarative config to an IncludeConfig
// with all of the package channels with the lowest bundle version.
func ConvertDCToIncludeConfig(dc declcfg.DeclarativeConfig) (ic v1alpha2.IncludeConfig, err error) {
	inputModel, err := declcfg.ConvertToModel(dc)
	if err != nil {
		return ic, err
	}
	for _, mpkg := range inputModel {
		icPkg := v1alpha2.IncludePackage{
			Name:     mpkg.Name,
			Channels: getFirstBundle(*mpkg),
		}
		sort.Slice(icPkg.Channels, func(i, j int) bool {
			return icPkg.Channels[i].Name < icPkg.Channels[j].Name
		})
		ic.Packages = append(ic.Packages, icPkg)
	}
	sort.Slice(ic.Packages, func(i, j int) bool {
		return ic.Packages[i].Name < ic.Packages[j].Name
	})
	return ic, nil
}

func getFirstBundle(mpkg model.Package) []v1alpha2.IncludeChannel {
	channels := []v1alpha2.IncludeChannel{}

	for _, ch := range mpkg.Channels {
		// initialize channel
		c := v1alpha2.IncludeChannel{
			Name: ch.Name,
		}

		keys := make([]string, 0, len(ch.Bundles))
		for k := range ch.Bundles {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			return ch.Bundles[keys[i]].Version.GT(ch.Bundles[keys[j]].Version)
		})

		b := v1alpha2.IncludeBundle{
			StartingVersion: ch.Bundles[keys[len(keys)-1]].Version,
		}
		c.IncludeBundle = b
		channels = append(channels, c)
	}

	return channels
}

// UpdateIncludeConfig will process the current IncludeConfig to add any new packages or channels. Starting versions are
// also validated and incremented if the version no longer exists in the catalog.
func UpdateIncludeConfig(dc declcfg.DeclarativeConfig, curr v1alpha2.IncludeConfig) (ic v1alpha2.IncludeConfig, err error) {
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
			ch, err := getCurrBundle(*mpkg, currPkg)
			if err != nil {
				return ic, err
			}
			icPkg.Channels = ch
		}

		sort.Slice(icPkg.Channels, func(i, j int) bool {
			return icPkg.Channels[i].Name < icPkg.Channels[j].Name
		})
		ic.Packages = append(ic.Packages, icPkg)
	}
	sort.Slice(ic.Packages, func(i, j int) bool {
		return ic.Packages[i].Name < ic.Packages[j].Name
	})
	return ic, nil
}

// getCurrBundle will process the current package return the starting bundle based
// on the previous IncludeConfig
func getCurrBundle(mpkg model.Package, icPkg v1alpha2.IncludePackage) ([]v1alpha2.IncludeChannel, error) {

	// Add every bundle with a specified bundle name or
	// directly satisfying a bundle version to bundles.
	includeChannels := make(map[string]v1alpha2.IncludeBundle, len(icPkg.Channels))
	for _, ch := range icPkg.Channels {
		includeChannels[ch.Name] = ch.IncludeBundle
	}
	channels := []v1alpha2.IncludeChannel{}

	for _, ch := range mpkg.Channels {
		// initialize channel
		c := v1alpha2.IncludeChannel{
			Name: ch.Name,
		}

		bundleSet := make(map[string]struct{}, len(ch.Bundles))
		versionsToInclude := []semver.Version{}
		for _, b := range ch.Bundles {
			bundleSet[b.Version.String()] = struct{}{}
			versionsToInclude = append(versionsToInclude, b.Version)
		}

		var startingBundle v1alpha2.IncludeBundle
		var err error
		icBundle, found := includeChannels[ch.Name]

		// If the channel is new, return the channel head.
		// If the channel is found and the bundle is found,
		// keep the current include bundle. If the target version
		// does not exist in the bundle set, sort by version and
		// find the next version using binary search.
		if !found {
			startingBundle, err = getHeadBundle(*ch)
			if err != nil {
				return nil, err
			}
			c.IncludeBundle = startingBundle
			channels = append(channels, c)
			continue
		} else if _, found = bundleSet[icBundle.StartingVersion.String()]; found {
			startingBundle = icBundle
		} else {
			versionsToInclude = append(versionsToInclude, icBundle.StartingVersion)
			startingBundle, err = findNextBundle(versionsToInclude, icBundle.StartingVersion)
			if err != nil {
				return nil, err
			}
		}

		c.IncludeBundle = startingBundle
		channels = append(channels, c)
	}
	return channels, nil
}

// findNextBundle will find the next highest bundle in a set of versions in relation
// to the target.
func findNextBundle(versions []semver.Version, target semver.Version) (v1alpha2.IncludeBundle, error) {
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].LT(versions[j])
	})
	nextVerision := search(versions, target, 0, len(versions)-1)
	return v1alpha2.IncludeBundle{StartingVersion: nextVerision}, nil
}

// search perform a binary search to find the next highest version in relation
// to the target.
func search(versions []semver.Version, target semver.Version, low, high int) semver.Version {
	// If the target is the highest version, there is no next
	// version so return
	if high < low || versions[len(versions)-1].EQ(target) {
		return semver.Version{}
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

	return v1alpha2.IncludeBundle{StartingVersion: bundle.Version}, nil
}
