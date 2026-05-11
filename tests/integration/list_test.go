package integration_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
)

const testCatalog string = "quay.io/oc-mirror/oc-mirror-dev:test-catalog-latest"

var _ = Describe("oc-mirror list operators", Ordered, func() {
	BeforeAll(func() {
		if skipListTest() {
			Skip("oc-mirror list operators not available in < 4.22")
		}
	})
	It("should list operators of a specific catalog", func() {
		result, err := runner.ListOperators(ctx, "--catalog", testCatalog)
		expectOcMirrorCommandSuccess(result, err)
		// FIXME: consider getting this information dynamically once our test catalog gets more complex.
		expected := [][]string{
			{"bar", "", "stable"},
			{"baz", "", "stable"},
			{"foo", "", "beta"},
		}
		header := []string{"NAME", "DISPLAY NAME", "DEFAULT CHANNEL"}
		compareTableOutput(header, result.Stdout, expected)
	})
	It("should list channels of a specific operator", func() {
		result, err := runner.ListOperators(ctx, "--catalog", testCatalog, "--package", "foo")
		expectOcMirrorCommandSuccess(result, err)
		lines := slices.Collect(strings.Lines(result.Stdout))

		// First 2 lines show the default channel
		defChannelResult := strings.Join(lines[:2], "")
		header := []string{"NAME", "DISPLAY NAME", "DEFAULT CHANNEL"}
		compareTableOutput(header, defChannelResult, [][]string{{"foo", "", "beta"}})

		// The other lines contain the channels in the operator
		chanResult := strings.Join(lines[3:], "")
		header = []string{"PACKAGE", "CHANNEL", "HEAD"}
		compareTableOutput(header, chanResult, [][]string{{"foo", "beta", "foo.v0.3.1"}})
	})
	It("should list versions of a specific channel", func() {
		result, err := runner.ListOperators(ctx, "--catalog", testCatalog, "--package", "foo", "--channel", "beta")
		expectOcMirrorCommandSuccess(result, err)
		expected := []string{"0.1.0", "0.2.0", "0.3.0", "0.3.1"}
		const headerCount int = 1 // VERSIONS\n
		compareLineOutput(result.Stdout, headerCount, expected)
	})
})

var _ = Describe("oc-mirror list releases", Ordered, func() {
	// Initialize oc-mirror runner
	binaryPath := os.Getenv("OC_MIRROR_BINARY")
	// Custom runner so we can override envs without affecting other tests
	releaseRunner := ocmirror.NewRunner(binaryPath)
	BeforeAll(func() {
		if skipListTest() {
			Skip("oc-mirror list releases not available in < 4.22")
		}
		graphData, err := os.ReadFile(filepath.Join(graphDataDir, "graph_data.json"))
		Expect(err).NotTo(HaveOccurred(), "should read test graph data")
		// Mock Cincinnati endpoint.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if contentType := r.Header.Get("Accept"); contentType != "application/json" {
				io.WriteString(w, `{"kind":"invalid_content_type","value": "invalid Content-Type requested"}`)
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}
			query := r.URL.Query()
			if query.Get("channel") == "" {
				io.WriteString(w, `{"kind":"missing_params","value":"mandatory client parameters missing: channel"}`)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Write(graphData)
		}))
		releaseRunner = releaseRunner.WithEnv([]string{fmt.Sprintf("UPDATE_URL_OVERRIDE=%s", server.URL)})
		DeferCleanup(server.Close)
	})
	It("should list OCP major releases ", func() {
		result, err := releaseRunner.ListReleases(ctx)
		expectOcMirrorCommandSuccess(result, err)
		const headerCount int = 1 // VERSIONS\n
		compareLineOutput(result.Stdout, headerCount, []string{"4.20", "4.21", "4.22", "5.0"})
	})
	It("should list all versions in a given release", func() {
		result, err := releaseRunner.ListReleases(ctx, "--version", "4.20")
		expectOcMirrorCommandSuccess(result, err)
		const headerCount int = 4 // "Listing stable channels...\nUse oc-mirror ...\n\nVersions\n"
		compareLineOutput(result.Stdout, headerCount, []string{"4.20.0", "4.20.1", "4.20.2"})
	})
	It("should list all channels in a given version", func() {
		result, err := releaseRunner.ListReleases(ctx, "--channels", "--version", "4.20")
		expectOcMirrorCommandSuccess(result, err)
		const headerCount int = 2 // Listing channels...\n\n
		compareLineOutput(result.Stdout, headerCount, []string{"stable-4.20", "candidate-4.20", "eus-4.20", "fast-4.20"})
	})
})

// Parse indexes of columns starts based on header
func parseColumnIndexesFromHeader(headerFields []string, header string) []int {
	indexes := make([]int, 0, len(headerFields))
	for _, field := range headerFields {
		idx := strings.Index(header, field)
		indexes = append(indexes, idx)
	}
	return indexes
}

func parseFields(headerIndexes []int, line string) []string {
	nIndexes := len(headerIndexes)
	fields := make([]string, 0, nIndexes)
	for i, idx := range headerIndexes[:nIndexes-1] {
		nextIdx := headerIndexes[i+1]
		fields = append(fields, strings.TrimSpace(line[idx:nextIdx]))
	}
	fields = append(fields, strings.TrimSpace(line[headerIndexes[nIndexes-1]:]))
	return fields
}

func compareTableOutput(headerFields []string, result string, expected [][]string) {
	lines := slices.Collect(strings.Lines(result))
	indexes := parseColumnIndexesFromHeader(headerFields, lines[0])
	for idx, line := range lines[1:] { // skip header
		fields := parseFields(indexes, line)
		Expect(fields).Should(HaveExactElements(expected[idx]), "line %d does not match", idx)
	}
}

func compareLineOutput(result string, skipHeadersCount int, expected []string) {
	lines := make([]string, 0, len(result))
	for line := range strings.Lines(result) {
		if skipHeadersCount > 0 {
			skipHeadersCount--
			continue
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	Expect(lines).To(ContainElements(expected), "mismatched elements")
}

func skipListTest() bool {
	return isOCMirrorVersionBefore(4, 22)
}
