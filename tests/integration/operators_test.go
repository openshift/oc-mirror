package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"gopkg.in/yaml.v3"

	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

var _ = Describe("operators", func() {
	var workDir string
	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	Describe("operator with version range (minVersion != maxVersion)", func() {
		iscFile := filepath.Join("operators", "isc-operator-version-range.yaml")

		It("should mirror only operator bundle versions within the range", func() {
			By("running mirrorToMirror with a version range")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the operator catalog is mirrored in the registry")
			expectSuccessfulMirrorInRegistry(filepath.Join(iscDir, iscFile), *testRegistry)

			By("verifying the catalog contains exactly the bundles within the version range")
			expectCatalogBundlesMatchISC(ctx, *testRegistry, filepath.Join(iscDir, iscFile),
				map[string][]string{
					"foo": {"foo.v0.2.0", "foo.v0.3.0", "foo.v0.3.1"},
				})
		})
	})

	Describe("operator with pinned version (minVersion == maxVersion)", func() {
		iscFile := filepath.Join("operators", "isc-operator-pinned-version.yaml")

		It("should mirror only the pinned operator bundle version and generate correct cluster resources", func() {
			By("running mirrorToMirror with a pinned operator version")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the operator catalog is mirrored in the registry")
			expectSuccessfulMirrorInRegistry(filepath.Join(iscDir, iscFile), *testRegistry)

			By("verifying the catalog contains only the pinned bundle version")
			expectCatalogBundlesMatchISC(ctx, *testRegistry, filepath.Join(iscDir, iscFile),
				map[string][]string{
					"foo": {"foo.v0.3.1"},
				})

			By("verifying the rebuilt catalog excludes packages that were not requested")
			expectCatalogContainsOnlyExpectedPackages(ctx, *testRegistry, filepath.Join(iscDir, iscFile))

			By("verifying IDMS contains all the expected images and no empty fields")
			expectCorrectIDMS(workDir, filepath.Join(iscDir, iscFile))

			By("verifying CatalogSource YAML references the rebuilt catalog in the target registry")
			expectCorrectCatalogSourceYAMLs(workDir, testRegistry.Endpoint(), filepath.Join(iscDir, iscFile))
		})
	})

	Describe("operator with invalid version range (minVersion > maxVersion)", func() {
		iscFile := filepath.Join("operators", "isc-operator-invalid-version-range.yaml")

		It("should fail when minVersion is greater than maxVersion", func() {
			By("running mirrorToMirror with an invalid version range")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandFailure(result, err)

			By("verifying no content was mirrored to the registry")
			expectNoRepositoriesInRegistry(*testRegistry)
		})
	})

	Describe("file permissions after catalog rebuild", func() {
		iscFile := filepath.Join("operators", "isc-operator-version-range.yaml")

		It("should only set executable permissions on graph-preparation and filtered-catalog-image files", func() {
			By("running mirrorToDisk to trigger catalog rebuild")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscFile), workDir, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying only allowed files have executable permissions")
			expectOnlyAllowedExecutableFiles(workDir)
		})
	})

	Describe("catalog digest rebuild tagging", func() {
		iscFile := filepath.Join("operators", "isc-operator-catalog-digest.yaml")

		It("should tag rebuilt catalog images with a value matching the manifest digest", func() {
			By("running mirrorToMirror with digest-pinned catalog references")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying mirrored content exists in the registry")
			expectSuccessfulMirrorInRegistry(filepath.Join(iscDir, iscFile), *testRegistry)

			By("verifying the rebuilt catalog tag matches the fetched manifest digest")
			expectRebuiltTagMatchesDigest(ctx, *testRegistry, filepath.Join(iscDir, iscFile))
		})
	})

	// OCPBUGS-33081: a catalog may contain bundles with invalid related images (missing
	// name, missing tag/digest, unsupported oci:// scheme, ...).
	Describe("catalog with a bundle containing an invalid related image", func() {
		iscFile := filepath.Join("operators", "isc-operator-invalid-images.yaml")

		It("handles invalid related image references in a catalog", func() {
			By("running mirrorToMirror against a catalog with one valid and one invalid bundle")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--dest-tls-verify=false")
			expectOcMirrorExitCode(result, err, 4, "collection error", "tag and digest are empty")

			By("verifying no content was mirrored, even though one of the two bundles was valid")
			expectNoRepositoriesInRegistry(*testRegistry)
		})
	})

	// OCPBUGS-132865: a bundle's image (olm.bundle.image) must be mirrored even when the
	// catalog does not repeat it in the bundle's relatedImages. The catalog is built from
	// testdata/catalogs/primary-bundle-image and served by a separate source registry, so
	// testRegistry only receives what oc-mirror mirrors. That source registry serves plain
	// HTTP, hence --src-tls-verify=false (as in enclave_test.go); the flag also applies to
	// the bundle and related images pulled from quay.io.
	Describe("catalog with bundle images missing from their related images", func() {
		const (
			fixtureRepo    = "fixtures/primary-bundle-catalog"
			fixtureTag     = "v1"
			barBundleImage = "quay.io/" + testCatalogRepo + ":bar-bundle-v0.1.0"
			bazBundleImage = "quay.io/" + testCatalogRepo + ":baz-bundle-v1.0.0"
			fooBundleImage = "quay.io/" + testCatalogRepo + ":foo-bundle-v0.3.1"
		)
		var catalog catalogFixture

		BeforeEach(func() {
			By("serving the catalog fixture from a separate source registry")
			catalog = seedCatalogFixture(filepath.Join(catalogsDir, "primary-bundle-image"), fixtureRepo, fixtureTag)
		})

		DescribeTable("mirrorToMirror mirrors the bundle image and its related images",
			func(pkg, bundle string, images []string) {
				isc := writeCatalogFixtureISC(catalog.Ref, pkg)

				By("running mirrorToMirror for package " + pkg)
				result, err := runner.MirrorToMirror(ctx, isc, workDir, testRegistry.Endpoint(),
					"--remove-signatures=true", "--dest-tls-verify=false", "--src-tls-verify=false")
				expectOcMirrorCommandSuccess(result, err)

				By("verifying the registry holds exactly the catalog, the bundle image and its related images")
				expectRegistryContainsExactly(*testRegistry, expectedFixtureMirror(catalog, images...))
				expectFixtureImagesReadable(images...)

				By("verifying the mirrored catalog contains the selected bundle")
				expectCatalogBundlesMatchISC(ctx, *testRegistry, isc, map[string][]string{pkg: {bundle}})
			},
			Entry("bundle image omitted from a non-empty related images list",
				"bar", "bar.v0.1.0", []string{barBundleImage, operandImage}),
			Entry("empty related images list",
				"baz", "baz.v1.0.0", []string{bazBundleImage}),
			Entry("bundle image also listed in related images (control)",
				"foo", "foo.v0.3.1", []string{fooBundleImage, operandImage}),
		)

		It("mirrorToDisk and diskToMirror mirror a bundle image omitted from related images", func() {
			images := []string{barBundleImage, operandImage}
			isc := writeCatalogFixtureISC(catalog.Ref, "bar")

			By("running mirrorToDisk")
			result, err := runner.MirrorToDisk(ctx, isc, workDir, "--remove-signatures=true", "--src-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the local cache and the archive hold the bundle image and its related image")
			cached := mirroredImages(images...)
			cached[catalog.Repo+":"+catalog.Tag] = catalog.DigestRef
			expectLocalCacheContainsExactly(cacheDir, cached, catalog.Repo)
			expectArchiveContainsExactly(workDir, cached, catalog.Repo)
			for _, img := range images {
				digest := remoteDigest(img)
				ref, err := name.NewDigest(localCacheHost + "/" + testCatalogRepo + "@" + digest)
				Expect(err).NotTo(HaveOccurred())
				expectImageInLocalCache(cacheDir, ref)
				expectTarContainsBlobForDigests(workDir, digest)
			}

			By("removing the local cache so diskToMirror can only use the archive")
			Expect(os.RemoveAll(cacheDir)).To(Succeed())

			By("running diskToMirror")
			result, err = runner.DiskToMirror(ctx, isc, workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the registry holds exactly the catalog, the bundle image and its related image")
			expectRegistryContainsExactly(*testRegistry, expectedFixtureMirror(catalog, images...))
			expectFixtureImagesReadable(images...)
			expectCatalogBundlesMatchISC(ctx, *testRegistry, isc, map[string][]string{"bar": {"bar.v0.1.0"}})
		})
	})
})

