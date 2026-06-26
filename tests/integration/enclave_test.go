package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

// A registries.conf redirect makes the enclave-side oc-mirror pull from the
// intermediate registry instead of the original remote source.
var _ = Describe("enclave", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	Describe("mirror-to-mirror with registries.conf redirect", func() {
		iscFile := filepath.Join("operators", "isc-operator-version-range.yaml")

		// Workflow:
		//  1. Mirror operators from the remote catalog into an intermediate registry.
		//  2. Configure registries.conf to redirect quay.io pulls to the intermediate registry.
		//  3. Mirror from the intermediate registry into a separate enclave registry.
		//  4. Assert that the enclave catalog contains the correctly filtered bundles.
		It("should forward operators from intermediate to enclave registry via registries.conf", func() {
			iscPath := filepath.Join(iscDir, iscFile)
			GinkgoWriter.Printf("ISC path: %s\n", iscPath)
			GinkgoWriter.Printf("workDir: %s\n", workDir)

			By("starting a second registry for the enclave destination")
			enclaveRegistry, err := registry.Start(ctx, registryConfig, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(enclaveRegistry.WaitReady(ctx, 30*time.Second)).To(Succeed())
			defer enclaveRegistry.Stop()
			GinkgoWriter.Printf("intermediate registry: %s\n", testRegistry.Endpoint())
			GinkgoWriter.Printf("enclave registry:      %s\n", enclaveRegistry.Endpoint())

			By("mirroring from the remote catalog to the intermediate registry")
			result, err := runner.MirrorToMirror(ctx, iscPath, workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			logOcMirrorResult("step-1 mirrorToMirror", result)
			expectOcMirrorCommandSuccess(result, err)

			By("verifying operator images exist in the intermediate registry")
			logRegistryRepositories("intermediate", *testRegistry)
			expectSuccessfulMirrorInRegistry(iscPath, *testRegistry)

			By("mirroring from the intermediate registry to the enclave registry using registries.conf")
			enclaveWorkDir := setupWorkDir()
			defer cleanupWorkDir(enclaveWorkDir)

			registriesConf := writeRegistriesConf(enclaveWorkDir, testRegistry.Endpoint())
			GinkgoWriter.Printf("registries.conf: %s\n", registriesConf)
			GinkgoWriter.Printf("enclave workDir: %s\n", enclaveWorkDir)

			enclaveRunner := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).
				WithEnv([]string{"CONTAINERS_REGISTRIES_CONF=" + registriesConf})

			result, err = enclaveRunner.MirrorToMirror(ctx, iscPath, enclaveWorkDir, enclaveRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false", "--src-tls-verify=false")
			logOcMirrorResult("step-2 mirrorToMirror", result)
			expectOcMirrorCommandSuccess(result, err)

			By("verifying operator images exist in the enclave registry")
			logRegistryRepositories("enclave", *enclaveRegistry)
			expectSuccessfulMirrorInRegistry(iscPath, *enclaveRegistry)

			By("verifying the enclave catalog contains the correctly filtered bundles")
			expectCatalogBundlesMatchISC(ctx, *enclaveRegistry, iscPath, map[string][]string{
				"foo": {"foo.v0.2.0", "foo.v0.3.0", "foo.v0.3.1"},
			})
		})
	})
})

// writeRegistriesConf generates a containers registries.conf file that
// redirects all quay.io pulls to the given mirror endpoint over plain HTTP.
func writeRegistriesConf(dir, mirrorEndpoint string) string {
	content := fmt.Sprintf(`[[registry]]
  location = "quay.io"
  [[registry.mirror]]
    location = "%s"
    insecure = true
`, mirrorEndpoint)

	p := filepath.Join(dir, "registries.conf")
	Expect(os.WriteFile(p, []byte(content), 0o644)).To(Succeed())
	return p
}
