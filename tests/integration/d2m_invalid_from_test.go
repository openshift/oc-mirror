package integration_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
)

// OCPBUGS-58193 and test case OCP-83091
var _ = Describe("Clear error message from invalid --from", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
		os.RemoveAll("invalid-dir")
	})

	iscHappyPath := filepath.Join("happy_path", "isc-happy-path.yaml")

	It("--from folder does not exist", func() {
		By("running diskToMirror")
		result, err := runner.DiskToMirror(ctx, filepath.Join(iscDir, iscHappyPath), "invalid-dir", testRegistry.Endpoint(),
			"--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandInvalidFrom(result, err)
	})

	It("--from folder is empty", func() {
		By("running diskToMirror")
		result, err := runner.DiskToMirror(ctx, filepath.Join(iscDir, iscHappyPath), workDir, testRegistry.Endpoint(),
			"--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandInvalidFrom(result, err)
	})
})

func expectOcMirrorCommandInvalidFrom(result *ocmirror.Result, err error) {
	Expect(err).ToNot(HaveOccurred())
	Expect(result.ExitCode).ToNot(BeZero(), "should be non-zero exit status")
	Expect(result.Stdout).To(ContainSubstring(`no tar archives matching "mirror_[0-9]{6}\\.tar"`))
}
