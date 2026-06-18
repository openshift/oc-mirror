// incremental_mirroring_test.go validates incremental mirroring workflows.
// It verifies that oc-mirror produces archives containing only new content on
// subsequent runs, whether filtered by --since date or by config changes.
package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// findTarArchives returns the list of tar archives in workDir matching mirror_*.tar.
func findTarArchives(workDir string) []string {
	matches, err := filepath.Glob(filepath.Join(workDir, "mirror_*.tar"))
	Expect(err).NotTo(HaveOccurred())
	Expect(matches).NotTo(BeEmpty(), "no tar archive found in %s", workDir)
	return matches
}

// countBlobEntriesInArchives counts the number of blob entries (under docker/registry/v2/blobs/)
// in the given tar archives. This represents actual image layer data stored in the archive.
func countBlobEntriesInArchives(tarFiles []string) int {
	blobCount := 0
	for _, tarFile := range tarFiles {
		entries := listTarEntries(tarFile)
		for _, entry := range entries {
			if strings.HasPrefix(entry, "docker/registry/v2/blobs/") {
				blobCount++
			}
		}
	}
	return blobCount
}

// expectNoImageBlobsInArchives verifies that the given tar archives contain no image layer
// blobs (entries under docker/registry/v2/blobs/). Only repository metadata links and
// working-dir entries should be present.
func expectNoImageBlobsInArchives(tarFiles []string) {
	var blobEntries []string
	for _, tarFile := range tarFiles {
		entries := listTarEntries(tarFile)
		for _, entry := range entries {
			if strings.HasPrefix(entry, "docker/registry/v2/blobs/") {
				blobEntries = append(blobEntries, entry)
			}
		}
	}
	Expect(blobEntries).To(BeEmpty(),
		"expected no image blob entries in archive, but found %d:\n%s",
		len(blobEntries), strings.Join(blobEntries, "\n"))
}

