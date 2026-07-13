package integration_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// releaseImagesRepo is the destination repository for the pinned platform release image
// itself, as opposed to platformReleaseRepo which holds the release's referenced
// component images.
const releaseImagesRepo = "openshift/release-images"

var _ = Describe("secure policy", func() {
	var workDir string

	iscHappyPath := filepath.Join("happy_path", "isc-happy-path.yaml")

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	It("should not error in mirrorToDisk and diskToMirror with --secure-policy=true", func() {
		cfg := parseImageSetConfig(filepath.Join(iscDir, iscHappyPath))
		releaseTag := extractTag(cfg.Mirror.Platform.Release)
		catalogRepo := extractRepositoryName(cfg.Mirror.Operators[0].Catalog)
		catalogTag := extractTag(cfg.Mirror.Operators[0].Catalog)
		// --secure-policy=true falls back to the system-wide /etc/containers/policy.json when
		// --policy is not set. That file is not guaranteed to exist in every environment (e.g.
		// CI images), so an explicit permissive policy is supplied to keep the test hermetic.
		policyPath := filepath.Join(policiesDir, "insecure-accept-anything.json")

		By("running mirrorToDisk with --secure-policy=true")
		result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscHappyPath), workDir,
			"--secure-policy=true", "--policy", policyPath)
		expectOcMirrorCommandSuccess(result, err)

		By("verifying the release image has a signature tag in the local cache")
		expectSignatureTagInLocalCache(cacheDir, releaseImagesRepo, releaseTag)

		By("verifying the rebuilt operator catalog has no signature tag in the local cache")
		expectNoSignatureTagInLocalCache(cacheDir, catalogRepo, catalogTag)

		By("running diskToMirror with --secure-policy=true")
		result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscHappyPath), workDir, testRegistry.Endpoint(),
			"--secure-policy=true", "--policy", policyPath, "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying all images are mirrored in the local registry")
		expectSuccessfulMirrorInRegistry(filepath.Join(iscDir, iscHappyPath), *testRegistry)
	})

	It("should still collect and rebuild the operator catalog when a strict trust policy rejects unsigned images", func() {
		iscOperatorsOnly := filepath.Join("secure_policy", "isc-operators-only.yaml")
		cfg := parseImageSetConfig(filepath.Join(iscDir, iscOperatorsOnly))
		policyPath := filepath.Join(policiesDir, "reject-all.json")

		By("running mirrorToDisk with --secure-policy=true and a reject-all trust policy")
		result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscOperatorsOnly), workDir,
			"--secure-policy=true", "--policy", policyPath)

		By("verifying the operator catalog was pulled and converted to OCI format on disk despite the reject-all policy")
		expectCatalogCollectedInWorkingDir(workDir, cfg.Mirror.Operators[0].Catalog)

		By("verifying the run failed, since the final image copies are rejected by policy")
		expectOcMirrorExitCode(result, err, 4, "is rejected by policy")
		expectNoTarArchive(workDir)
	})

	It("should mirror signed images and reject unsigned images under a sigstoreSigned trust policy", func() {
		isc := filepath.Join("secure_policy", "isc-signed-catalog-unsigned-additional.yaml")
		cfg := parseImageSetConfig(filepath.Join(iscDir, isc))
		catalogRepo := extractRepositoryName(cfg.Mirror.Operators[0].Catalog)
		catalogTag := extractTag(cfg.Mirror.Operators[0].Catalog)
		additionalImageRepo := extractRepositoryName(cfg.Mirror.AdditionalImages[0].Name)
		policyPath := writeSigstoreSignedPolicy(workDir, filepath.Join(keysDir, "cosign.pub"), "quay.io/oc-mirror")

		By("running mirrorToDisk with --secure-policy=true and a policy requiring cosign signatures for quay.io/oc-mirror")
		result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, isc), workDir,
			"--secure-policy=true", "--policy", policyPath)
		expectOcMirrorExitCode(result, err, 8, "2 / 2 operator images mirrored successfully")

		By("verifying the signed operator catalog was actually written to the local cache")
		Expect(localCacheHasTag(cacheDir, catalogRepo, catalogTag)).To(BeTrue(),
			"expected signed catalog %s:%s to be present in the local cache", catalogRepo, catalogTag)

		By("verifying the unsigned additional image was never written to the local cache")
		expectNoRepositoryInLocalCache(cacheDir, additionalImageRepo)

		By("verifying no tar archive was produced, since the additional image failed to mirror")
		expectNoTarArchive(workDir)
	})
})

