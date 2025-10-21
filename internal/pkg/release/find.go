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
	if min {
		sort.Slice(keys, func(i, j int) bool {
			return vers[keys[i]].GT(vers[keys[j]])
		})
	} else {
		sort.Slice(keys, func(i, j int) bool {
			return vers[keys[i]].LT(vers[keys[j]])
		})
	}

	return keys[len(keys)-1], vers[keys[len(keys)-1]], nil
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
