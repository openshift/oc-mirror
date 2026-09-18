package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

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

	// CLID-717: the related images of a bundle may carry labels, typically saying which
	// product feature each image belongs to. The `selectors` of a package pick the labelled
	// images to mirror; images without labels are always mirrored.
	Describe("related image selection by label", func() {
		// Tags of the test-catalog-labels catalog: the bundle and the operator image of
		// foo.v0.3.1 carry no label, each operand carries a different feature label.
		const (
			testImagesRepo = "oc-mirror/oc-mirror-dev"

			bundleTag    = "foo-bundle-v0.3.1"
			operatorTag  = "foo-v0.3.1"
			analyticsTag = "bar-v1.0.0"
			loggingTag   = "baz-v1.0.0"
			metricsTag   = "baz-v1.1.0"
		)

		It("should mirror the unlabelled images and the ones matched by matchLabels", func() {
			iscFile := filepath.Join("operators", "isc-operator-selectors-match-labels.yaml")

			By("running mirrorToMirror with a selector on feature=analytics")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the operator catalog is mirrored in the registry")
			expectSuccessfulMirrorInRegistry(filepath.Join(iscDir, iscFile), *testRegistry)

			By("verifying only the unlabelled images and the analytics operand are mirrored")
			expectMirroredTags(*testRegistry, testImagesRepo,
				[]string{bundleTag, operatorTag, analyticsTag},
				[]string{loggingTag, metricsTag})
		})

		It("should mirror the images matched by any of the matchExpressions selectors", func() {
			iscFile := filepath.Join("operators", "isc-operator-selectors-match-expressions.yaml")

			By("running mirrorToMirror with a selector on feature in (analytics, metrics)")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the analytics and metrics operands are mirrored, the logging one is not")
			expectMirroredTags(*testRegistry, testImagesRepo,
				[]string{bundleTag, operatorTag, analyticsTag, metricsTag},
				[]string{loggingTag})
		})

		It("should mirror only the unlabelled images when the package has no selector", func() {
			iscFile := filepath.Join("operators", "isc-operator-no-selectors.yaml")

			By("running mirrorToMirror without any selector")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying no labelled operand is mirrored")
			expectMirroredTags(*testRegistry, testImagesRepo,
				[]string{bundleTag, operatorTag},
				[]string{analyticsTag, loggingTag, metricsTag})
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
})

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

// expectMirroredTags verifies which tags of a repository reached the destination registry:
// every tag of present must be there, none of the tags of absent may be.
func expectMirroredTags(reg registry.Registry, repo string, present, absent []string) {
	GinkgoHelper()

	tags, err := reg.ListTags(ctx, repo)
	Expect(err).NotTo(HaveOccurred(), "failed to list the tags of repository %q", repo)

	for _, tag := range present {
		Expect(tags).To(ContainElement(tag), "image %s:%s should have been mirrored, got tags: %v", repo, tag, tags)
	}
	for _, tag := range absent {
		Expect(tags).NotTo(ContainElement(tag), "image %s:%s should have been left out, got tags: %v", repo, tag, tags)
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
