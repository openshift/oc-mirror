package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

// These tests reproduce the tag+digest cache collision: when two images share the same
// repository and tag but pin different digests (the "repo:tag@sha256:..." form), oc-mirror
// stores them in the local cache under the tag only. A registry tag resolves to a single
// digest, so the archive keeps blob data for only one of them and the other is lost - it
// ends up with link pointers but no blob data in the tar, and is "manifest unknown" at the
// destination after diskToMirror.
//
// The assertions verify that BOTH digests survive the mirrorToDisk -> diskToMirror round
// trip: their blob data is present in the tar archive and both are resolvable by digest in
// the destination registry. They fail against the current (buggy) behavior and pass once
// tag+digest images are cached under a per-digest tag.
const (
	// collisionRepo is the repository (path component) the colliding images land under.
	collisionRepo = "oc-mirror/oc-mirror-dev"

	// collisionDigestA and collisionDigestB are two distinct, real manifests that already
	// exist in quay.io/oc-mirror/oc-mirror-dev. Both are referenced under the same
	// "shared-operand" tag, so they collide on a single cache tag.
	collisionDigestA = "sha256:1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58"
	collisionDigestB = "sha256:1b8392488dabcf78c82c72866d34edf6ef3d5bfb6ec00c81a377486a90d3d9ad"
)

var _ = Describe("tag and digest collision", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	// Requires the test-catalog-tag-digest-collision catalog image to be built and pushed -
	// see tests/integration/image-builders/operator/catalogs/README.md.
	Describe("operator related images sharing a repo:tag with different digests", func() {
		iscFile := filepath.Join("tag_digest_collision", "isc-operator-tag-digest-collision.yaml")

		It("preserves both related-image digests through mirrorToDisk and diskToMirror", func() {
			By("running mirrorToDisk")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscFile), workDir, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the tar archive contains blob data for BOTH colliding related-image digests")
			expectTarContainsBlobForDigests(workDir, collisionDigestA, collisionDigestB)

			By("running diskToMirror")
			result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying BOTH related-image digests are resolvable by digest in the destination registry")
			expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestA)
			expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestB)

			By("verifying the shared tag resolves to one of the two colliding related-image digests")
			expectTagResolvesToOneOfDigests(*testRegistry, collisionRepo, "shared-operand", collisionDigestA, collisionDigestB)
		})
	})

	Describe("additionalImages sharing a repo:tag with different digests", func() {
		iscFile := filepath.Join("tag_digest_collision", "isc-additional-tag-digest-collision.yaml")

		It("preserves both digests through mirrorToDisk and diskToMirror", func() {
			By("running mirrorToDisk")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscFile), workDir, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the tar archive contains blob data for BOTH colliding digests")
			expectTarContainsBlobForDigests(workDir, collisionDigestA, collisionDigestB)

			By("running diskToMirror")
			result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying BOTH digests are resolvable by digest in the destination registry")
			expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestA)
			expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestB)

			By("verifying the shared tag resolves to one of the two colliding digests")
			expectTagResolvesToOneOfDigests(*testRegistry, collisionRepo, "shared-operand", collisionDigestA, collisionDigestB)
		})
	})

	// Inverse scenario: two tags that share a single digest. This exercises the same
	// tag+digest cache-vs-destination code path as the collision specs above, but from the
	// opposite direction (one digest, many tags), and needs no operator catalog.
	Describe("additionalImages sharing a digest under different tags", func() {
		iscFile := filepath.Join("tag_digest_collision", "isc-additional-same-digest-tags.yaml")

		It("recreates both tags pointing to the shared digest through mirrorToDisk and diskToMirror", func() {
			By("running mirrorToDisk")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscFile), workDir, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the tar archive contains blob data for the shared digest")
			expectTarContainsBlobForDigests(workDir, collisionDigestA)

			By("running diskToMirror")
			result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the shared digest is resolvable and BOTH tags point to it")
			expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestA)
			expectTagResolvesToDigest(*testRegistry, collisionRepo, "same-digest-a", collisionDigestA)
			expectTagResolvesToDigest(*testRegistry, collisionRepo, "same-digest-b", collisionDigestA)
		})
	})

	// mirrorToMirror has no local cache, so unlike mirrorToDisk it cannot key colliding
	// digests under distinct per-digest cache tags. The destination tag can only ever point
	// at whichever digest was pushed last - a real registry limitation, not an oc-mirror bug
	// (see the OCPBUGS-105878 comment in buildMirrorToDiskPaths, which deliberately keeps M2M
	// on the human tag). What must still hold is that BOTH digests are faithfully copied with
	// no data loss: even though only one ends up owning the shared tag, both remain
	// independently resolvable by digest immediately after the run.
	Describe("additionalImages sharing a repo:tag with different digests (mirrorToMirror)", func() {
		iscFile := filepath.Join("tag_digest_collision", "isc-additional-tag-digest-collision.yaml")

		It("copies both digests with no data loss, even though only one keeps the shared tag", func() {
			By("running mirrorToMirror")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying BOTH digests are resolvable by digest in the destination registry")
			expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestA)
			expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestB)

			By("verifying the shared tag resolves to one of the two colliding digests, not a broken state")
			expectTagResolvesToOneOfDigests(*testRegistry, collisionRepo, "shared-operand", collisionDigestA, collisionDigestB)
		})
	})

	// Uses the local chart at testdata/helm-charts/tag-digest-collision, whose container and
	// initContainer both reference oc-mirror-dev:shared-operand but pin the two colliding
	// digests - see that chart's templates/deployment.yaml for details.
	Describe("helm-referenced images sharing a repo:tag with different digests", func() {
		It("preserves both digests through mirrorToDisk and diskToMirror", func() {
			iscPath := resolveHelmISC(workDir)

			By("running mirrorToDisk")
			result, err := runner.MirrorToDisk(ctx, iscPath, workDir, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the tar archive contains blob data for BOTH colliding digests")
			expectTarContainsBlobForDigests(workDir, collisionDigestA, collisionDigestB)

			By("running diskToMirror")
			result, err = runner.DiskToMirror(ctx, iscPath, workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying BOTH digests are resolvable by digest in the destination registry")
			expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestA)
			expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestB)

			By("verifying the shared tag resolves to one of the two colliding digests")
			expectTagResolvesToOneOfDigests(*testRegistry, collisionRepo, "shared-operand", collisionDigestA, collisionDigestB)
		})
	})
})

