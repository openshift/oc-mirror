package integration_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
)

// OCP-75219 - diskToMirror should not require network access to source registries.
//
// The d2m workflow by design reads all image sources from the local cache (localhost); it never
// contacts source registries. As an extra safety net, HTTP_PROXY/HTTPS_PROXY are pointed at an
// unreachable address so any accidental outbound call fails immediately. This works because Go's
// net/http stack (used by both oc-mirror and containers/image) universally honours proxy env vars
// via http.ProxyFromEnvironment. Retries are capped at 1 (--retry-times=1) to avoid exponential
// backoff delays in case a call does hit the blocked proxy.
var _ = Describe("diskToMirror without network access", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	It("should mirror successfully from a local archive when outbound network access is blocked", func() {
		iscPath := filepath.Join(iscDir, "secure_policy", "isc-operators-only.yaml")

		By("running mirrorToDisk with network access")
		result, err := runner.MirrorToDisk(ctx, iscPath, workDir)
		expectOcMirrorCommandSuccess(result, err)

		By("removing the local cache to simulate a clean disk-to-mirror environment")
		Expect(os.RemoveAll(cacheDir)).To(Succeed())

		By("running diskToMirror with outbound network access blocked")
		offlineRunner := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).
			WithEnv([]string{
				"HTTP_PROXY=http://127.0.0.1:1",
				"HTTPS_PROXY=http://127.0.0.1:1",
				"NO_PROXY=localhost,127.0.0.1",
			})
		result, err = offlineRunner.DiskToMirror(ctx, iscPath, workDir, testRegistry.Endpoint(),
			"--dest-tls-verify=false", "--retry-times=1")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying images were mirrored to the destination registry")
		expectSuccessfulMirrorInRegistry(iscPath, *testRegistry)
	})

	// OCPBUGS-136389: a full catalog (full: true, no packages) is never rebuilt, so
	// diskToMirror must read it from the archive instead of its source registry.
	It("should mirror a full catalog from a local archive when outbound network access is blocked", func() {
		iscPath := filepath.Join(iscDir, "operators", "isc-operator-full-catalog.yaml")
		cfg := parseImageSetConfig(iscPath)

		By("resolving expected operator images independently from the source catalog")
		expectedImages := sourceCatalogImageDigests(ctx, cfg.Mirror.Operators[0].Catalog)

		By("running mirrorToDisk with network access")
		result, err := runner.MirrorToDisk(ctx, iscPath, workDir)
		expectOcMirrorCommandSuccess(result, err)

		By("removing the local cache to simulate a clean disk-to-mirror environment")
		Expect(os.RemoveAll(cacheDir)).To(Succeed(), "failed to remove the local cache %s", cacheDir)

		By("removing the working-dir left by mirrorToDisk so that diskToMirror can only use the mirror archive")
		m2dWorkingDir := filepath.Join(workDir, dirWorkingDir)
		Expect(filepath.Join(m2dWorkingDir, dirOperatorCatalog)).To(BeADirectory(),
			"mirrorToDisk should have stored the operator catalogs in its working-dir %s", m2dWorkingDir)
		archives, err := filepath.Glob(filepath.Join(workDir, "mirror_*.tar"))
		Expect(err).NotTo(HaveOccurred())
		Expect(archives).NotTo(BeEmpty(), "mirrorToDisk should have written mirror archives to %s", workDir)
		Expect(os.RemoveAll(m2dWorkingDir)).To(Succeed(), "failed to remove the mirrorToDisk working-dir %s", m2dWorkingDir)
		Expect(m2dWorkingDir).NotTo(BeAnExistingFile(), "the mirrorToDisk working-dir %s must not exist before diskToMirror", m2dWorkingDir)
		for _, archive := range archives {
			Expect(archive).To(BeARegularFile(), "mirror archive %s must still be present for diskToMirror", archive)
		}

		By("running diskToMirror with outbound network access blocked")
		offlineRunner := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).
			WithEnv([]string{
				"HTTP_PROXY=http://127.0.0.1:1",
				"HTTPS_PROXY=http://127.0.0.1:1",
				"NO_PROXY=localhost,127.0.0.1",
			})
		result, err = offlineRunner.DiskToMirror(ctx, iscPath, workDir, testRegistry.Endpoint(),
			"--dest-tls-verify=false", "--retry-times=1")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying diskToMirror rebuilt the working-dir from the mirror archive")
		Expect(filepath.Join(m2dWorkingDir, dirOperatorCatalog)).To(BeADirectory(),
			"diskToMirror should have extracted the operator catalogs from the mirror archive into %s", m2dWorkingDir)

		By("verifying the catalog and every bundle and related image were mirrored to the destination registry")
		for _, ref := range expectedImages {
			expectImageReadableByDigest(ctx, *testRegistry, ref.Context().RepositoryStr(), ref.DigestStr())
		}
	})
})
