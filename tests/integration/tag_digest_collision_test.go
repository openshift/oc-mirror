package integration_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
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
		})
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
})