// expectedFixtureMirror returns the registry content expected after mirroring the given
// source images and the catalog fixture. The catalog is rebuilt, so its digest is not checked.
func expectedFixtureMirror(catalog catalogFixture, images ...string) map[string]string {
	expected := mirroredImages(images...)
	expected[catalog.Repo+":"+catalog.Tag] = ""
	return expected
}

// mirroredImages maps the repository:tag oc-mirror uses for each source image to that image.
func mirroredImages(images ...string) map[string]string {
	GinkgoHelper()
	mirrored := make(map[string]string, len(images))
	for _, img := range images {
		mirrored[mirroredRepoTag(img)] = img
	}
	return mirrored
}

// expectFixtureImagesReadable verifies that every given source image can be fully read
// from testRegistry by its source digest.
func expectFixtureImagesReadable(images ...string) {
	GinkgoHelper()
	for _, img := range images {
		ref, err := name.ParseReference(img)
		Expect(err).NotTo(HaveOccurred())
		expectImageReadableByDigest(ctx, *testRegistry, ref.Context().RepositoryStr(), remoteDigest(img))
	}
}

// mirroredRepoTag returns the repository:tag oc-mirror uses for a source image: the
// source tag, or sha256-<digest> for images referenced by digest.
func mirroredRepoTag(img string) string {
	GinkgoHelper()
	ref, err := name.ParseReference(img)
	Expect(err).NotTo(HaveOccurred())
	repo := ref.Context().RepositoryStr()
	if digest, ok := ref.(name.Digest); ok {
		return repo + ":" + strings.Replace(digest.DigestStr(), ":", "-", 1)
	}
	return repo + ":" + ref.Identifier()
}

