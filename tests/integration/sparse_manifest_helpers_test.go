package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

// expectOnlyPlatformBlobsPresent verifies that only the specified platforms have
// reachable blobs in the registry for the given manifest-list image.
// Other platforms may appear in the index but their blobs must be absent (HTTP 404).
func expectOnlyPlatformBlobsPresent(ctx context.Context, reg *registry.Registry, repo, tag string, expectedPlatforms []string) {
	GinkgoHelper()
	present, missing := getPlatformBlobPresence(ctx, reg, repo, tag)
	for _, exp := range expectedPlatforms {
		found := false
		for _, got := range present {
			if got == exp || strings.HasPrefix(got, exp+"/") {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(),
			"expected platform %q (or a variant such as %s/v8) to have a blob in the registry", exp, exp)
	}
	for _, got := range present {
		matched := false
		for _, exp := range expectedPlatforms {
			if got == exp || strings.HasPrefix(got, exp+"/") {
				matched = true
				break
			}
		}
		Expect(matched).To(BeTrue(),
			"unexpected platform %q has a blob in the registry (should be filtered out)", got)
	}
	_ = missing // platforms in index but no blob — expected for sparse manifests
}

// expectAllPlatformBlobsPresent verifies that every platform entry in the
// manifest list has a reachable blob (no sparse filtering applied).
func expectAllPlatformBlobsPresent(ctx context.Context, reg *registry.Registry, repo, tag string) {
	GinkgoHelper()
	_, missing := getPlatformBlobPresence(ctx, reg, repo, tag)
	Expect(missing).To(BeEmpty(),
		"expected all platform blobs to be present, but these are missing: %v", missing)
}

// getPlatformBlobPresence returns two lists: platforms with blobs present and
// platforms whose blobs are missing (listed in index but not stored).
func getPlatformBlobPresence(ctx context.Context, reg *registry.Registry, repo, tag string) (present, missing []string) {
	GinkgoHelper()

	refStr := fmt.Sprintf("%s/%s:%s", reg.Endpoint(), repo, tag)
	ref, err := name.ParseReference(refStr, name.Insecure)
	Expect(err).NotTo(HaveOccurred())

	idx, err := remote.Index(ref,
		remote.WithContext(ctx),
		remote.WithAuth(authn.Anonymous))
	Expect(err).NotTo(HaveOccurred(), "expected a manifest list at %s", refStr)

	manifest, err := idx.IndexManifest()
	Expect(err).NotTo(HaveOccurred())
	Expect(manifest.Manifests).NotTo(BeEmpty(), "manifest list has no entries")

	for _, m := range manifest.Manifests {
		if m.Platform == nil {
			continue
		}
		plat := m.Platform.OS + "/" + m.Platform.Architecture
		if m.Platform.Variant != "" {
			plat += "/" + m.Platform.Variant
		}

		// Check if the per-platform manifest blob is reachable.
		manifestURL := fmt.Sprintf("http://%s/v2/%s/manifests/%s", reg.Endpoint(), repo, m.Digest.String())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json,application/vnd.oci.image.manifest.v1+json")

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			present = append(present, plat)
		} else {
			missing = append(missing, plat)
		}
	}
	return present, missing
}
