package integration_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
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
})
