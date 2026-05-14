package integration_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

	Describe("using a mirrored local catalog as source", func() {
		remoteISCFile := filepath.Join("operators", "isc-operator-remote-catalog.yaml")
		localISCFile := filepath.Join("operators", "isc-operator-local-catalog.yaml")

		var archiveDir string
		var mirrorToMirrorCacheDir string
		var mirrorToDiskCacheDir string

		BeforeEach(func() {
			var err error
			archiveDir = setupWorkDir()

			mirrorToMirrorCacheDir, err = os.MkdirTemp("", "oc-mirror-m2m-cache-*")
			Expect(err).NotTo(HaveOccurred())

			mirrorToDiskCacheDir, err = os.MkdirTemp("", "oc-mirror-m2d-cache-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			cleanupWorkDir(archiveDir)
			cleanupWorkDir(mirrorToMirrorCacheDir)
			cleanupWorkDir(mirrorToDiskCacheDir)
		})

		It("should mirror from the generated local catalog reference to disk", func() {
			remoteISCPath := filepath.Join(iscDir, remoteISCFile)
			localISCPath := filepath.Join(iscDir, localISCFile)

			By("running mirrorToMirror for the remote operator catalog")
			result, err := runner.MirrorToMirror(ctx, remoteISCPath, workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false", "--cache-dir", mirrorToMirrorCacheDir)
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the operator catalog is mirrored in the local registry")
			expectSuccessfulMirrorInRegistry(remoteISCPath, *testRegistry)

			By("verifying IDMS contains the mirrored operator content")
			expectCorrectIDMS(workDir, remoteISCPath)

			By("verifying the generated ClusterCatalog matches the checked-in local ISC")
			localISC := parseImageSetConfig(localISCPath)
			Expect(localISC.Mirror.Operators).To(HaveLen(1))
			expectClusterCatalogSource(workDir, localISC.Mirror.Operators[0].Catalog)

			By("running mirrorToDisk from the local registry catalog")
			result, err = runner.MirrorToDisk(ctx, localISCPath, archiveDir,
				"--remove-signatures=true", "--src-tls-verify=false", "--cache-dir", mirrorToDiskCacheDir)
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the operator catalog content was cached locally")
			expectSuccessfulMirrorInLocalCache(localISCPath, filepath.Join(mirrorToDiskCacheDir, ".oc-mirror", ".cache"))

			By("verifying a tar archive was created for the mirrored content")
			expectCorrectTarArchiveContents(localISCPath, archiveDir)
		})
	})
})
