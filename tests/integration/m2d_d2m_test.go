package integration_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("mirrorToDisk + diskToMirror", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	Describe("mirrorToDisk + diskToMirror happy path", func() {
		iscHappyPath := filepath.Join("happy_path", "isc-happy-path.yaml")
		discHappyPath := filepath.Join("happy_path", "disc-happy-path.yaml")

		It("should mirror from remote registry to disk and then from disk to local registry", func() {
			deleteYaml := filepath.Join(workDir, "working-dir", "delete", "delete-images.yaml")

			By("running mirrorToDisk")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscHappyPath), workDir, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying images are mirrored in the local cache registry")
			expectSuccessfulMirrorInLocalCache(filepath.Join(iscDir, iscHappyPath), cacheDir)

			By("verifying tar archive contents")
			expectCorrectTarArchiveContents(filepath.Join(iscDir, iscHappyPath), workDir)

			By("running diskToMirror")
			result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscHappyPath), workDir, testRegistry.Endpoint(),
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
})