// resolveHelmISC returns the path to a temp copy of the helm tag+digest
// collision ISC fixture with the local chart's `path:` rewritten to an absolute
// path. oc-mirror resolves a local chart's `path:` relative to its own process's
// CWD (not the ISC file's location), which isn't guaranteed to match the test
// binary's CWD in every environment - e.g. in this repo's Prow CI job the test
// binary's CWD is an unrelated artifacts directory, not the checkout. So the
// absolute path is derived from iscDir (already correctly rooted via
// ARTIFACTS_DIR, see integration_suite_test.go) rather than the process's CWD.
func resolveHelmISC(workDir string) string {
	const relChartPath = "testdata/helm-charts/tag-digest-collision"

	absChartPath := filepath.Join(filepath.Dir(iscDir), "helm-charts", "tag-digest-collision")

	srcISC := filepath.Join(iscDir, "tag_digest_collision", "isc-helm-tag-digest-collision.yaml")
	data, err := os.ReadFile(srcISC)
	Expect(err).NotTo(HaveOccurred())

	placeholder := "path: " + relChartPath
	Expect(string(data)).To(ContainSubstring(placeholder), "expected chart path placeholder %q in %s", placeholder, srcISC)
	rendered := strings.Replace(string(data), placeholder, "path: "+absChartPath, 1)

	dst := filepath.Join(workDir, "isc-helm-tag-digest-collision.yaml")
	Expect(os.WriteFile(dst, []byte(rendered), 0o644)).To(Succeed())
	return dst
}

// expectTarContainsBlobForDigests verifies that the tar archive in workDir contains the
// content-addressed blob data file for every given digest (e.g. "sha256:abcd..."). Because
// OCI blobs are content-addressed, the presence of blobs/sha256/<xx>/<hex>/data is proof
// that the manifest/config/layer bytes for that digest were archived. A missing entry is
// the fingerprint of the tag+digest cache collision: the digest has link pointers but no data.
func expectTarContainsBlobForDigests(workDir string, digests ...string) {
	matches, err := filepath.Glob(filepath.Join(workDir, "mirror_*.tar"))
	Expect(err).NotTo(HaveOccurred())
	Expect(matches).NotTo(BeEmpty(), "no tar archive found")

	entries := listTarEntries(matches[0])
	Expect(entries).NotTo(BeEmpty(), "tar archive has no entries")

	for _, d := range digests {
		hex := strings.TrimPrefix(d, "sha256:")
		Expect(len(hex)).To(BeNumerically(">", 2), "invalid digest %q", d)
		blobPath := fmt.Sprintf("docker/registry/v2/blobs/sha256/%s/%s/data", hex[:2], hex)
		expectTarContainsPath(entries, blobPath)
	}
}

// expectManifestExistsByDigest asserts that a manifest with the given digest can be resolved
// by digest in the destination registry. When an image is lost to a tag collision it is
// reported as "manifest unknown" (HTTP 404), which fails this assertion.
func expectManifestExistsByDigest(reg registry.Registry, repo, digest string) {
	exists, err := reg.ManifestExistsByDigest(ctx, repo, digest)
	Expect(err).NotTo(HaveOccurred(), "error resolving manifest %s@%s", repo, digest)
	Expect(exists).To(BeTrue(),
		"manifest %s@%s is missing from the destination registry (digest lost to a tag collision)", repo, digest)
}

// expectTagResolvesToDigest asserts that repo:tag in the destination registry resolves to
// the given digest. Used to verify the inverse of the tag+digest collision: two distinct
// tags that share a single digest must both survive and point to that digest.
func expectTagResolvesToDigest(reg registry.Registry, repo, tag, digest string) {
	got, err := reg.TagDigest(ctx, repo, tag)
	Expect(err).NotTo(HaveOccurred(), "error resolving tag %s:%s", repo, tag)
	Expect(got).To(Equal(digest),
		"tag %s:%s should resolve to %s but resolved to %s", repo, tag, digest, got)
}

// expectTagResolvesToOneOfDigests asserts that repo:tag in the destination registry resolves
// to one of the given candidate digests, without requiring which one specifically. Used
// wherever the real destination registry tag can only ever point at whichever digest was
// pushed last (last-write-wins is a real registry limitation, not something oc-mirror can or
// should work around, for either mirrorToMirror or diskToMirror). This only confirms the tag
// isn't left missing/broken.
func expectTagResolvesToOneOfDigests(reg registry.Registry, repo, tag string, candidates ...string) {
	got, err := reg.TagDigest(ctx, repo, tag)
	Expect(err).NotTo(HaveOccurred(), "error resolving tag %s:%s", repo, tag)
	Expect(candidates).To(ContainElement(got),
		"tag %s:%s resolved to %s, expected one of %v", repo, tag, got, candidates)
}
