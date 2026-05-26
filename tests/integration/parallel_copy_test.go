package integration_test

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
)

var _ = Describe("parallel copy", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	Describe("parallel-layers", func() {
		Describe("multilayers image mirrorToDisk", func() {
			iscFile := filepath.Join("parallel_copy", "isc-multilayers.yaml")

			It("should copy with --parallel-layers 10", func() {
				By("running mirrorToDisk with --parallel-layers 10")
				result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscFile), workDir,
					"--parallel-layers", "10",
				)
				expectOcMirrorCommandSuccess(result, err)
			})
		})
	})

	Describe("parallel-images", func() {
		iscFile := filepath.Join("parallel_copy", "isc-parallel-images.yaml")

		It("should copy 10 images in parallel with --parallel-images 10", func() {
			By("running mirrorToDisk with --parallel-images 10")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscFile), workDir,
				"--parallel-images", "10",
				"--log-level", "debug",
			)
			expectOcMirrorCommandSuccess(result, err)

			By("verifying enough images are collected to exercise parallelism")
			expectMinImagesToCopy(result, 10)
		})
	})
})

var imagesToCopyRe = regexp.MustCompile(`images to copy\s+(\d+)`)

// mirrorCopyOutput returns the section of oc-mirror output that covers image copying.
func mirrorCopyOutput(output string) string {
	start := strings.Index(output, "Start copying the images")
	if start == -1 {
		start = strings.Index(output, "going to discover")
	}
	if start == -1 {
		return output
	}

	end := strings.Index(output[start:], "=== Results ===")
	if end == -1 {
		return output[start:]
	}
	return output[start : start+end]
}

// imagesToCopyCount parses the "images to copy N" line from oc-mirror output.
// It only inspects the copy phase (after "Start copying the images") so debug
// lines like "total operator images to copy 0" are not matched first.
func imagesToCopyCount(output string) int {
	match := imagesToCopyRe.FindStringSubmatch(mirrorCopyOutput(output))
	if len(match) < 2 {
		return 0
	}
	count, err := strconv.Atoi(match[1])
	Expect(err).NotTo(HaveOccurred())
	return count
}

// expectMinImagesToCopy verifies the mirror collected at least the expected number of images.
func expectMinImagesToCopy(result *ocmirror.Result, min int) {
	output := result.Stdout + result.Stderr
	count := imagesToCopyCount(output)
	Expect(count).To(BeNumerically(">=", min),
		"expected at least %d images to copy but got %d\nstdout: %s\nstderr: %s",
		min, count, result.Stdout, result.Stderr)
}
