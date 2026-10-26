package integration_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("platform targetRepo destination conflicts", Label("platform-target-repo"), func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
		cacheDir = filepath.Join(workDir, "cache", ".oc-mirror", ".cache")
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	It("rejects an additional image that would overwrite the release payload", func() {
		isc := filepath.Join(iscDir, "platform_target_repo", "isc-conflict.yaml")
		result, err := runner.MirrorToMirror(ctx, isc, workDir, testRegistry.Endpoint(),
			"--cache-dir", filepath.Join(workDir, "cache"), "--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorExitCode(result, err, 1, "conflicting destination", "/releases:v0.0.1")
		expectEmptyRegistry(*testRegistry)
	})

	It("rejects a destination namespace that would cause targetRepo to be flattened", func() {
		result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, "platform_target_repo", "isc.yaml"), workDir, testRegistry.Endpoint()+"/team",
			"--cache-dir", filepath.Join(workDir, "cache"), "--dest-tls-verify=false", "--max-nested-paths=2")
		expectOcMirrorExitCode(result, err, 1, "platform.targetRepo", "exceeds --max-nested-paths=2")
		expectEmptyRegistry(*testRegistry)
	})

	DescribeTable("reserves the graph repository before collection can push it", func(repo string, limit int) {
		isc := filepath.Join(workDir, "graph-conflict.yaml")
		Expect(os.WriteFile(isc, fmt.Appendf(nil, `kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v2alpha1
mirror:
  platform:
    targetRepo: %s
    graph: true
`, repo), 0600)).To(Succeed())
		result, err := runner.MirrorToMirror(ctx, isc, workDir, testRegistry.Endpoint()+"/team",
			"--cache-dir", filepath.Join(workDir, "cache"), "--dest-tls-verify=false", fmt.Sprintf("--max-nested-paths=%d", limit))
		expectOcMirrorExitCode(result, err, 1, "conflicts with the Cincinnati graph repository")
		expectEmptyRegistry(*testRegistry)
	},
		Entry("original path", "openshift/graph-image", 0),
		Entry("flattened path", "openshift-graph-image", 2),
	)
})
