package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

// These tests delete images that share a destination tag but have different digests (see
// tag_digest_collision_test.go for the mirroring side). A registry tag points at a single
// manifest, so the delete file must address each image by digest, and the deletion must remove
// every selected manifest exactly once.
var _ = Describe("delete images sharing a destination tag", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	deleteYaml := func() string {
		return filepath.Join(workDir, "working-dir", "delete", "delete-images.yaml")
	}

	It("deletes both digests of a repo:tag collision from the registry and the cache", func() {
		By("mirroring both digests through mirrorToDisk and diskToMirror")
		mirrorToDiskToMirror(workDir, collisionConfig("isc-additional-tag-digest-collision.yaml"))
		expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestA)
		expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestB)

		By("generating the delete file")
		result, err := runner.DeletePhaseOne(ctx, collisionConfig("disc-additional-tag-digest-collision.yaml"),
			workDir, "", testRegistry.Endpoint())
		expectOcMirrorCommandSuccess(result, err)

		By("verifying the delete file addresses each digest")
		Expect(deletePlanReferences(deleteYaml())).To(Equal(collisionReferences(testRegistry.Endpoint(), collisionDigestA, collisionDigestB)))

		By("deleting with --force-cache-delete")
		result, err = runner.DeletePhaseTwo(ctx, deleteYaml(), testRegistry.Endpoint(),
			"--dest-tls-verify=false", "--force-cache-delete=true")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying both manifests are gone from the registry and the cache")
		expectManifestAbsentByDigest(*testRegistry, collisionRepo, collisionDigestA)
		expectManifestAbsentByDigest(*testRegistry, collisionRepo, collisionDigestB)
		expectEmptyRegistry(*testRegistry)
		expectRepositoriesAbsentFromLocalCache(cacheDir, []string{collisionRepo})
	})

	It("deletes every digest-only image addressed as latest with --delete-v1-images", func() {
		By("caching both digest-only images")
		result, err := runner.MirrorToDisk(ctx, collisionConfig("isc-additional-digest-only-v1.yaml"), workDir, "--remove-signatures=true")
		expectOcMirrorCommandSuccess(result, err)

		By("seeding the destination the way oc-mirror v1 mirrored digest-only images: under :latest only")
		result, err = runner.DiskToMirror(ctx, collisionConfig("isc-additional-digest-only-v1.yaml"), workDir, testRegistry.Endpoint(),
			"--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		tagByDigest(*testRegistry, collisionRepo, "latest", collisionDigestA)
		tagByDigest(*testRegistry, collisionRepo, "latest", collisionDigestB)
		untag(*testRegistry, collisionRepo, digestTag(collisionDigestA))
		untag(*testRegistry, collisionRepo, digestTag(collisionDigestB))
		tags, err := testRegistry.ListTags(ctx, collisionRepo)
		Expect(err).NotTo(HaveOccurred())
		Expect(tags).To(ConsistOf("latest"))
		expectTagResolvesToDigest(*testRegistry, collisionRepo, "latest", collisionDigestB)
		expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestA)

		By("generating the delete file with --delete-v1-images")
		result, err = runner.DeletePhaseOne(ctx, collisionConfig("disc-additional-digest-only-v1.yaml"),
			workDir, "", testRegistry.Endpoint(), "--delete-v1-images")
		expectOcMirrorCommandSuccess(result, err)
		Expect(deletePlanReferences(deleteYaml())).To(Equal(collisionReferences(testRegistry.Endpoint(), collisionDigestA, collisionDigestB)))

		By("deleting with --force-cache-delete")
		result, err = runner.DeletePhaseTwo(ctx, deleteYaml(), testRegistry.Endpoint(),
			"--dest-tls-verify=false", "--force-cache-delete=true")
		expectOcMirrorCommandSuccess(result, err)

		expectManifestAbsentByDigest(*testRegistry, collisionRepo, collisionDigestA)
		expectManifestAbsentByDigest(*testRegistry, collisionRepo, collisionDigestB)
		expectEmptyRegistry(*testRegistry)
		expectRepositoriesAbsentFromLocalCache(cacheDir, []string{collisionRepo})
	})

	It("deletes both digest-only images sharing a targetTag mirrored with mirrorToMirror", func() {
		By("mirroring both digests with mirrorToMirror")
		result, err := runner.MirrorToMirror(ctx, collisionConfig("isc-additional-digest-targettag-collision.yaml"),
			workDir, testRegistry.Endpoint(), "--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestA)
		expectManifestExistsByDigest(*testRegistry, collisionRepo, collisionDigestB)
		expectTagResolvesToOneOfDigests(*testRegistry, collisionRepo, "shared-target", collisionDigestA, collisionDigestB)

		By("generating the delete file")
		result, err = runner.DeletePhaseOne(ctx, collisionConfig("disc-additional-digest-targettag-collision.yaml"),
			workDir, "", testRegistry.Endpoint())
		expectOcMirrorCommandSuccess(result, err)
		Expect(deletePlanReferences(deleteYaml())).To(Equal(collisionReferences(testRegistry.Endpoint(), collisionDigestA, collisionDigestB)))

		By("deleting")
		result, err = runner.DeletePhaseTwo(ctx, deleteYaml(), testRegistry.Endpoint(), "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		expectManifestAbsentByDigest(*testRegistry, collisionRepo, collisionDigestA)
		expectManifestAbsentByDigest(*testRegistry, collisionRepo, collisionDigestB)
		expectEmptyRegistry(*testRegistry)
	})

	It("rejects a delete file mixing tag-only and digest-pinned selections of the same tag", func() {
		result, err := runner.DeletePhaseOne(ctx, collisionConfig("disc-additional-mixed-tag-digest.yaml"),
			workDir, "", testRegistry.Endpoint())
		expectOcMirrorExitCode(result, err, 1,
			"delete plan rejected",
			"quay.io/oc-mirror/oc-mirror-dev:shared-operand",
			"no delete file was written")

		_, statErr := os.Stat(deleteYaml())
		Expect(os.IsNotExist(statErr)).To(BeTrue(), "no delete file may be written for a rejected plan")
	})

	// Guards against deleting one manifest twice when a digest pinned for a shared tag is also
	// tagged elsewhere in the repository. The tags are set explicitly instead of relying on the
	// order of the concurrent pushes.
	DescribeTable("deletes a digest that is also tagged under another tag only once",
		func(extraArgs ...string) {
			mirrorToDiskToMirror(workDir, collisionConfig("isc-additional-tag-digest-alias.yaml"))
			tagByDigest(*testRegistry, collisionRepo, "shared-operand", collisionDigestB)
			tagByDigest(*testRegistry, collisionRepo, "alias-operand", collisionDigestA)
			expectTagResolvesToDigest(*testRegistry, collisionRepo, "shared-operand", collisionDigestB)
			expectTagResolvesToDigest(*testRegistry, collisionRepo, "alias-operand", collisionDigestA)

			By("generating the delete file")
			result, err := runner.DeletePhaseOne(ctx, collisionConfig("disc-additional-tag-digest-alias.yaml"),
				workDir, "", testRegistry.Endpoint())
			expectOcMirrorCommandSuccess(result, err)
			expected := append(collisionReferences(testRegistry.Endpoint(), collisionDigestA, collisionDigestB),
				fmt.Sprintf("docker://%s/%s:alias-operand", testRegistry.Endpoint(), collisionRepo))
			sort.Strings(expected)
			Expect(deletePlanReferences(deleteYaml())).To(Equal(expected))

			By("deleting")
			args := append([]string{"--dest-tls-verify=false"}, extraArgs...)
			result, err = runner.DeletePhaseTwo(ctx, deleteYaml(), testRegistry.Endpoint(), args...)
			expectOcMirrorCommandSuccess(result, err)

			expectManifestAbsentByDigest(*testRegistry, collisionRepo, collisionDigestA)
			expectManifestAbsentByDigest(*testRegistry, collisionRepo, collisionDigestB)
			expectEmptyRegistry(*testRegistry)
		},
		Entry("from the registry"),
		Entry("from the registry and the cache", "--force-cache-delete=true"),
	)
})

