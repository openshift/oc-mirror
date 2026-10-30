package integration_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

// blockedImages maps the image that isc-blocked-images.yaml blocks for each content type, as
// repository:tag at the destination when it is not blocked, to its source reference.
var blockedImages = map[string]string{
	"openshift/release:v0.0.1-alpine":           "quay.io/oc-mirror/release/test-image:v0.0.1",
	"oc-mirror/oc-mirror-dev:bar-bundle-v1.0.0": "quay.io/oc-mirror/oc-mirror-dev:bar-bundle-v1.0.0",
	"oc-mirror/oc-mirror-dev:foo-bundle-v0.1.0": "quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.1.0",
	"oc-mirror/oc-mirror-dev:foo-bundle-v0.3.0": "quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.3.0",
}

// allowedImages maps every image that isc-blocked-images.yaml still mirrors, as repository:tag at
// the destination, to its source reference. The operator catalog is rebuilt, so it has no source.
var allowedImages = map[string]string{
	"openshift/release-images:v0.0.1":                                                                 "quay.io/oc-mirror/release/test-release-index:v0.0.1",
	"oc-mirror/oc-mirror-dev:test-catalog-latest":                                                     "",
	"oc-mirror/oc-mirror-dev:baz-bundle-v1.1.0":                                                       "quay.io/oc-mirror/oc-mirror-dev:baz-bundle-v1.1.0",
	"oc-mirror/oc-mirror-dev:sha256-1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58": "quay.io/oc-mirror/oc-mirror-dev@sha256:1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58",
	"oc-mirror/oc-mirror-dev:foo-bundle-v0.2.0":                                                       "quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.2.0",
	"oc-mirror/oc-mirror-dev:foo-bundle-v0.3.2":                                                       "quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.3.2",
}

var _ = Describe("blocked images", func() {
	var workDir string
	var iscBlocked string

	BeforeEach(func() {
		workDir = setupWorkDir()
		data, err := os.ReadFile(filepath.Join(iscDir, "blocked_images", "isc-blocked-images.yaml"))
		Expect(err).NotTo(HaveOccurred())
		chartPath, err := filepath.Abs(filepath.Join(filepath.Dir(iscDir), "helm-charts", "blocked-images"))
		Expect(err).NotTo(HaveOccurred())
		iscBlocked = filepath.Join(workDir, "isc-blocked-images.yaml")
		data = []byte(strings.Replace(string(data), "path: testdata/helm-charts/blocked-images", "path: "+chartPath, 1))
		Expect(os.WriteFile(iscBlocked, data, 0o644)).To(Succeed())
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	It("should exclude blocked images from the dry-run mapping, the archive and the registry in mirrorToDisk + diskToMirror", func() {
		By("running mirrorToDisk with --dry-run")
		result, err := runner.MirrorToDisk(ctx, iscBlocked, workDir, "--remove-signatures=true", "--dry-run")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying mapping.txt lists only the allowed images")
		var mappings []string
		for destination, source := range allowedImages {
			if source != "" {
				mappings = append(mappings, "docker://"+source+"=docker://"+localCacheHost+"/"+destination)
			}
		}
		mappings = append(mappings, "docker://quay.io/"+testCatalogRepo+"@"+remoteDigest(happyPathCatalog)+
			"=docker://"+localCacheHost+"/"+testCatalogRepo+":test-catalog-latest")
		expectDryRunFile(workDir, mappingFileName, mappings)

		By("running mirrorToDisk")
		result, err = runner.MirrorToDisk(ctx, iscBlocked, workDir, "--remove-signatures=true")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying the archive contains the allowed images only")
		cachedImages := map[string]string{}
		for destination, source := range allowedImages {
			cachedImages[destination] = source
		}
		cachedImages[testCatalogRepo+":test-catalog-latest"] = happyPathCatalog
		expectArchiveContainsExactly(workDir, cachedImages, testCatalogRepo)

		By("running diskToMirror")
		result, err = runner.DiskToMirror(ctx, iscBlocked, workDir, testRegistry.Endpoint(),
			"--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying only the allowed images were mirrored")
		expectOnlyAllowedImagesMirrored(*testRegistry, workDir)
	})

	It("should exclude blocked images while mirroring allowed images in mirrorToMirror", func() {
		By("running mirrorToMirror with both allowed and blocked images")
		result, err := runner.MirrorToMirror(ctx, iscBlocked, workDir, testRegistry.Endpoint(),
			"--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying only the allowed images were mirrored")
		expectOnlyAllowedImagesMirrored(*testRegistry, workDir)
	})

	It("should fail without mirroring anything when every image is blocked", func() {
		By("running mirrorToMirror with a pattern that blocks every image")
		result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, "blocked_images", "isc-block-all.yaml"), workDir, testRegistry.Endpoint(),
			"--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorExitCode(result, err, 1, "No images to mirror", "no images to copy")

		By("verifying nothing was mirrored and no cluster resources were generated")
		expectNoRepositoriesInRegistry(*testRegistry)
		expectNoClusterResources(workDir)
	})
})

// expectOnlyAllowedImagesMirrored verifies that the registry holds exactly the allowed images,
// that no blocked image manifest was pushed, and that the generated IDMS and ITMS no longer
// reference the blocked release component.
func expectOnlyAllowedImagesMirrored(reg registry.Registry, workDir string) {
	GinkgoHelper()
	expectRegistryContainsExactly(reg, allowedImages)

	for destination, source := range blockedImages {
		repo, _, _ := strings.Cut(destination, ":")
		exists, err := reg.ManifestExistsByDigest(ctx, repo, remoteDigest(source))
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeFalse(), "blocked image %s was mirrored to %s", source, repo)
	}

	mirrors := map[string]string{
		"quay.io/oc-mirror/release/test-release-index": "openshift/release-images",
		"quay.io/oc-mirror":                            "oc-mirror",
	}
	expectMirrorSets(workDir, reg.Endpoint(), mirrors, mirrors)
}

// expectRegistryContainsExactly is used by the new blocked-image and nested-path cases.
func expectRegistryContainsExactly(reg registry.Registry, expected map[string]string) {
	GinkgoHelper()
	repos, err := reg.ListRepositories(ctx)
	Expect(err).NotTo(HaveOccurred())
	actual := map[string]string{}
	for _, repo := range repos {
		tags, err := reg.ListTags(ctx, repo)
		Expect(err).NotTo(HaveOccurred())
		for _, tag := range tags {
			digest, err := reg.TagDigest(ctx, repo, tag)
			Expect(err).NotTo(HaveOccurred())
			actual[repo+":"+tag] = digest
		}
	}
	expectImagesExactly("registry", actual, expected)
}
