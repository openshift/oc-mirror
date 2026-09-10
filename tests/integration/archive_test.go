package integration_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
)

var _ = Describe("tar archive edge cases", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	// OCPBUGS-57461 / OCP-83817
	Describe("empty tar files", func() {
		It("first tar file is empty", func() {
			iscHappyPath := filepath.Join("happy_path", "isc-happy-path.yaml")

			By("creating an empty tar file")
			createEmptyTar(workDir, "mirror_000001.tar")

			By("running diskToMirror")
			result, err := runner.DiskToMirror(ctx, filepath.Join(iscDir, iscHappyPath), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorEmptyTarError(result, err)
		})

		It("tar file other than first is empty", func() {
			iscHappyPath := filepath.Join("happy_path", "isc-happy-path.yaml")

			By("running mirrorToDisk to generate a valid first tar file")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscHappyPath), workDir, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying tar archive contents")
			expectCorrectTarArchiveContents(filepath.Join(iscDir, iscHappyPath), workDir)

			By("creating an extra empty tar file")
			createEmptyTar(workDir, "mirror_000002.tar")

			By("running diskToMirror")
			result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscHappyPath), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorEmptyTarError(result, err)
		})
	})

	// OCP-83128
	Describe("non-unique tar file names", func() {
		iscHappyPath := filepath.Join("happy_path", "isc-happy-path.yaml")

		It("should clean existing tar files", func() {
			By("running mirrorToDisk")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscHappyPath), workDir, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			tarPath := filepath.Join(workDir, "mirror_000001.tar")
			newTarPath := filepath.Join(workDir, "mirror_000002.tar")
			By("verifying tar archive contents")
			expectCorrectTarArchiveContents(filepath.Join(iscDir, iscHappyPath), workDir)
			Expect(newTarPath).ToNot(BeAnExistingFile(), "should generate only one tar file")

			By("renaming existing tar")
			err = os.Rename(tarPath, newTarPath)
			Expect(err).ToNot(HaveOccurred(), "should rename tar file")

			By("running mirrorToDisk again")
			result, err = runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscHappyPath), workDir, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying tar archive contents")
			expectCorrectTarArchiveContents(filepath.Join(iscDir, iscHappyPath), workDir)
			Expect(newTarPath).NotTo(BeAnExistingFile(), "previous tar files should be deleted")
			Expect(tarPath).To(BeAnExistingFile(), "mirror_000001.tar should exist")
		})
	})
})

// CLID-725: Validate that oc-mirror fails when a file exceeds the strict archive size limit.
// The ImageSetConfiguration sets archiveSize: 1 (1 GB). A 2 GB sparse file is injected into the
// working directory so that the strict adder encounters it during archive creation and errors out.
var _ = Describe("strict archive size", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	iscFile := filepath.Join("archive", "isc-strict-archive.yaml")

	It("should fail mirrorToDisk with --strict-archive when a file exceeds archiveSize", SpecTimeout(5*time.Minute), func(specCtx SpecContext) {
		By("creating a 2 GB sparse file in the working directory to exceed the 1 GB archiveSize limit")
		oversizedFile := filepath.Join(workDir, dirWorkingDir, "oversized-test-file.bin")
		createSparseFile(oversizedFile, 2*1024*1024*1024)

		By("running mirrorToDisk with --strict-archive and archiveSize: 1")
		result, err := runner.MirrorToDisk(specCtx, filepath.Join(iscDir, iscFile), workDir,
			"--remove-signatures=true", "--strict-archive")
		logOcMirrorResult("strict-archive mirrorToDisk", result)
		expectOcMirrorCommandFailure(result, err)

		By("verifying the error output mentions the archive size limit")
		output := result.Stdout + result.Stderr
		Expect(output).To(ContainSubstring("maxArchiveSize 1G is too small compared to sizes of files"),
			"expected strict archive error in output:\nstdout: %s\nstderr: %s", result.Stdout, result.Stderr)
	})

	It("should succeed mirrorToDisk with --strict-archive when content fits within archiveSize", SpecTimeout(5*time.Minute), func(specCtx SpecContext) {
		By("running mirrorToDisk with --strict-archive and archiveSize: 1 (no oversized files)")
		result, err := runner.MirrorToDisk(specCtx, filepath.Join(iscDir, iscFile), workDir,
			"--remove-signatures=true", "--strict-archive")
		logOcMirrorResult("strict-archive-success mirrorToDisk", result)
		expectOcMirrorCommandSuccess(result, err)

		By("verifying tar archive was produced with expected content")
		expectCorrectTarArchiveContents(filepath.Join(iscDir, iscFile), workDir)
	})
})

// createSparseFile creates a sparse file at the given path with the specified reported size.
// The file takes no actual disk space but os.Stat reports the full size.
func createSparseFile(path string, size int64) {
	f, err := os.Create(path)
	Expect(err).NotTo(HaveOccurred(), "failed to create sparse file %s", path)
	err = f.Truncate(size)
	Expect(err).NotTo(HaveOccurred(), "failed to truncate sparse file %s to %d bytes", path, size)
	err = f.Close()
	Expect(err).NotTo(HaveOccurred(), "failed to close sparse file %s", path)
}

func createEmptyTar(workdir, name string) {
	filename := filepath.Join(workdir, name)
	file, err := os.Create(filename)
	Expect(err).ToNot(HaveOccurred(), "should create tar file")
	err = file.Close()
	Expect(err).ToNot(HaveOccurred(), "should close tar file")
	Expect(filename).To(BeAnExistingFile(), "tar file shoud exist")
	stat, err := os.Stat(filename)
	Expect(err).ToNot(HaveOccurred(), "should stat tar file")
	Expect(stat.Size()).To(BeZero(), "tar file should be empty")
}

func expectOcMirrorEmptyTarError(result *ocmirror.Result, err error) {
	Expect(err).ToNot(HaveOccurred())
	Expect(result.ExitCode).ToNot(BeZero(), "expected non-zero exit code")
	Expect(result.Stdout).To(ContainSubstring("empty archive file"), "should contain empty file error")
}
