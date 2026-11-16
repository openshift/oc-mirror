package integration_test

import (
	"maps"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

// blockedImages maps the image that isc-blocked-images.yaml blocks for each content type, as
// repository:tag at the destination when it is not blocked, to its source reference: the release
// component, an operator bundle, an additional image and a helm chart image. The additional and
// helm images are published on Quay.
func blockedImages() map[string]string {
	return map[string]string{
		"openshift/release:v0.0.1-alpine":           releaseComponentImage,
		testCatalogRepo + ":bar-bundle-v1.0.0":      bundleImage("bar-bundle-v1.0.0"),
		"oc-mirror/oc-mirror-dev:foo-bundle-v0.1.0": "quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.1.0",
		"oc-mirror/oc-mirror-dev:foo-bundle-v0.3.0": "quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.3.0",
	}
}

// allowedImages maps every image that isc-blocked-images.yaml still mirrors, as repository:tag at
// the destination, to its source reference. The operator catalog is rebuilt, so it has no source.
func allowedImages() map[string]string {
	return map[string]string{
		"openshift/release-images:v0.0.1":               testReleaseImage,
		testCatalogRepo + ":" + testCatalogTag:          "",
		testCatalogRepo + ":baz-bundle-v1.1.0":          bundleImage("baz-bundle-v1.1.0"),
		testCatalogRepo + ":" + digestTag(operandImage): operandImage,
		"oc-mirror/oc-mirror-dev:foo-bundle-v0.2.0":     "quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.2.0",
		"oc-mirror/oc-mirror-dev:foo-bundle-v0.3.2":     "quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.3.2",
	}
}

var _ = Describe("blocked images", func() {
	var workDir string
	var iscBlocked string

	BeforeEach(func() {
		workDir = setupWorkDir()
		data, err := os.ReadFile(filepath.Join(iscDir, "blocked_images", "isc-blocked-images.yaml"))
		Expect(err).NotTo(HaveOccurred())
		chartPath, err := filepath.Abs(filepath.Join(testdataDir, "helm-charts", "blocked-images"))
		Expect(err).NotTo(HaveOccurred())
		var config map[string]any
		Expect(yaml.Unmarshal(data, &config)).To(Succeed())
		helm := config["mirror"].(map[string]any)["helm"].(map[string]any)
		charts := helm["local"].([]any)
		Expect(charts).To(HaveLen(1))
		charts[0].(map[string]any)["path"] = chartPath
		data, err = yaml.Marshal(config)
		Expect(err).NotTo(HaveOccurred())
		iscBlocked = filepath.Join(workDir, "isc-blocked-images.yaml")
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
		for destination, source := range allowedImages() {
			if source != "" {
				mappings = append(mappings, "docker://"+source+"=docker://"+localCacheHost+"/"+destination)
			}
		}
		mappings = append(mappings, "docker://"+fixtureByDigest(happyPathCatalog)+
			"=docker://"+localCacheHost+"/"+testCatalogRepo+":"+testCatalogTag)
		expectDryRunFile(workDir, mappingFileName, mappings)

		By("running mirrorToDisk")
		result, err = runner.MirrorToDisk(ctx, iscBlocked, workDir, "--remove-signatures=true")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying the archive contains the allowed images only")
		cachedImages := allowedImages()
		cachedImages[testCatalogRepo+":"+testCatalogTag] = happyPathCatalog
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
	expectRegistryContainsExactly(reg, allowedImages())

	for destination, source := range blockedImages() {
		repo, _, _ := strings.Cut(destination, ":")
		exists, err := reg.ManifestExistsByDigest(ctx, repo, remoteDigest(source))
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeFalse(), "blocked image %s was mirrored to %s", source, repo)
	}

	// The operand is the only image referenced by digest, and the additional and helm images, on
	// Quay, are referenced by tag.
	idms := map[string]string{
		referenceWithoutTagOrDigest(testReleaseImage): "openshift/release-images",
		originRegistry.Endpoint() + "/oc-mirror":      "oc-mirror",
	}
	itms := maps.Clone(idms)
	itms["quay.io/oc-mirror"] = "oc-mirror"
	expectMirrorSets(workDir, reg.Endpoint(), idms, itms)
}
