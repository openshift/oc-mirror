package integration_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

	Describe("mirrorToDisk dry-run with a manifest list image", func() {
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
	})
})

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
	destBase := stripTagOrDigest(destination)
	for _, digest := range subDigests {
		subSource := sourceBase + "@" + digest
		subDest := destBase + "@" + digest
		Expect(mappings).To(HaveKeyWithValue(subSource, subDest),
			"mapping.txt is missing sub-digest entry %q -> %q", subSource, subDest)
	}
}

// stripTagOrDigest removes a trailing ":tag" or "@digest" from an image reference. Tag
// separators are only looked for after the last "/", so a registry port (e.g. "localhost:55000")
// isn't mistaken for one.
func stripTagOrDigest(ref string) string {
	if base, _, found := strings.Cut(ref, "@"); found {
		return base
	}
	if slash := strings.LastIndex(ref, "/"); slash != -1 {
		if colon := strings.Index(ref[slash:], ":"); colon != -1 {
			return ref[:slash+colon]
		}
	}
	return ref
}
