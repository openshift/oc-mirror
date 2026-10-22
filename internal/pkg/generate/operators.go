package generate

import (
	"maps"
	"slices"

	"github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/model"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

// buildOperator describes a mirrored (already filtered) catalog. The catalog in
// the mirror only holds what was mirrored, so each package and channel is
// selected with version bounds read from its bundles. Full stays unset: the
// catalog filter rejects Full combined with version bounds.
func buildOperator(ref string, m model.Model) v2alpha1.Operator {
	op := v2alpha1.Operator{Catalog: ref}
	for _, name := range slices.Sorted(maps.Keys(m)) {
		pkg := m[name]
		ip := v2alpha1.IncludePackage{Name: name}
		for _, chName := range slices.Sorted(maps.Keys(pkg.Channels)) {
			ic := v2alpha1.IncludeChannel{Name: chName}
			var lo, hi *semver.Version
			for _, b := range pkg.Channels[chName].Bundles {
				v := b.Version
				if lo == nil || v.LT(*lo) {
					lo = &v
				}
				if hi == nil || v.GT(*hi) {
					hi = &v
				}
			}
			if lo != nil {
				ic.MinVersion, ic.MaxVersion = lo.String(), hi.String()
			}
			ip.Channels = append(ip.Channels, ic)
		}
		op.Packages = append(op.Packages, ip)
	}
	return op
}
