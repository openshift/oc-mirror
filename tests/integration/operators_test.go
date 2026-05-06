package integration_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
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

		It("should mirror only the pinned operator bundle version", func() {
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
})
