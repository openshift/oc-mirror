package generate

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

// Tags oc-mirror gives to images it mirrors by digest: 12 hex characters or
// sha256-<digest>. They belong to operators, releases or Helm charts and are
// indistinguishable from digest-mirrored additional images, so they are skipped.
var digestTagRe = regexp.MustCompile(`^([a-f0-9]{12}|sha256-[a-f0-9]{64})$`)

// buildAdditional turns the tags of one repository into additional images,
// sorted by tag. Signature tags are dropped silently; digest-derived tags are
// dropped and counted in skipped.
func buildAdditional(host, repo string, tags []string) (images []v2alpha1.AdditionalImage, skipped int) {
	for _, tag := range slices.Sorted(slices.Values(tags)) {
		if isSignatureTag(tag) {
			continue
		}
		if digestTagRe.MatchString(tag) {
			skipped++
			continue
		}
		images = append(images, v2alpha1.AdditionalImage{Name: fmt.Sprintf("%s/%s:%s", host, repo, tag)})
	}
	return images, skipped
}
