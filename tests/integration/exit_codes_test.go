package integration_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("exit codes", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	It("should return exit code 1 for ISC with unknown field (generic error)", func() {
		isc := filepath.Join(iscDir, "exit_codes", "isc-generic-error.yaml")

		By("running M2D with an ISC containing the unknown field 'storageConfig'")
		result, err := runner.MirrorToDisk(ctx, isc, workDir)
		expectOcMirrorExitCode(result, err, 1, "unknown field")

		By("verifying no tar archive was produced")
		expectNoTarArchive(workDir)
	})

	It("should return exit code 2 for nonexistent release channel (fatal release error)", func() {
		isc := filepath.Join(iscDir, "exit_codes", "isc-release-error.yaml")

		By("running M2D with an ISC referencing nonexistent channel stable-99.99")
		result, err := runner.MirrorToDisk(ctx, isc, workDir)
		expectOcMirrorExitCode(result, err, 2, "no release images found")

		By("verifying no tar archive was produced")
		expectNoTarArchive(workDir)
	})

	It("should return exit code 4 when operator catalog collection fails", func() {
		isc := filepath.Join(iscDir, "exit_codes", "isc-operator-error.yaml")

		By("running M2D with a valid release but an invalid operator catalog")
		result, err := runner.MirrorToDisk(ctx, isc, workDir, "--remove-signatures=true")
		expectOcMirrorExitCode(result, err, 4, "collection error", "registry.invalid.example.com")

		By("verifying no tar archive was produced")
		expectNoTarArchive(workDir)
	})

	It("should return exit code 8 when additional images fail to mirror", func() {
		isc := filepath.Join(iscDir, "exit_codes", "isc-additional-image-error.yaml")

		By("running M2D with only nonexistent additional images")
		result, err := runner.MirrorToDisk(ctx, isc, workDir)
		expectOcMirrorExitCode(result, err, 8,
			"some errors occurred during the mirroring",
			"registry.invalid.example.com/nonexistent/image:latest",
			"quay.io/nonexistent-org/nonexistent-image:v1.0.0",
		)
	})

	It("should return exit code 16 when helm chart path does not exist", func() {
		isc := filepath.Join(iscDir, "exit_codes", "isc-helm-error.yaml")

		By("running M2D with a nonexistent local helm chart path")
		result, err := runner.MirrorToDisk(ctx, isc, workDir)
		expectOcMirrorExitCode(result, err, 16, "no such file or directory")

		By("verifying no tar archive was produced")
		expectNoTarArchive(workDir)
	})

	It("should return exit code 20 when both operator and helm fail (4|16)", func() {
		isc := filepath.Join(iscDir, "exit_codes", "isc-operator-helm-error.yaml")

		By("running M2D with an invalid operator catalog and a nonexistent helm chart path")
		result, err := runner.MirrorToDisk(ctx, isc, workDir)
		expectOcMirrorExitCode(result, err, 20, "collection error", "registry.invalid.example.com", "no such file or directory")

		By("verifying no tar archive was produced")
		expectNoTarArchive(workDir)
	})
})
