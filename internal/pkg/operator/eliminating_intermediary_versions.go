package operator

import (
	"strings"

	"github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/declcfg"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

func eliminatingIntermediaryVersions(dc *declcfg.DeclarativeConfig, iscCatalogFilter v2alpha1.Operator, log clog.PluggableLoggerInterface) *declcfg.DeclarativeConfig {
	for _, pkg := range dc.Packages {
		channels := make([]declcfg.Channel, 0, len(dc.Channels))
		for _, chanel := range dc.Channels {
			if chanel.Package == pkg.Name {
				maxVersion := foundMaxVersion(chanel.Name, pkg.Name, iscCatalogFilter.Packages)
				chanel.Entries = eliminatingIntermediaryVersionsWithMaxVersion(chanel, maxVersion, log)
			}
			channels = append(channels, chanel)
		}
		dc.Channels = channels
	}
	return dc
}

func foundMaxVersion(chanelName, pkgName string, packages []v2alpha1.IncludePackage) string {
	for _, pkg := range packages {
		if pkg.Name == pkgName {
			for _, chanel := range pkg.Channels {
				if chanel.Name == chanelName {
					return chanel.MaxVersion
				}
			}
		}
	}
	return ""
}

// eliminatingIntermediaryVersionsWithMaxVersion eliminates intermediary versions between maxVersion and the head.
// Starting from the channel head, it follows the replaces chain, collecting every entry whose version is greater
// than maxVersion. The walk stops at the first entry that is not greater than maxVersion; that entry becomes the
// head's new replaces target. Elimination only proceeds when the head skips every version it would then jump over
// (see skipLookGood).
func eliminatingIntermediaryVersionsWithMaxVersion(channel declcfg.Channel, maxVersion string, log clog.PluggableLoggerInterface) []declcfg.ChannelEntry {
	maxV, err := semver.Parse(maxVersion)
	if err != nil {
		log.Error("error parsing max version %s: %v", maxVersion, err)
		return channel.Entries
	}
	if len(channel.Entries) == 0 {
		return channel.Entries
	}
	head, ok := findHead(channel.Entries)
	if !ok {
		return channel.Entries
	}
	eliminated, target, ok := intermediariesAboveMaxVersion(channel.Entries, head, maxV)
	if !ok {
		return channel.Entries
	}
	// The head must skip every version it would jump over once the intermediaries are removed:
	// the eliminated entries plus the new replaces target.
	if !skipLookGood(head, append(eliminated, target)) {
		return channel.Entries
	}
	head.Replaces = target.Name
	return rebuildWithoutEliminated(channel, head, eliminated, log)
}

// findHead returns the channel head: the first entry that no other entry replaces. ok is false when every entry
// is replaced by another (e.g. a fully cyclic chain).
func findHead(entries []declcfg.ChannelEntry) (declcfg.ChannelEntry, bool) {
	replaced := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.Replaces != "" {
			replaced[e.Replaces] = struct{}{}
		}
	}
	for _, e := range entries {
		if _, isReplaced := replaced[e.Name]; !isReplaced {
			return e, true
		}
	}
	return declcfg.ChannelEntry{}, false
}

// intermediariesAboveMaxVersion follows the replaces chain from the head and returns every entry whose version is
// greater than maxV, along with the first entry that is not greater (the head's new replaces target). ok is false
// when nothing can be eliminated: the chain ends before reaching such a target, it cycles, or no entry exceeds maxV.
func intermediariesAboveMaxVersion(entries []declcfg.ChannelEntry, head declcfg.ChannelEntry, maxV semver.Version) (eliminated []declcfg.ChannelEntry, target declcfg.ChannelEntry, ok bool) {
	byName := make(map[string]declcfg.ChannelEntry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}
	visited := map[string]struct{}{}
	for current := head; ; {
		if _, seen := visited[current.Name]; seen {
			return nil, declcfg.ChannelEntry{}, false // cyclic replaces chain in untrusted catalog data
		}
		visited[current.Name] = struct{}{}
		next, found := byName[current.Replaces]
		if !found {
			return nil, declcfg.ChannelEntry{}, false // chain ends before reaching maxVersion
		}
		if !greater(next.Name, maxV) {
			return eliminated, next, len(eliminated) > 0
		}
		eliminated = append(eliminated, next)
		current = next
	}
}

// rebuildWithoutEliminated returns the channel entries with the eliminated intermediaries removed and the head
// replaced by its updated form, preserving the original ordering of the remaining entries.
func rebuildWithoutEliminated(channel declcfg.Channel, head declcfg.ChannelEntry, eliminated []declcfg.ChannelEntry, log clog.PluggableLoggerInterface) []declcfg.ChannelEntry {
	gone := make(map[string]struct{}, len(eliminated))
	for _, e := range eliminated {
		gone[e.Name] = struct{}{}
		log.Info("eliminating intermediary version %q for channel %q of package %q", e.Name, channel.Name, channel.Package)
	}
	result := make([]declcfg.ChannelEntry, 0, len(channel.Entries)-len(eliminated))
	for _, e := range channel.Entries {
		if e.Name == head.Name {
			result = append(result, head)
		} else if _, removed := gone[e.Name]; !removed {
			result = append(result, e)
		}
	}
	return result
}

// skipLookGood reports whether the head can safely absorb the jump over the given entries. After elimination
// the head replaces its new target directly, so it is safe only when the head skips every version it jumps
// over: each entry in jumped. A version is skipped when its name is listed in the head's Skips or when it
// falls within the head's SkipRange.
func skipLookGood(head declcfg.ChannelEntry, jumped []declcfg.ChannelEntry) bool {
	if len(jumped) == 0 {
		return false
	}
	skips := make(map[string]struct{}, len(head.Skips))
	for _, s := range head.Skips {
		skips[s] = struct{}{}
	}
	// A missing or malformed SkipRange simply provides no coverage.
	var skipRange semver.Range
	if head.SkipRange != "" {
		skipRange, _ = semver.ParseRange(head.SkipRange)
	}
	for _, entry := range jumped {
		if _, listed := skips[entry.Name]; listed {
			continue
		}
		if v, ok := versionFromEntryName(entry.Name); ok && skipRange != nil && skipRange(v) {
			continue
		}
		return false
	}
	return true
}

// versionFromEntryName extracts the semver from a channel entry name. Names look
// like "<package>.<semver>" or "<package>.v<semver>" (e.g. "rhods-operator.3.4.3"
// or "foo.v1.3.0"). Because the package name carries no version-like suffix, the
// version is the trailing part after the leftmost dot (with an optional leading
// "v") that parses as a full semver.
func versionFromEntryName(entryName string) (semver.Version, bool) {
	for i := range len(entryName) {
		if entryName[i] != '.' {
			continue
		}
		candidate := strings.TrimPrefix(entryName[i+1:], "v")
		if v, err := semver.Parse(candidate); err == nil {
			return v, true
		}
	}
	return semver.Version{}, false
}

func greater(entryName string, version semver.Version) bool {
	entryVersion, ok := versionFromEntryName(entryName)
	if !ok {
		return false
	}
	return entryVersion.GT(version)
}
