package integration_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// startReleaseChannelServer serves the signed test payload through Cincinnati.
func startReleaseChannelServer() *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"nodes":[{"version":"4.18.1","payload":"quay.io/oc-mirror/release/test-release-index@sha256:f81792339c8b5934191d18a53b18bc1d584e01a9f37d59c0aa6905b00200aa1b"}],"edges":[]}`)
	}))
	DeferCleanup(server.Close)
	return server
}

func registryImageDigest(imageRef string) string {
	GinkgoHelper()
	ref, err := name.ParseReference(imageRef, name.Insecure)
	Expect(err).NotTo(HaveOccurred())
	desc, err := remote.Get(ref, remote.WithContext(ctx))
	Expect(err).NotTo(HaveOccurred())
	return desc.Digest.String()
}

func mappingDestinations(workDir string) []string {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(workDir, dirWorkingDir, "dry-run", "mapping.txt"))
	Expect(err).NotTo(HaveOccurred())
	Expect(strings.TrimSpace(string(data))).NotTo(BeEmpty())
	var destinations []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		source, destination, found := strings.Cut(line, "=")
		Expect(found).To(BeTrue(), "invalid mapping line: %s", line)
		Expect(source).NotTo(BeEmpty())
		Expect(destination).NotTo(BeEmpty())
		destinations = append(destinations, destination)
	}
	return destinations
}
