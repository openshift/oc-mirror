package integration_test

import (
	"os"
	"path/filepath"
	"time"

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
		proxyRunner := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).WithEnv([]string{
			"HTTP_PROXY=" + fwdProxy.URL(),
			"HTTPS_PROXY=" + fwdProxy.URL(),
			"http_proxy=" + fwdProxy.URL(),
			"https_proxy=" + fwdProxy.URL(),
		})

		By("running mirrorToDisk with HTTP_PROXY/HTTPS_PROXY set")
		result, err := proxyRunner.MirrorToDisk(ctx, filepath.Join(iscDir, iscHappyPath), workDir)
		expectOcMirrorCommandSuccess(result, err)

		By("verifying images are mirrored in the local cache")
		expectSuccessfulMirrorInLocalCache(filepath.Join(iscDir, iscHappyPath), cacheDir)

		By("verifying the proxy observed connections to the expected external hosts")
		GinkgoWriter.Printf("hosts seen by proxy: %v\n", fwdProxy.Hosts())
		Expect(fwdProxy.SawHost("quay.io")).To(BeTrue(),
			"expected the proxy to observe a connection to quay.io (release/catalog/additional images); hosts seen: %v", fwdProxy.Hosts())
		Expect(fwdProxy.SawHost("stefanprodan.github.io")).To(BeTrue(),
			"expected the proxy to observe a connection to stefanprodan.github.io (helm repository index/chart); hosts seen: %v", fwdProxy.Hosts())
		Expect(fwdProxy.SawHost("ghcr.io")).To(BeTrue(),
			"expected the proxy to observe a connection to ghcr.io (image referenced by the helm chart); hosts seen: %v", fwdProxy.Hosts())
	})

	It("should fail when the configured proxy is unreachable", func() {
		unreachableAddr, err := proxy.UnusedAddr()
		Expect(err).NotTo(HaveOccurred())

		proxyRunner := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).WithEnv([]string{
			"HTTP_PROXY=http://" + unreachableAddr,
			"HTTPS_PROXY=http://" + unreachableAddr,
			"http_proxy=http://" + unreachableAddr,
			"https_proxy=http://" + unreachableAddr,
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
		})

		By("running mirrorToDisk with graph+channels and an unreachable proxy configured")
		result, err := proxyRunner.MirrorToDisk(ctx, filepath.Join(iscDir, iscProxyChannels), workDir)
		expectOcMirrorCommandFailure(result, err)

		By("verifying oc-mirror exits promptly instead of hanging until the Cincinnati request times out")
		Expect(result.Duration).To(BeNumerically("<", 30*time.Second),
			"oc-mirror took %s to fail; expected it to exit promptly instead of hanging", result.Duration)

		Expect(result.Stdout+result.Stderr).To(ContainSubstring("no release images found"),
			"expected a clear release-collection error, got:\nstdout: %s\nstderr: %s", result.Stdout, result.Stderr)
	})
})