// collisionConfig returns the path of an image set configuration of the tag_digest_collision test data.
func collisionConfig(name string) string {
	return filepath.Join(iscDir, "tag_digest_collision", name)
}

// mirrorToDiskToMirror mirrors the ImageSetConfiguration to the local cache and then to the
// test registry.
func mirrorToDiskToMirror(workDir, isc string) {
	GinkgoHelper()
	result, err := runner.MirrorToDisk(ctx, isc, workDir, "--remove-signatures=true")
	expectOcMirrorCommandSuccess(result, err)
	result, err = runner.DiskToMirror(ctx, isc, workDir, testRegistry.Endpoint(), "--remove-signatures=true", "--dest-tls-verify=false")
	expectOcMirrorCommandSuccess(result, err)
}

// collisionReferences returns the sorted digest references of collisionRepo in the registry.
func collisionReferences(endpoint string, digests ...string) []string {
	refs := make([]string, 0, len(digests))
	for _, d := range digests {
		refs = append(refs, fmt.Sprintf("docker://%s/%s@%s", endpoint, collisionRepo, d))
	}
	sort.Strings(refs)
	return refs
}

// deletePlanReferences returns the sorted image references of a generated delete file.
func deletePlanReferences(path string) []string {
	GinkgoHelper()
	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	var plan struct {
		Items []struct {
			ImageReference string `yaml:"imageReference"`
		} `yaml:"items"`
	}
	Expect(yaml.Unmarshal(data, &plan)).To(Succeed())
	refs := make([]string, 0, len(plan.Items))
	for _, item := range plan.Items {
		refs = append(refs, item.ImageReference)
	}
	sort.Strings(refs)
	return refs
}

