package release

import (
	"errors"
	"sort"

	"github.com/blang/semver/v4"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

// ErrNoPreviousRelease is returned when no releases can be found in the
// release channels.
var ErrNoPreviousRelease = errors.New("no previous release downloads detected")

// FindRelease will find the minimum or maximum release for a set of ReleaseChannels
func FindRelease(channels []v2alpha1.ReleaseChannel, min bool) (string, semver.Version, error) {
	vers, err := findReleases(channels, min)
	if err != nil {
		return "", semver.Version{}, err
	}

	keys := make([]string, 0, len(vers))
	for k := range vers {
		keys = append(keys, k)
	}
	// If two channels have the same version, pick the same one every time.
	// Otherwise, disk-to-mirror might look for a graph we didn't save.
	sort.Strings(keys)
	if min {
		sort.SliceStable(keys, func(i, j int) bool {
			return vers[keys[i]].GT(vers[keys[j]])
		})
	} else {
		sort.SliceStable(keys, func(i, j int) bool {
			return vers[keys[i]].LT(vers[keys[j]])
		})
	}

	return keys[len(keys)-1], vers[keys[len(keys)-1]], nil
}

// tiedChannels returns the selected channel followed by the other channels with the same minimum or
// maximum version, in the reverse name order in which FindRelease prefers them.
func tiedChannels(channels []v2alpha1.ReleaseChannel, selected string, version semver.Version, min bool) []string {
	vers, err := findReleases(channels, min)
	if err != nil {
		return []string{selected}
	}
	others := make([]string, 0, len(vers))
	for name, ver := range vers {
		if name != selected && ver.EQ(version) {
			others = append(others, name)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(others)))
	return append([]string{selected}, others...)
}

func findReleases(channels []v2alpha1.ReleaseChannel, min bool) (map[string]semver.Version, error) {
	vers := make(map[string]semver.Version, len(channels))
	if len(channels) == 0 {
		return vers, ErrNoPreviousRelease
	}

	for _, ch := range channels {

		ver := ch.MaxVersion
		if min {
			ver = ch.MinVersion
		}
		parsedVer, err := semver.Parse(ver)
		if err != nil {
			return vers, err
		}
		vers[ch.Name] = parsedVer
	}

	return vers, nil
}