// expectSignatureTagInLocalCache verifies that the given repo/tag in the local oc-mirror
// cache has a corresponding <digest>.sig tag, proving a signature was fetched and cached.
func expectSignatureTagInLocalCache(cacheDir, repo, tag string) {
	digest := localCacheTagDigest(cacheDir, repo, tag)
	sigTag := strings.Replace(digest, ":", "-", 1) + ".sig"
	Expect(localCacheHasTag(cacheDir, repo, sigTag)).To(BeTrue(),
		"expected signature tag %q for %s:%s (digest %s) in local cache", sigTag, repo, tag, digest)
}

// expectNoSignatureTagInLocalCache verifies that the given repo/tag in the local oc-mirror
// cache has no corresponding <digest>.sig tag. This is expected for rebuilt/filtered
// catalog images, which are built locally and therefore never signed.
func expectNoSignatureTagInLocalCache(cacheDir, repo, tag string) {
	digest := localCacheTagDigest(cacheDir, repo, tag)
	sigTag := strings.Replace(digest, ":", "-", 1) + ".sig"
	Expect(localCacheHasTag(cacheDir, repo, sigTag)).To(BeFalse(),
		"unexpected signature tag %q for %s:%s (digest %s) in local cache", sigTag, repo, tag, digest)
}

// localCacheTagDigest reads the manifest digest that a tag in the local oc-mirror cache
// points to, by resolving its "current" link file.
func localCacheTagDigest(cacheDir, repo, tag string) string {
	linkPath := filepath.Join(cacheDir, cacheRepositoriesSubdir, repo, "_manifests", "tags", tag, "current", "link")
	data, err := os.ReadFile(linkPath)
	Expect(err).NotTo(HaveOccurred(), "failed to read tag digest link for %s:%s", repo, tag)
	return strings.TrimSpace(string(data))
}

// localCacheHasTag returns whether a tag directory exists for the given repo in the local
// oc-mirror cache.
func localCacheHasTag(cacheDir, repo, tag string) bool {
	_, err := os.Stat(filepath.Join(cacheDir, cacheRepositoriesSubdir, repo, "_manifests", "tags", tag))
	return err == nil
}

// expectNoRepositoryInLocalCache verifies that the given repository was never written to the
// local oc-mirror cache, e.g. because its image copy was rejected by a trust policy before
// completing.
func expectNoRepositoryInLocalCache(cacheDir, repo string) {
	_, err := os.Stat(filepath.Join(cacheDir, cacheRepositoriesSubdir, repo))
	Expect(os.IsNotExist(err)).To(BeTrue(),
		"expected repository %q to be absent from the local cache, but it exists", repo)
}

// expectCatalogCollectedInWorkingDir verifies that oc-mirror pulled the given operator catalog
// and converted it to OCI format under the working directory. This succeeds independently of
// whether the final image copy to the destination is later allowed or rejected by policy.
func expectCatalogCollectedInWorkingDir(workDir, catalogRef string) {
	catalogName := extractImageName(catalogRef)
	pattern := filepath.Join(workDir, dirWorkingDir, dirOperatorCatalog, catalogName, "*", "catalog-image", "index.json")
	matches, err := filepath.Glob(pattern)
	Expect(err).NotTo(HaveOccurred())
	Expect(matches).NotTo(BeEmpty(),
		"expected operator catalog %q to be collected into OCI format under the working directory (%s)", catalogRef, pattern)
}

// writeSigstoreSignedPolicy writes a container trust policy file to workDir that rejects
// everything by default, except images under repoPrefix, which must carry a valid sigstore
// signature verifiable with the cosign public key at cosignPubKeyPath. The key content is
// read from disk rather than hardcoded, so the policy always matches the current test key.
func writeSigstoreSignedPolicy(workDir, cosignPubKeyPath, repoPrefix string) string {
	keyData, err := os.ReadFile(cosignPubKeyPath)
	Expect(err).NotTo(HaveOccurred())

	policy := map[string]any{
		"default": []map[string]string{{"type": "reject"}},
		"transports": map[string]any{
			"docker": map[string]any{
				repoPrefix: []map[string]any{
					{
						"type":           "sigstoreSigned",
						"keyData":        base64.StdEncoding.EncodeToString(keyData),
						"signedIdentity": map[string]string{"type": "matchRepository"},
					},
				},
			},
		},
	}

	data, err := json.Marshal(policy)
	Expect(err).NotTo(HaveOccurred())

	policyPath := filepath.Join(workDir, "policy.json")
	Expect(os.WriteFile(policyPath, data, 0o644)).To(Succeed())
	return policyPath
}
