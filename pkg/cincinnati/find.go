package cincinnati

import (
	"errors"
	"sort"

	"github.com/blang/semver/v4"

	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
)

var ErrNoPreviousRelease = errors.New("no previous release downloads detected")

// FindRelease will find the latest or first release recorded in a mirror
func FindRelease(mirror v1alpha1.Mirror, min bool) (string, semver.Version, error) {
	vers, err := findReleases(mirror, min)
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

func findReleases(mirror v1alpha1.Mirror, min bool) (map[string]semver.Version, error) {
	vers := make(map[string]semver.Version)
	for _, ch := range mirror.OCP.Channels {

		// Get the latest and first semver download in each channel
		if min {
			min, err := semver.Parse(ch.MinVersion)
			if err != nil {
				return vers, err
			}
			vers[ch.Name] = min
		} else {
			max, err := semver.Parse(ch.MaxVersion)
			if err != nil {
				return vers, err
			}
			vers[ch.Name] = max
		}
	}

	if len(vers) != 0 {
		return vers, nil
	}

	return nil, ErrNoPreviousRelease
}
