// incremental_mirroring_test.go validates the incremental mirroring workflow using the --since flag.
// It verifies that oc-mirror produces a smaller archive (no image layer blobs) when the
// --since date filters out previously mirrored content.
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

// CLID-655: Validate incremental mirroring using archives filtered by date.
var _ = Describe("incremental mirroring", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	// Verifies that --since flag produces an incremental archive with only new content.
	// When --since is set to a future date (nothing cached after that date), the archive
	// should contain no image layer blobs, only repository metadata links.
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
})
