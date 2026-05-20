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

			It("should copy at most 10 layers in parallel with --parallel-layers 10", func() {
				By("running mirrorToDisk with --parallel-layers 10")
				result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscFile), workDir,
					"--parallel-images", "1",
					"--parallel-layers", "10",
					"--log-level", "debug",
				)
				expectOcMirrorCommandSuccess(result, err)

				By("verifying no more than 10 layers were copied concurrently")
				expectParallelLayersLimit(result, 10)
			})
		})

		Describe("operator mirrorToDisk", func() {
			iscFile := filepath.Join("operators", "isc-operator-pinned-version.yaml")

			It("should copy at most 5 layers in parallel using the default parallel-layers value", func() {
				By("running mirrorToDisk with default parallel-layers")
				result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscFile), workDir,
					"--parallel-images", "1",
					"--log-level", "debug",
				)
				expectOcMirrorCommandSuccess(result, err)

				By("verifying no more than 5 layers were copied concurrently")
				expectParallelLayersLimit(result, 5)
			})
		})
	})

	Describe("parallel-images", func() {
		iscFile := filepath.Join("parallel_copy", "isc-parallel-images.yaml")

		It("should copy at most 10 images in parallel with --parallel-images 10", func() {
			By("running mirrorToDisk with --parallel-images 10")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscFile), workDir,
				"--parallel-images", "10",
				"--log-level", "debug",
			)
			expectOcMirrorCommandSuccess(result, err)

			By("verifying enough images are collected to exercise parallelism")
			expectMinImagesToCopy(result, 10)

			By("verifying no more than 10 images were copied concurrently")
			expectParallelImagesLimit(result, 10)
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

// maxConcurrentCopyingBlobs returns the maximum number of consecutive "Copying blob" lines in oc-mirror debug output.
func maxConcurrentCopyingBlobs(output string) int {
	copyingBlobLineRe := regexp.MustCompile(`^Copying blob `)

	max := 0
	current := 0
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if copyingBlobLineRe.MatchString(trimmed) {
			current++
			if current > max {
				max = current
			}
			continue
		}
		current = 0
	}
	return max
}

// maxConcurrentImageCopies returns the peak number of images copying concurrently in progress output.
func maxConcurrentImageCopies(output string) int {
	output = mirrorCopyOutput(output)
	max := 0
	inProgress := 0

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "➡️") {
			if inProgress > max {
				max = inProgress
			}
			inProgress = 0
			continue
		}
		if strings.HasPrefix(trimmed, "✓") {
			continue
		}
		inProgress++
		if inProgress > max {
			max = inProgress
		}
	}

	if inProgress > max {
		max = inProgress
	}
	return max
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

// expectParallelImagesLimit verifies image copies did not exceed the configured parallel-images limit.
func expectParallelImagesLimit(result *ocmirror.Result, limit int) {
	output := mirrorCopyOutput(result.Stdout + result.Stderr)
	concurrent := maxConcurrentImageCopies(output)
	Expect(concurrent).To(BeNumerically(">", 0),
		"expected at least one concurrent image copy in output\nstdout: %s\nstderr: %s",
		result.Stdout, result.Stderr)
	Expect(concurrent).To(BeNumerically("<=", limit),
		"expected at most %d concurrent image copies but saw %d\nstdout: %s\nstderr: %s",
		limit, concurrent, result.Stdout, result.Stderr)
}

// expectParallelLayersLimit verifies layer copies did not exceed the configured parallel-layers limit.
func expectParallelLayersLimit(result *ocmirror.Result, limit int) {
	output := mirrorCopyOutput(result.Stdout + result.Stderr)
	concurrent := maxConcurrentCopyingBlobs(output)
	Expect(concurrent).To(BeNumerically(">", 0),
		"expected at least one concurrent blob copy in output\nstdout: %s\nstderr: %s",
		result.Stdout, result.Stderr)
	Expect(concurrent).To(BeNumerically("<=", limit),
		"expected at most %d concurrent layer copies but saw %d\nstdout: %s\nstderr: %s",
		limit, concurrent, result.Stdout, result.Stderr)
}
