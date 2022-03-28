package operator

import (
	"sort"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/model"
)

// ConvertDCToIncludeConfig converts a heads-only rendered declarative config to an IncludeConfig
// with all of the package channels with the lowest bundle version.
func ConvertDCToIncludeConfig(dc declcfg.DeclarativeConfig) (ic v1alpha2.IncludeConfig, err error) {
	model, err := declcfg.ConvertToModel(dc)
	if err != nil {
		return ic, err
	}
	for _, mpkg := range model {
		icPkg := v1alpha2.IncludePackage{
			Name:     mpkg.Name,
			Channels: traverseModelChannels(*mpkg),
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

func traverseModelChannels(mpkg model.Package) []v1alpha2.IncludeChannel {
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
