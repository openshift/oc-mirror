package integration_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("mirrorToMirror", func() {
	var workDir string
	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	Describe("mirrorToMirror happy path", func() {
		iscHappyPath := filepath.Join("happy_path", "isc-happy-path.yaml")
		discHappyPath := filepath.Join("happy_path", "disc-happy-path.yaml")

		It("should mirror from remote registry to a local registry", func() {
			deleteYaml := filepath.Join(workDir, "working-dir", "delete", "delete-images.yaml")

			By("running mirrorToMirror")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscHappyPath), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying images are mirrored in the local registry")
			expectSuccessfulMirrorInRegistry(filepath.Join(iscDir, iscHappyPath), *testRegistry)

			By("verifying IDMS contains all the expected images and no empty fields")
			expectCorrectIDMS(workDir, filepath.Join(iscDir, iscHappyPath))

			By("running delete workflow - phase 1: generating delete yaml")
			result, err = runner.DeletePhaseOne(ctx, filepath.Join(iscDir, discHappyPath), workDir, "", testRegistry.Endpoint())
			expectOcMirrorCommandSuccess(result, err)

			By("verifying delete images files are correct")
			expectValidDeleteImagesFiles(workDir, "")

			By("running delete workflow - phase 2: delete images from registry")
			result, err = runner.DeletePhaseTwo(ctx, deleteYaml, testRegistry.Endpoint(),
				"--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying local registry is empty after delete")
			expectEmptyRegistry(*testRegistry)

		})
	})

	Describe("mirrorToMirror when no image can be copied", func() {
		It("should not generate any cluster resources", func() {
			isc := filepath.Join(iscDir, "exit_codes", "isc-additional-image-error.yaml")

			By("running mirrorToMirror with only nonexistent additional images")
			result, err := runner.MirrorToMirror(ctx, isc, workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorExitCode(result, err, 8, "some errors occurred during the mirroring")

			By("verifying nothing was mirrored and no cluster resources were generated")
			expectNoRepositoriesInRegistry(*testRegistry)
			expectNoClusterResources(workDir)
		})
	})

})

func expectNoClusterResources(workDir string) {
	GinkgoHelper()
	files, err := filepath.Glob(filepath.Join(workDir, dirWorkingDir, dirClusterResources, "*"))
	Expect(err).NotTo(HaveOccurred())
	Expect(files).To(BeEmpty(), "expected no cluster resources to be generated")
}
