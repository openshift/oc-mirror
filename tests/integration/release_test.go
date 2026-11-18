package integration_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
)

var _ = DescribeTable("cross-major release discovery with minVersion and maxVersion", func(hasPath bool) {
	workDir := setupWorkDir()
	isc := filepath.Join(iscDir, "releases", "isc-cross-major.yaml")

	By("serving the source and target channel graphs locally")
	type node struct {
		Version string `json:"version"`
		Payload string `json:"payload"`
	}
	type graph struct {
		Nodes []node   `json:"nodes"`
		Edges [][2]int `json:"edges"`
	}
	// Use a different signed payload for the out-of-range releases so they are visible in mapping.txt.
	nodes := []node{
		{"4.22.18", "quay.io/oc-mirror/release/test-release-index-multi@sha256:2352c020dacb945a96cf665e553644f3a3c39341b636a4d9ef1653944fd9de1f"},
		{"4.22.19", "quay.io/oc-mirror/release/test-release-index@sha256:f81792339c8b5934191d18a53b18bc1d584e01a9f37d59c0aa6905b00200aa1b"},
		{"4.22.20", "quay.io/oc-mirror/release/test-release-index@sha256:f81792339c8b5934191d18a53b18bc1d584e01a9f37d59c0aa6905b00200aa1b"},
		{"4.22.21", "quay.io/oc-mirror/release/test-release-index-multi@sha256:2352c020dacb945a96cf665e553644f3a3c39341b636a4d9ef1653944fd9de1f"},
		{"5.0.0", "quay.io/oc-mirror/release/test-release-index@sha256:f81792339c8b5934191d18a53b18bc1d584e01a9f37d59c0aa6905b00200aa1b"},
	}
	target := graph{Nodes: nodes[1:], Edges: [][2]int{{0, 1}, {1, 2}}}
	if hasPath {
		// Only the target graph can connect 4.22.21 to 5.0.0.
		target.Edges = append(target.Edges, [2]int{2, 3})
	}
	graphs := map[string]graph{
		"candidate-4.22": {Nodes: nodes[:4], Edges: [][2]int{{0, 1}, {1, 2}, {2, 3}}},
		"candidate-5.0":  target,
	}
	cincinnati := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g, ok := graphs[r.URL.Query().Get("channel")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(g)
	}))
	DeferCleanup(cincinnati.Close)

	By("running mirror-to-disk in dry-run mode to get the copy list")
	command := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).WithEnv([]string{"UPDATE_URL_OVERRIDE=" + cincinnati.URL})
	result, err := command.MirrorToDisk(ctx, isc, workDir, "--dry-run", "--remove-signatures=true")
	expectOcMirrorCommandSuccess(result, err)

	By("checking that the copy list includes the out-of-range payload only when it is needed for the upgrade")
	mapping, err := os.ReadFile(filepath.Join(workDir, dirWorkingDir, "dry-run", mappingFileName))
	Expect(err).NotTo(HaveOccurred())
	var releases []string
	for line := range strings.SplitSeq(string(mapping), "\n") {
		source, destination, _ := strings.Cut(line, "=")
		if strings.Contains(destination, "/openshift/release-images:") {
			releases = append(releases, source)
		}
	}
	// Signature verification resolves the digest references to the signed v0.0.1 tags.
	expected := []string{dockerTransportPrefix + "quay.io/oc-mirror/release/test-release-index" + ":v0.0.1"}
	output := result.Stdout + result.Stderr
	if hasPath {
		expected = append(expected, dockerTransportPrefix+"quay.io/oc-mirror/release/test-release-index-multi"+":v0.0.1")
		Expect(output).NotTo(ContainSubstring("NoUpgradePath:"))
	} else {
		Expect(output).To(ContainSubstring("NoUpgradePath:"))
	}
	Expect(releases).To(ConsistOf(expected))
},
	Entry("adds the intermediate release from the target graph", true),
	Entry("warns and keeps only the configured releases when the cross-major edge is absent", false),
)
