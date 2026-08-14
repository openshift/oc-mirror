// breakpoint_resume_test.go validates that oc-mirror can recover from an interrupted
// mirrorToDisk run and that re-running with a warm cache produces correct results.
//
// OCP-73039: [CLID-5] Support breakpoint resume for v2
package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("breakpoint resume", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	Describe("mirrorToDisk", func() {
		iscFile := filepath.Join("happy_path", "isc-happy-path.yaml")

		It("should complete successfully after being interrupted and resumed", SpecTimeout(5*time.Minute), func(specCtx SpecContext) {
			iscPath := filepath.Join(iscDir, iscFile)

			By("running mirrorToDisk with an early interruption")
			interruptCtx, interruptCancel := context.WithTimeout(specCtx, 1*time.Second)
			defer interruptCancel()

			result, err := runner.MirrorToDisk(interruptCtx, iscPath, workDir, "--remove-signatures=true")
			logOcMirrorResult("interrupted run", result)

			wasInterrupted := err != nil || result.ExitCode != 0
			GinkgoWriter.Printf("first run interrupted: %v (err=%v, exitCode=%d)\n",
				wasInterrupted, err, result.ExitCode)
			Expect(wasInterrupted).To(BeTrue(), "expected the first run to be interrupted by the timeout")

			By("cleaning partial working directory state from the interrupted run")
			entries, readErr := os.ReadDir(filepath.Join(workDir, dirWorkingDir))
			Expect(readErr).NotTo(HaveOccurred())
			for _, entry := range entries {
				if entry.Name() != "signatures" {
					Expect(os.RemoveAll(filepath.Join(workDir, dirWorkingDir, entry.Name()))).To(Succeed())
				}
			}
			partialArchives, _ := filepath.Glob(filepath.Join(workDir, "mirror_*.tar"))
			for _, f := range partialArchives {
				Expect(os.Remove(f)).To(Succeed())
			}

			By("resuming mirrorToDisk")
			result, err = runner.MirrorToDisk(specCtx, iscPath, workDir, "--remove-signatures=true")
			logOcMirrorResult("resumed run", result)
			expectOcMirrorCommandSuccess(result, err)

			By("verifying images are mirrored in the local cache registry")
			expectSuccessfulMirrorInLocalCache(iscPath, cacheDir)

			By("verifying tar archive contents")
			expectCorrectTarArchiveContents(iscPath, workDir)
		})

		It("should produce correct results when re-run with existing cache", SpecTimeout(5*time.Minute), func(specCtx SpecContext) {
			iscPath := filepath.Join(iscDir, iscFile)

			By("running initial mirrorToDisk")
			result, err := runner.MirrorToDisk(specCtx, iscPath, workDir, "--remove-signatures=true")
			logOcMirrorResult("initial run", result)
			expectOcMirrorCommandSuccess(result, err)

			By("verifying initial mirror produced correct results")
			expectSuccessfulMirrorInLocalCache(iscPath, cacheDir)
			expectCorrectTarArchiveContents(iscPath, workDir)

			By("counting blob entries in the initial archive")
			initialArchives := findTarArchives(workDir)
			initialBlobCount := countBlobEntriesInArchives(initialArchives)
			GinkgoWriter.Printf("initial archive blob count: %d (files: %v)\n", initialBlobCount, initialArchives)
			Expect(initialBlobCount).To(BeNumerically(">", 0), "initial archive should contain blobs")

			By("removing tar archives while keeping the cache and history")
			for _, f := range initialArchives {
				Expect(os.Remove(f)).To(Succeed())
			}

			By("running mirrorToDisk again with existing cache")
			result, err = runner.MirrorToDisk(specCtx, iscPath, workDir, "--remove-signatures=true")
			logOcMirrorResult("cache-warm run", result)
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the cached run produced fewer archive blobs")
			cachedArchives := findTarArchives(workDir)
			cachedBlobCount := countBlobEntriesInArchives(cachedArchives)
			GinkgoWriter.Printf("cached archive blob count: %d (initial was %d, files: %v)\n",
				cachedBlobCount, initialBlobCount, cachedArchives)
			Expect(cachedBlobCount).To(BeNumerically("<", initialBlobCount),
				"cached archive should have fewer blobs (%d) than initial (%d)",
				cachedBlobCount, initialBlobCount)

			By("verifying images are mirrored in the local cache registry")
			expectSuccessfulMirrorInLocalCache(iscPath, cacheDir)

			By("verifying tar archive contents")
			expectCorrectTarArchiveContents(iscPath, workDir)
		})
	})
})
