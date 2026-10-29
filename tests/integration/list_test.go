package integration_test

import (
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
	"github.com/openshift/oc-mirror/tests/integration/pkg/proxy"
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
	It("lists both channels and the default channel of a multi-channel package", func() {
		result, err := runner.ListOperators(ctx, "--catalog", testCatalog, "--package", "bar")
		expectOcMirrorCommandSuccess(result, err)
		sections := strings.SplitN(result.Stdout, "\n\n", 2)
		Expect(sections).To(HaveLen(2))
		compareTableOutput([]string{"NAME", "DISPLAY NAME", "DEFAULT CHANNEL"}, sections[0]+"\n",
			[][]string{{"bar", "", "stable"}})
		compareTableOutput([]string{"PACKAGE", "CHANNEL", "HEAD"}, sections[1],
			[][]string{{"bar", "alpha", "bar.v1.0.0"}, {"bar", "stable", "bar.v1.0.0"}})
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
				w.WriteHeader(http.StatusNotAcceptable)
				io.WriteString(w, `{"kind":"invalid_content_type","value": "invalid Content-Type requested"}`)
				return
			}
			query := r.URL.Query()
			if query.Get("channel") == "" {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, `{"kind":"missing_params","value":"mandatory client parameters missing: channel"}`)
				return
			}
			w.Write(graphData)
		}))
		// Tag listing contacts the origin directly rather than using registries.conf
		// pull mirrors, so route only this hardcoded host to a local TLS endpoint.
		registryServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/":
				w.WriteHeader(http.StatusOK)
			case "/v2/openshift-release-dev/ocp-release/tags/list":
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"name":"openshift-release-dev/ocp-release","tags":["4.20.0-amd64","4.20.1-arm64","4.21.0-amd64","4.22.0-amd64","5.0.0-amd64","99.99.0.sig","latest"]}`)
			default:
				http.NotFound(w, r)
			}
		}))
		DeferCleanup(registryServer.Close)
		routingProxy, err := proxy.StartWithRoutes(map[string]string{"quay.io:443": registryServer.Listener.Addr().String()})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(routingProxy.Stop)
		releaseRunner = releaseRunner.WithEnv([]string{
			"UPDATE_URL_OVERRIDE=" + server.URL,
			"HTTP_PROXY=" + routingProxy.URL(), "HTTPS_PROXY=" + routingProxy.URL(),
			"NO_PROXY=localhost,127.0.0.1",
		})
		DeferCleanup(server.Close)
	})
	It("should list OCP major releases ", func() {
		result, err := releaseRunner.ListReleases(ctx, "--src-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		const headerCount int = 1 // VERSIONS\n
		compareLineOutput(result.Stdout, headerCount, []string{"4.20", "4.21", "4.22", "5.0"})
	})
	It("should list all versions in a given release", func() {
		result, err := releaseRunner.ListReleases(ctx, "--version", "4.20")
		expectOcMirrorCommandSuccess(result, err)
		const headerCount int = 5 // Stable-channel notices, blank line, channel, architecture.
		compareLineOutput(result.Stdout, headerCount, []string{"4.20.0", "4.20.1", "4.20.2"})
	})
	It("should list all channels in a given version", func() {
		result, err := releaseRunner.ListReleases(ctx, "--channels", "--version", "4.20")
		expectOcMirrorCommandSuccess(result, err)
		const headerCount int = 2 // Listing channels...\n\n
		compareLineOutput(result.Stdout, headerCount, []string{"stable-4.20", "candidate-4.20", "eus-4.20", "fast-4.20"})
	})

	DescribeTable("channel and architecture selection", func(specCtx SpecContext, channel, arches string, expected []string, expectedRequests []string) {
		data, err := os.ReadFile(filepath.Join(graphDataDir, "list-releases.json"))
		Expect(err).NotTo(HaveOccurred())
		var graphs map[string]json.RawMessage
		Expect(json.Unmarshal(data, &graphs)).To(Succeed())
		var mu sync.Mutex
		requested := make(map[string]struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.URL.Query().Get("channel") + "/" + r.URL.Query().Get("arch")
			mu.Lock()
			requested[key] = struct{}{}
			mu.Unlock()
			graph, ok := graphs[key]
			if !ok || r.Header.Get("Accept") != "application/json" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(graph)
		}))
		DeferCleanup(server.Close)
		command := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).WithEnv([]string{"UPDATE_URL_OVERRIDE=" + server.URL})
		result, err := command.ListReleases(specCtx, "--channel", channel, "--filter-by-archs", arches)
		expectOcMirrorCommandSuccess(result, err)
		Expect(strings.TrimSpace(result.Stdout)).To(Equal(strings.Join(expected, "\n")))
		mu.Lock()
		actual := slices.Collect(maps.Keys(requested))
		mu.Unlock()
		Expect(actual).To(ConsistOf(expectedRequests))
	},
		Entry("selects a non-default channel", SpecTimeout(time.Minute), "candidate-4.20", "amd64",
			[]string{"Channel: candidate-4.20", "Architecture: amd64", "4.20.3"}, []string{"candidate-4.20/amd64"}),
		Entry("selects a single architecture", SpecTimeout(time.Minute), "fast-4.20", "arm64",
			[]string{"Channel: fast-4.20", "Architecture: arm64", "4.20.2"}, []string{"fast-4.20/arm64"}),
		Entry("keeps results distinct for multiple architectures", SpecTimeout(time.Minute), "fast-4.20", "amd64,arm64",
			[]string{"Channel: fast-4.20", "Architecture: amd64", "4.20.1", "Channel: fast-4.20", "Architecture: arm64", "4.20.2"},
			[]string{"fast-4.20/amd64", "fast-4.20/arm64"}),
	)
})

// Parse indexes of columns starts based on header
func parseColumnIndexesFromHeader(headerFields []string, header string) []int {
	GinkgoHelper()
	indexes := make([]int, 0, len(headerFields))
	for _, field := range headerFields {
		idx := strings.Index(header, field)
		Expect(idx).To(BeNumerically(">=", 0), "missing column %q in %q", field, header)
		indexes = append(indexes, idx)
	}
	return indexes
}

func parseFields(headerIndexes []int, line string) []string {
	GinkgoHelper()
	nIndexes := len(headerIndexes)
	Expect(nIndexes).To(BeNumerically(">", 0))
	Expect(len(line)).To(BeNumerically(">=", headerIndexes[nIndexes-1]), "incomplete table row %q", line)
	fields := make([]string, 0, nIndexes)
	for i, idx := range headerIndexes[:nIndexes-1] {
		nextIdx := headerIndexes[i+1]
		fields = append(fields, strings.TrimSpace(line[idx:nextIdx]))
	}
	fields = append(fields, strings.TrimSpace(line[headerIndexes[nIndexes-1]:]))
	return fields
}

func compareTableOutput(headerFields []string, result string, expected [][]string) {
	GinkgoHelper()
	lines := slices.Collect(strings.Lines(result))
	Expect(lines).To(HaveLen(len(expected)+1), "unexpected table rows in:\n%s", result)
	indexes := parseColumnIndexesFromHeader(headerFields, lines[0])
	for idx, line := range lines[1:] { // skip header
		fields := parseFields(indexes, line)
		Expect(fields).Should(HaveExactElements(expected[idx]), "line %d does not match", idx)
	}
}

func compareLineOutput(result string, skipHeadersCount int, expected []string) {
	GinkgoHelper()
	lines := make([]string, 0, len(result))
	for line := range strings.Lines(result) {
		if skipHeadersCount > 0 {
			skipHeadersCount--
			continue
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	Expect(lines).To(ConsistOf(expected), "mismatched elements")
}

func skipListTest() bool {
	return isOCMirrorVersionBefore(4, 22)
}
