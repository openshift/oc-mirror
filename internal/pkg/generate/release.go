package generate

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/blang/semver/v4"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

const (
	releaseImagesRepo  = "openshift/release-images"
	releaseContentRepo = "openshift/release"
)

// Tag format: <version>-<arch>, for example 4.16.3-x86_64 or 4.17.0-rc.1-multi.
var releaseTagRe = regexp.MustCompile(`^(\d+\.\d+\.\d+.*)-(x86_64|aarch64|ppc64le|s390x|multi)$`)

var archNames = map[string]string{
	"x86_64":  "amd64",
	"aarch64": "arm64",
	"ppc64le": "ppc64le",
	"s390x":   "s390x",
	"multi":   "multi",
}

// isRepo reports whether repo is base, optionally under a registry path prefix.
func isRepo(repo, base string) bool {
	return repo == base || strings.HasSuffix(repo, "/"+base)
}

func isSignatureTag(tag string) bool {
	return strings.HasSuffix(tag, ".sig") || strings.HasSuffix(tag, ".att") || strings.HasSuffix(tag, ".sbom")
}

// buildPlatform infers platform channels and architectures from the tags of
// the release-images repository. Channels are derived as stable-X.Y; the
// original channel names cannot be recovered from a registry.
func buildPlatform(tags []string) (v2alpha1.Platform, []string) {
	type span struct{ min, max semver.Version }
	spans := map[string]*span{}
	archs := map[string]struct{}{}
	var warns []string

	for _, tag := range tags {
		if isSignatureTag(tag) {
			continue
		}
		m := releaseTagRe.FindStringSubmatch(tag)
		if m == nil {
			continue
		}
		v, err := semver.Parse(m[1])
		if err != nil {
			warns = append(warns, fmt.Sprintf("release tag %q skipped: %v", tag, err))
			continue
		}
		archs[archNames[m[2]]] = struct{}{}
		key := fmt.Sprintf("%d.%d", v.Major, v.Minor)
		s, ok := spans[key]
		if !ok {
			spans[key] = &span{min: v, max: v}
			continue
		}
		if v.LT(s.min) {
			s.min = v
		}
		if v.GT(s.max) {
			s.max = v
		}
	}

	var p v2alpha1.Platform
	if len(spans) == 0 {
		return p, warns
	}
	for _, key := range slices.Sorted(maps.Keys(spans)) {
		p.Channels = append(p.Channels, v2alpha1.ReleaseChannel{
			Name:       "stable-" + key,
			Type:       v2alpha1.TypeOCP,
			MinVersion: spans[key].min.String(),
			MaxVersion: spans[key].max.String(),
		})
	}
	p.Architectures = slices.Sorted(maps.Keys(archs))
	warns = append(warns, "platform channels are inferred as stable-X.Y with min/max versions from release tags; review before use")
	return p, warns
}
