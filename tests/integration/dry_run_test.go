package integration_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// dockerTransportPrefix is the "docker://" transport prefix used in mapping.txt entries.
const (
	dockerTransportPrefix = "docker://"
	localCacheHost        = "127.0.0.1:55000"
	mappingFileName       = "mapping.txt"
	missingFileName       = "missing.txt"
)

var _ = Describe("dry-run", func() {
	var workDir string
	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	Describe("mirrorToMirror dry-run", func() {
		iscHappyPath := filepath.Join("happy_path", "isc-happy-path.yaml")

		It("should generate mapping file without mirroring images to the registry", func() {
			By("running mirrorToMirror with --dry-run")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscHappyPath), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false", "--dry-run")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying mapping.txt contains valid mappings for all expected repositories")
			expectValidMappingFile(workDir, filepath.Join(iscDir, iscHappyPath))

			By("verifying no images were mirrored to the registry")
			expectNoRepositoriesInRegistry(*testRegistry)
		})
	})

	Describe("mirrorToDisk + diskToMirror dry-run", func() {
		iscHappyPath := filepath.Join("happy_path", "isc-happy-path.yaml")

		It("should generate mapping files without creating a tar archive or mirroring images to the registry", func() {
			By("running mirrorToDisk with --dry-run")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscHappyPath), workDir,
				"--remove-signatures=true", "--dry-run")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying mapping.txt contains valid mappings for all expected repositories")
			expectValidMappingFile(workDir, filepath.Join(iscDir, iscHappyPath))

			By("verifying no tar archive was created")
			expectNoTarArchive(workDir)

			By("running mirrorToDisk without --dry-run to prepare the archive")
			result, err = runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscHappyPath), workDir,
				"--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("running diskToMirror with --dry-run")
			result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscHappyPath), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false", "--dry-run")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying mapping.txt contains valid mappings for all expected repositories")
			expectValidMappingFile(workDir, filepath.Join(iscDir, iscHappyPath))

			By("verifying no images were mirrored to the registry")
			expectNoRepositoriesInRegistry(*testRegistry)
		})
	})

	Describe("mirrorToDisk dry-run missing images report", func() {
		initialImage := "docker://quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.1.0=docker://" + localCacheHost + "/oc-mirror/oc-mirror-dev:foo-bundle-v0.1.0"
		addedImage := "docker://quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.2.0=docker://" + localCacheHost + "/oc-mirror/oc-mirror-dev:foo-bundle-v0.2.0"

		It("should list in missing.txt only the images that are not in the local cache", func() {
			iscInitial := filepath.Join(iscDir, "dry_run", "isc-missing-images-initial.yaml")
			iscFull := filepath.Join(iscDir, "dry_run", "isc-missing-images-full.yaml")

			By("running a dry-run with an empty cache")
			result, err := runner.MirrorToDisk(ctx, iscFull, workDir, "--dry-run")
			expectOcMirrorCommandSuccess(result, err)
			expectDryRunFile(workDir, mappingFileName, []string{initialImage, addedImage})
			expectDryRunFile(workDir, missingFileName, []string{initialImage, addedImage})

			By("mirroring only the first image to the cache")
			result, err = runner.MirrorToDisk(ctx, iscInitial, workDir)
			expectOcMirrorCommandSuccess(result, err)

			By("running a dry-run with a partially populated cache")
			result, err = runner.MirrorToDisk(ctx, iscFull, workDir, "--dry-run")
			expectOcMirrorCommandSuccess(result, err)
			expectDryRunFile(workDir, mappingFileName, []string{initialImage, addedImage})
			expectDryRunFile(workDir, missingFileName, []string{addedImage})

			By("mirroring all images to the cache")
			result, err = runner.MirrorToDisk(ctx, iscFull, workDir)
			expectOcMirrorCommandSuccess(result, err)

			By("running a dry-run with a fully populated cache")
			result, err = runner.MirrorToDisk(ctx, iscFull, workDir, "--dry-run")
			expectOcMirrorCommandSuccess(result, err)
			expectDryRunFile(workDir, mappingFileName, []string{initialImage, addedImage})
			_, err = os.Stat(filepath.Join(workDir, dirWorkingDir, "dry-run", missingFileName))
			Expect(os.IsNotExist(err)).To(BeTrue(), "missing.txt should not exist when every image is in the cache")
			Expect(result.Stdout + result.Stderr).To(ContainSubstring("all 2 images required for mirroring are available in local cache"))
		})
	})

	Describe("dry-run with a manifest list image", func() {
		iscManifestList := filepath.Join("dry_run", "isc-manifest-list.yaml")
		topLevelSource := "docker://quay.io/openshifttest/hello-openshift@sha256:61b8f5e1a3b5dbd9e2c35fd448dc5106337d7a299873dd3a6f0cd8d4891ecc27"

		// The digest above resolves to a manifest list (image index) with these two
		// platform-specific sub-manifests, which never change since it's pinned by digest.
		manifestListSubDigests := []string{
			"sha256:685a0ca5f33d9f921966c9d9f5922e266affbf93dde0c156709ecdea362f88f4",
			"sha256:a51d6da571b2e1f57249f4d966af65cbfb361dad66cde7121c1bb656ab196269",
		}

		It("should include manifest list sub-digests in mapping.txt (OCPBUGS-66263)", func() {
			By("running mirrorToDisk with --dry-run-manifest-lists")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscManifestList), workDir,
				"--remove-signatures=true", "--dry-run-manifest-lists")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying mapping.txt contains an entry for each manifest list sub-digest")
			expectMappingContainsManifestListSubDigests(workDir, topLevelSource, manifestListSubDigests)
		})

		It("should include manifest list sub-digests in mapping.txt in mirrorToMirror (OCPBUGS-66263)", func() {
			By("running mirrorToMirror with --dry-run-manifest-lists")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscManifestList), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false", "--dry-run-manifest-lists")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying mapping.txt contains an entry for each manifest list sub-digest")
			expectMappingContainsManifestListSubDigests(workDir, topLevelSource, manifestListSubDigests)
			expectNoRepositoriesInRegistry(*testRegistry)
		})
	})
})