// expectManifestAbsentByDigest verifies that the manifest can no longer be resolved by digest.
func expectManifestAbsentByDigest(reg registry.Registry, repo, digest string) {
	GinkgoHelper()
	exists, err := reg.ManifestExistsByDigest(ctx, repo, digest)
	Expect(err).NotTo(HaveOccurred(), "error resolving manifest %s@%s", repo, digest)
	Expect(exists).To(BeFalse(), "manifest %s@%s still exists after delete", repo, digest)
}

// tagByDigest points tag at the manifest digest already present in the test registry.
func tagByDigest(reg registry.Registry, repo, tag, digest string) {
	GinkgoHelper()
	src, err := name.NewDigest(fmt.Sprintf("%s/%s@%s", reg.Endpoint(), repo, digest), name.Insecure)
	Expect(err).NotTo(HaveOccurred())
	desc, err := remote.Get(src, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))
	Expect(err).NotTo(HaveOccurred())
	dst, err := name.NewTag(fmt.Sprintf("%s/%s:%s", reg.Endpoint(), repo, tag), name.Insecure)
	Expect(err).NotTo(HaveOccurred())
	Expect(remote.Tag(dst, desc, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))).To(Succeed())
}

// untag removes a tag from the test registry, leaving the manifest in place.
func untag(reg registry.Registry, repo, tag string) {
	GinkgoHelper()
	ref, err := name.NewTag(fmt.Sprintf("%s/%s:%s", reg.Endpoint(), repo, tag), name.Insecure)
	Expect(err).NotTo(HaveOccurred())
	Expect(remote.Delete(ref, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))).To(Succeed())
}

// digestTag converts "sha256:<hex>" into the tag-safe "sha256-<hex>" form used by the cache.
func digestTag(digest string) string {
	return strings.Replace(digest, ":", "-", 1)
}
