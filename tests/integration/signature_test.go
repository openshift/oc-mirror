package integration_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("signatures", func() {
	var workDir string

	iscSignatures := filepath.Join("signatures", "isc-signatures.yaml")
	discSignatures := filepath.Join("signatures", "disc-signatures.yaml")

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	It("should not mirror signatures when using --remove-signatures=true", func() {
		By("running mirrorToDisk with --remove-signatures=true")
		result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscSignatures), workDir, "--remove-signatures=true")
		expectOcMirrorCommandSuccess(result, err)

		By("running diskToMirror with --remove-signatures=true")
		result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscSignatures), workDir, testRegistry.Endpoint(),
			"--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying no signatures are present in the local registry")
		expectNoSignaturesInRegistry(*testRegistry)
	})

	It("should mirror with signatures preserved and delete them", func() {
		deleteYaml := filepath.Join(workDir, "working-dir", "delete", "delete-images.yaml")

		By("running mirrorToDisk")
		result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscSignatures), workDir)
		expectOcMirrorCommandSuccess(result, err)

		By("running diskToMirror")
		result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscSignatures), workDir, testRegistry.Endpoint(), "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying signatures are present in the local registry")
		expectSignaturesInRegistry(*testRegistry)

		By("running delete with --delete-signatures")
		result, err = runner.DeletePhaseOne(ctx, filepath.Join(iscDir, discSignatures), workDir, "", testRegistry.Endpoint(),
			"--delete-signatures")
		expectOcMirrorCommandSuccess(result, err)

		result, err = runner.DeletePhaseTwo(ctx, deleteYaml, testRegistry.Endpoint(),
			"--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying non-catalog signatures are deleted")
		expectNonCatalogSignaturesDeleted(*testRegistry)
	})

	It("should keep signature tags when delete does not use --delete-signatures", func() {
		deleteYaml := filepath.Join(workDir, "working-dir", "delete", "delete-images.yaml")

		By("running mirrorToDisk")
		result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscSignatures), workDir)
		expectOcMirrorCommandSuccess(result, err)

		By("running diskToMirror")
		result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscSignatures), workDir, testRegistry.Endpoint(), "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying signatures are present in the local registry")
		expectSignaturesInRegistry(*testRegistry)

		By("running delete without --delete-signatures")
		result, err = runner.DeletePhaseOne(ctx, filepath.Join(iscDir, discSignatures), workDir, "", testRegistry.Endpoint())
		expectOcMirrorCommandSuccess(result, err)

		result, err = runner.DeletePhaseTwo(ctx, deleteYaml, testRegistry.Endpoint(),
			"--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying signature tags are still present")
		expectOnlySignatureTagsRemain(*testRegistry)
	})
})