// expectDryRunFile checks the exact source=destination lines in a dry-run report.
func expectDryRunFile(workDir, fileName string, expected []string) {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(workDir, dirWorkingDir, "dry-run", fileName))
	Expect(err).NotTo(HaveOccurred(), "%s not found", fileName)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	Expect(lines).To(ConsistOf(expected), "unexpected lines in %s", fileName)
}

// expectMappingContainsManifestListSubDigests verifies that mapping.txt contains one extra
// line per manifest list sub-digest, each re-pinned to the top-level destination's repository.
// Guards against OCPBUGS-66263, where sub-manifest digests were dropped from the mapping file.
func expectMappingContainsManifestListSubDigests(workDir, topLevelSource string, subDigests []string) {
	mappingPath := filepath.Join(workDir, dirWorkingDir, "dry-run", "mapping.txt")
	data, err := os.ReadFile(mappingPath)
	Expect(err).NotTo(HaveOccurred(), "mapping.txt not found at: %s", mappingPath)

	mappings := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.SplitN(line, "=", 2)
		Expect(parts).To(HaveLen(2), "mapping line does not follow source=destination format: %s", line)
		mappings[parts[0]] = parts[1]
	}

	destination, found := mappings[topLevelSource]
	Expect(found).To(BeTrue(), "mapping.txt does not contain top-level entry for %q", topLevelSource)

	sourceBase, _, _ := strings.Cut(topLevelSource, "@")
	// referenceWithoutTagOrDigest expects a bare image reference, so the "docker://" transport
	// prefix (present on mapping.txt destinations) must be stripped before parsing and restored
	// afterwards to rebuild a destination in the same format written by oc-mirror.
	destBase := dockerTransportPrefix + referenceWithoutTagOrDigest(strings.TrimPrefix(destination, dockerTransportPrefix))
	for _, digest := range subDigests {
		subSource := sourceBase + "@" + digest
		subDest := destBase + "@" + digest
		Expect(mappings).To(HaveKeyWithValue(subSource, subDest),
			"mapping.txt is missing sub-digest entry %q -> %q", subSource, subDest)
	}
}
