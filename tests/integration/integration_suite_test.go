package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

var (
	testRegistry   *registry.Registry
	runner         *ocmirror.Runner
	registryConfig string
	iscDir         string
	keysDir        string
	policiesDir    string
	cacheDir       string
	graphDataDir   string
	ctx            context.Context
	cancel         context.CancelFunc
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "oc-mirror integration tests")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithTimeout(context.Background(), suiteTimeout())

	// AfterEach removes the default cache using this process's HOME, not the
	// runner's environment. Never let suite cleanup reach a developer's cache.
	home := GinkgoT().TempDir()
	for key, value := range map[string]string{
		"HOME":            home,
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"XDG_CACHE_HOME":  filepath.Join(home, ".cache"),
		"XDG_RUNTIME_DIR": filepath.Join(home, "run"),
		"DOCKER_CONFIG":   filepath.Join(home, ".docker"),
		"OC_MIRROR_CACHE": "",
	} {
		GinkgoT().Setenv(key, value)
	}
	// Distribution interprets REGISTRY_* variables as its own configuration,
	// including an empty REGISTRY_AUTH_FILE. Remove it, but restore it on cleanup.
	GinkgoT().Setenv("REGISTRY_AUTH_FILE", "")
	Expect(os.Unsetenv("REGISTRY_AUTH_FILE")).To(Succeed())
	Expect(os.MkdirAll(filepath.Join(home, "run"), 0o700)).To(Succeed())

	artifactsDir := os.Getenv("ARTIFACTS_DIR")
	if artifactsDir == "" {
		artifactsDir = "testdata"
	}

	registryConfig = filepath.Join(artifactsDir, "registry-config.yaml")
	iscDir = filepath.Join(artifactsDir, "imagesetconfigs")
	graphDataDir = filepath.Join(artifactsDir, "graphdatas")
	keysDir = filepath.Join(artifactsDir, "keys")
	policiesDir = filepath.Join(artifactsDir, "policies")

	os.Setenv("OCP_SIGNATURE_VERIFICATION_PK", filepath.Join(keysDir, "release-pk.asc"))

	runner = ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY"))
})

var _ = BeforeEach(func() {
	// Reset cache dir to default before each test.
	// Individual tests can override this and pass --cache-dir to the runner commands
	cacheDir = defaultCacheDir()

	// Start a fresh registry for each test
	var err error
	testRegistry, err = registry.Start(ctx, registryConfig, 5000)
	Expect(err).NotTo(HaveOccurred(), "Failed to start registry")

	err = testRegistry.WaitReady(ctx, 30*time.Second)
	Expect(err).NotTo(HaveOccurred(), "Registry not ready")
})

var _ = AfterEach(func() {
	// Stop the registry and clean up storage
	if testRegistry != nil {
		if err := testRegistry.Stop(); err != nil {
			GinkgoWriter.Printf("Failed to stop registry: %v\n", err)
		}
	}

	// Clean up the oc-mirror cache
	if err := os.RemoveAll(cacheDir); err != nil {
		GinkgoWriter.Printf("Failed to clean cache dir %s: %v\n", cacheDir, err)
	}
})

var _ = AfterSuite(func() {
	if cancel != nil {
		cancel()
	}
})

func suiteTimeout() time.Duration {
	if v := os.Getenv("TEST_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 30 * time.Minute
}