var _ = Describe("incremental mirroring", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	// CLID-655: Validate date-based incremental mirroring using --since.
	// Runs mirrorToDisk twice with the same ISC: the first run produces a full
	// archive, and the second run with --since set to a future date produces an
	// archive with no image blobs (nothing is newer than the cutoff).
	Describe("mirrorToDisk incremental with --since flag", func() {
		iscFile := filepath.Join("happy_path", "isc-happy-path.yaml")

		It("should produce a smaller archive on incremental run with --since", SpecTimeout(5*time.Minute), func(_ SpecContext) {
			By("running initial mirrorToDisk")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscFile), workDir, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying initial tar archive was produced")
			initialArchives := findTarArchives(workDir)
			initialBlobCount := countBlobEntriesInArchives(initialArchives)
			GinkgoWriter.Printf("initial archive blob count: %d (files: %v)\n", initialBlobCount, initialArchives)
			Expect(initialBlobCount).To(BeNumerically(">", 0), "initial archive should contain blobs")

			By("removing initial tar archives to isolate incremental results")
			for _, f := range initialArchives {
				Expect(os.Remove(f)).To(Succeed())
			}

			By("running incremental mirrorToDisk with --since set to one year ahead (nothing new expected)")
			sinceDate := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
			GinkgoWriter.Printf("using --since date: %s\n", sinceDate)
			result, err = runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscFile), workDir,
				"--remove-signatures=true", "--since", sinceDate)
			expectOcMirrorCommandSuccess(result, err)

			By("verifying incremental archive has fewer blobs than initial")
			incrementalArchives := findTarArchives(workDir)
			incrementalBlobCount := countBlobEntriesInArchives(incrementalArchives)
			GinkgoWriter.Printf("incremental archive blob count: %d (initial was %d, files: %v)\n",
				incrementalBlobCount, initialBlobCount, incrementalArchives)
			Expect(incrementalBlobCount).To(BeNumerically("<", initialBlobCount),
				"incremental archive should have fewer blobs (%d) than initial (%d)",
				incrementalBlobCount, initialBlobCount)

			By("verifying incremental archive contains no image layer blobs")
			expectNoImageBlobsInArchives(incrementalArchives)
		})
	})

	// CLID-614: Validate config-based incremental mirroring for operators.
	// Runs mirrorToDisk twice with different ISCs against the same workspace:
	//   Step 1: mirror foo pinned at 0.2.0
	//   Step 2: expand foo to 0.2.0–0.3.1 and add bar (stable)
	// The second archive should contain only the new blobs (expanded foo versions
	// and bar) with no overlap from the first archive.
	Describe("operator incremental mirrorToDisk", func() {
		iscInitial := filepath.Join("operators", "isc-operator-incremental-initial.yaml")
		iscUpdate := filepath.Join("operators", "isc-operator-incremental-update.yaml")

		It("should produce a second tar with only incremental blob content", func() {
			findSingleMirrorTar := func(dir, phase string) string {
				matches, err := filepath.Glob(filepath.Join(dir, "mirror_*.tar"))
				Expect(err).NotTo(HaveOccurred())
				Expect(matches).To(HaveLen(1), "%s expected exactly one tar archive, got: %v", phase, matches)
				return matches[0]
			}

			iscInitialPath := filepath.Join(iscDir, iscInitial)
			iscUpdatePath := filepath.Join(iscDir, iscUpdate)

			By("running mirrorToDisk with the initial ISC (foo pinned at 0.2.0)")
			result, err := runner.MirrorToDisk(ctx, iscInitialPath, workDir, "--remove-signatures=true")
			logOcMirrorResult("incremental step-1 mirrorToDisk", result)
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the initial tar archive was created")
			initialTar := findSingleMirrorTar(workDir, "initial")
			Expect(initialTar).To(BeAnExistingFile(), "initial tar archive not found")
			logTarSummary("initial", initialTar)

			By("verifying the initial tar contains expected content")
			expectCorrectTarArchiveContents(iscInitialPath, workDir)

			By("moving the initial tar outside the working directory to preserve it")
			preserveDir, err := os.MkdirTemp("", "oc-mirror-preserved-tar-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(preserveDir)
			preservedTar := filepath.Join(preserveDir, "mirror_initial.tar")
			err = os.Rename(initialTar, preservedTar)
			Expect(err).NotTo(HaveOccurred(), "failed to move initial tar")

			By("running mirrorToDisk with the updated ISC (foo 0.2.0-0.3.1 + bar stable)")
			result, err = runner.MirrorToDisk(ctx, iscUpdatePath, workDir, "--remove-signatures=true")
			logOcMirrorResult("incremental step-2 mirrorToDisk", result)
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the incremental tar archive was created")
			incrementalTar := findSingleMirrorTar(workDir, "incremental")
			Expect(incrementalTar).To(BeAnExistingFile(), "incremental tar archive not found")
			logTarSummary("incremental", incrementalTar)

			By("comparing blob paths between the two tars")
			const blobPrefix = "docker/registry/v2/blobs/sha256"
			initialBlobs := collectTarBlobPaths(preservedTar, blobPrefix)
			incrementalBlobs := collectTarBlobPaths(incrementalTar, blobPrefix)
			GinkgoWriter.Printf("initial tar blob count:     %d\n", len(initialBlobs))
			GinkgoWriter.Printf("incremental tar blob count: %d\n", len(incrementalBlobs))

			Expect(incrementalBlobs).NotTo(BeEmpty(), "incremental tar should contain at least one blob")
			initialBlobSet := make(map[string]struct{}, len(initialBlobs))
			for _, b := range initialBlobs {
				initialBlobSet[b] = struct{}{}
			}
			for _, b := range incrementalBlobs {
				_, alreadyMirrored := initialBlobSet[b]
				Expect(alreadyMirrored).To(BeFalse(),
					"incremental tar re-included previously mirrored blob: %s", b)
			}

			By("verifying the initial tar does not contain the new operator package")
			initialEntries := listTarEntries(preservedTar)
			expectTarDoesNotContainPath(initialEntries, "/bar")

			By("verifying the incremental tar contains the expected repositories")
			expectCorrectTarArchiveContents(iscUpdatePath, workDir)
		})
	})
})