// expectCatalogContainsOnlyExpectedPackages verifies that the rebuilt catalog in the registry
// contains configs for exactly the packages selected in the ISC, and no others. This guards
// against a regression where oc-mirror copies the source catalog unfiltered instead of rebuilding
// a filtered index (OCPBUGS-56718).
func expectCatalogContainsOnlyExpectedPackages(ctx context.Context, reg registry.Registry, iscPath string) {
	cfg := parseImageSetConfig(iscPath)

	for _, op := range cfg.Mirror.Operators {
		configsDir := extractCatalogConfigs(ctx, reg, op.Catalog)
		DeferCleanup(func() {
			Expect(os.RemoveAll(configsDir)).To(Succeed(), "failed to clean up catalog configs dir %s", configsDir)
		})

		entries, err := os.ReadDir(configsDir)
		Expect(err).NotTo(HaveOccurred(), "failed to read catalog configs dir %s", configsDir)

		var actualPackages []string
		for _, entry := range entries {
			if entry.IsDir() {
				actualPackages = append(actualPackages, entry.Name())
			}
		}

		var expectedPackages []string
		for _, pkg := range op.Packages {
			expectedPackages = append(expectedPackages, pkg.Name)
		}

		Expect(actualPackages).To(ConsistOf(expectedPackages),
			"rebuilt catalog for %q should contain only the packages selected in the ISC", op.Catalog)
	}
}

// expectCorrectCatalogSourceYAMLs reads the CatalogSource YAML files generated by oc-mirror and
// verifies that every operator catalog in the ISC has a corresponding CatalogSource whose
// spec.image references the rebuilt catalog in the target registry (not the source registry).
func expectCorrectCatalogSourceYAMLs(workDir, targetRegistry, iscPath string) {
	pattern := filepath.Join(workDir, dirWorkingDir, dirClusterResources, "cs-*.yaml")
	matches, err := filepath.Glob(pattern)
	Expect(err).NotTo(HaveOccurred(), "failed to list CatalogSource files")
	Expect(matches).NotTo(BeEmpty(), "no CatalogSource YAML files found matching %s", pattern)

	var images []string
	for _, csFile := range matches {
		data, err := os.ReadFile(csFile)
		Expect(err).NotTo(HaveOccurred(), "failed to read CatalogSource file: %s", csFile)

		var raw map[string]any
		Expect(yaml.Unmarshal(data, &raw)).To(Succeed(), "failed to parse CatalogSource document: %s", csFile)

		jsonBytes, err := json.Marshal(raw)
		Expect(err).NotTo(HaveOccurred(), "failed to marshal CatalogSource document to JSON: %s", csFile)

		var cs operatorv1alpha1.CatalogSource
		Expect(json.Unmarshal(jsonBytes, &cs)).To(Succeed(), "failed to unmarshal CatalogSource: %s", csFile)

		Expect(cs.Kind).To(Equal("CatalogSource"), "unexpected kind in %s", csFile)
		Expect(cs.Spec.Image).NotTo(BeEmpty(), "CatalogSource %s has empty spec.image", csFile)

		images = append(images, cs.Spec.Image)
	}

	cfg := parseImageSetConfig(iscPath)
	for _, op := range cfg.Mirror.Operators {
		expectedRepo := targetRegistry + "/" + extractRepositoryName(op.Catalog)
		Expect(catalogSourceImagesCoverRepo(images, expectedRepo)).To(BeTrue(),
			"no CatalogSource image references the rebuilt catalog %q; images: %v", expectedRepo, images)
	}
}

// catalogSourceImagesCoverRepo returns true if any image reference points at the given
// repository, tagged or by digest.
func catalogSourceImagesCoverRepo(images []string, repo string) bool {
	for _, image := range images {
		if strings.HasPrefix(image, repo+":") || strings.HasPrefix(image, repo+"@") {
			return true
		}
	}
	return false
}
