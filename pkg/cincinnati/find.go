package cincinnati

import (
	"errors"
	"sort"

	"github.com/blang/semver/v4"

	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
)

var ErrNoPreviousRelease = errors.New("no previous release downloads detected")

// FindLastRelease will find the latest release that has been recorded in the metadata
func FindLastRelease(meta v1alpha1.Metadata, channel string) (string, semver.Version, error) {
	vers, err := findLastReleases(meta)
	if err != nil {
		return "", semver.Version{}, err
	}

	keys := make([]string, 0, len(vers))
	for k := range vers {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return vers[keys[i]].LT(vers[keys[j]])
	})

	return keys[len(keys)-1], vers[keys[len(keys)-1]], nil
}

func findLastReleases(meta v1alpha1.Metadata) (map[string]semver.Version, error) {
	vers := make(map[string]semver.Version)
	for _, mirror := range meta.PastMirrors {
		for _, ch := range mirror.Mirror.OCP.Channels {
			// If the there are no versions specified
			// for this channel continue
			if len(ch.Versions) == 0 {
				continue
			}
			// Get the latest semver download in each channel
			vers[ch.Name] = getLatestSemVer(ch.Versions)
		}
	}
	// Find the latest download version between the channels
	if len(vers) != 0 {
		return vers, nil
	}
	return nil, ErrNoPreviousRelease
}

func getLatestSemVer(stringVers []string) semver.Version {
	vers := []semver.Version{}
	for _, stringVer := range stringVers {
		vers = append(vers, semver.MustParse(stringVer))
	}
	if len(vers) == 0 {
		return semver.Version{}
	}
	semver.Sort(vers)
	return vers[len(vers)-1]
}
