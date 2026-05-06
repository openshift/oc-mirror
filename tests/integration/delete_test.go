package integration_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("delete", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	Describe("delete functionality tests", func() {
		iscHappyPath := filepath.Join("happy_path", "isc-happy-path.yaml")
		discHappyPath := filepath.Join("happy_path", "disc-happy-path.yaml")
		deleteId := "delete-test"

		It("should create delete yaml files with the delete-id in their names", func() {
			deleteYaml := filepath.Join(workDir, "working-dir", "delete", "delete-images-"+deleteId+".yaml")

			By("running mirrorToMirror")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscHappyPath), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("running delete phase 1 with --delete-id")
			result, err = runner.DeletePhaseOne(ctx, filepath.Join(iscDir, discHappyPath), workDir, deleteId, testRegistry.Endpoint())
			expectOcMirrorCommandSuccess(result, err)

			By("verifying delete images files are correct")
			expectValidDeleteImagesFiles(workDir, deleteId)

			By("running delete phase 2")
			result, err = runner.DeletePhaseTwo(ctx, deleteYaml, testRegistry.Endpoint(),
				"--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying local registry is empty after delete")
			expectEmptyRegistry(*testRegistry)
		})
	})
})
