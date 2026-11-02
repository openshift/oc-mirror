package integration_test

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
	"github.com/openshift/oc-mirror/tests/integration/pkg/proxy"
)

var _ = Describe("mirroring through an HTTP(S) proxy", func() {
	var workDir string
	iscHappyPath := filepath.Join("happy_path", "isc-happy-path.yaml")

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	It("should route all mirroring traffic through the configured proxy and succeed", func() {
		fwdProxy, err := proxy.Start()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(fwdProxy.Stop)

		// Custom runner so we don't mutate the shared global runner's env.
		// Clear NO_PROXY/no_proxy so an inherited value can't bypass the proxy.
		proxyRunner := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).WithEnv([]string{
			"HTTP_PROXY=" + fwdProxy.URL(),
			"HTTPS_PROXY=" + fwdProxy.URL(),
			"http_proxy=" + fwdProxy.URL(),
			"https_proxy=" + fwdProxy.URL(),
			"NO_PROXY=",
			"no_proxy=",
		})

		By("running mirrorToDisk with HTTP_PROXY/HTTPS_PROXY set")
		result, err := proxyRunner.MirrorToDisk(ctx, filepath.Join(iscDir, iscHappyPath), workDir)
		expectOcMirrorCommandSuccess(result, err)

		// Check mirroring traffic before expected-digest lookups add their own proxy connections.
		By("verifying the proxy observed connections to the expected external hosts")
		GinkgoWriter.Printf("hosts seen by proxy: %v\n", fwdProxy.Hosts())
		Expect(fwdProxy.SawHost("quay.io")).To(BeTrue(),
			"expected the proxy to observe a connection to quay.io (release/catalog/additional images); hosts seen: %v", fwdProxy.Hosts())
		Expect(fwdProxy.SawHost("stefanprodan.github.io")).To(BeTrue(),
			"expected the proxy to observe a connection to stefanprodan.github.io (helm repository index/chart); hosts seen: %v", fwdProxy.Hosts())
		Expect(fwdProxy.SawHost("ghcr.io")).To(BeTrue(),
			"expected the proxy to observe a connection to ghcr.io (image referenced by the helm chart); hosts seen: %v", fwdProxy.Hosts())

		By("verifying images are mirrored in the local cache, resolving expected digests through the same proxy")
		proxyURL, err := url.Parse(fwdProxy.URL())
		Expect(err).NotTo(HaveOccurred(), "failed to parse the test proxy URL")
		transport := remote.DefaultTransport.(*http.Transport).Clone()
		DeferCleanup(transport.CloseIdleConnections)
		var digestProxyRequests atomic.Int32
		transport.Proxy = func(*http.Request) (*url.URL, error) {
			digestProxyRequests.Add(1)
			return proxyURL, nil
		}
		originalTransport := sourceDigestTransport
		sourceDigestTransport = transport
		DeferCleanup(func() { sourceDigestTransport = originalTransport })

		expectLocalCacheContainsExactly(cacheDir, happyPathCachedImages, testCatalogRepo)
		Expect(digestProxyRequests.Load()).To(BeNumerically(">", 0),
			"expected source-digest lookups to use the test proxy")

		By("verifying the proxy itself did not fail unexpectedly while serving connections")
		Expect(fwdProxy.Err()).NotTo(HaveOccurred())
	})

	It("should fail when the configured proxy is unreachable", func() {
		unreachableAddr, err := proxy.UnusedAddr()
		Expect(err).NotTo(HaveOccurred())

		proxyRunner := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).WithEnv([]string{
			"HTTP_PROXY=http://" + unreachableAddr,
			"HTTPS_PROXY=http://" + unreachableAddr,
			"http_proxy=http://" + unreachableAddr,
			"https_proxy=http://" + unreachableAddr,
			"NO_PROXY=",
			"no_proxy=",
		})

		By("running mirrorToDisk with an unreachable proxy configured")
		result, err := proxyRunner.MirrorToDisk(ctx, filepath.Join(iscDir, iscHappyPath), workDir, "--retry-times=1")
		expectOcMirrorCommandFailure(result, err)

		output := result.Stdout + result.Stderr
		Expect(output).To(SatisfyAny(
			ContainSubstring("proxyconnect"),
			ContainSubstring("connection refused"),
		), "expected failure to be related to the unreachable proxy, got:\nstdout: %s\nstderr: %s", result.Stdout, result.Stderr)
	})

	It("should exit promptly instead of hanging when release channel resolution can't reach an unreachable proxy", func() {
		// Channel resolution has a 60-minute timeout and no retries, so this
		// checks oc-mirror fails fast instead of hanging on a dead proxy.
		iscProxyChannels := filepath.Join("proxy", "isc-proxy-channels.yaml")

		unreachableAddr, err := proxy.UnusedAddr()
		Expect(err).NotTo(HaveOccurred())

		proxyRunner := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).WithEnv([]string{
			"HTTP_PROXY=http://" + unreachableAddr,
			"HTTPS_PROXY=http://" + unreachableAddr,
			"http_proxy=http://" + unreachableAddr,
			"https_proxy=http://" + unreachableAddr,
			"NO_PROXY=",
			"no_proxy=",
		})

		// Bound this command so a regression of the 60-minute timeout fails
		// fast instead of burning the suite's time budget.
		commandCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		DeferCleanup(cancel)

		By("running mirrorToDisk with graph+channels and an unreachable proxy configured")
		result, err := proxyRunner.MirrorToDisk(commandCtx, filepath.Join(iscDir, iscProxyChannels), workDir)
		expectOcMirrorCommandFailure(result, err)

		By("verifying oc-mirror exits promptly instead of hanging until the Cincinnati request times out")
		Expect(result.Duration).To(BeNumerically("<", 30*time.Second),
			"oc-mirror took %s to fail; expected it to exit promptly instead of hanging", result.Duration)

		Expect(result.Stdout+result.Stderr).To(ContainSubstring("no release images found"),
			"expected a clear release-collection error, got:\nstdout: %s\nstderr: %s", result.Stdout, result.Stderr)
	})
})
