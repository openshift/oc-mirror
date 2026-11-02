package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/helmrepo"
	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
	"github.com/openshift/oc-mirror/tests/integration/pkg/testimages"
)

const (
	// helmRepositoryPort is the port of the local Helm chart repository, which
	// ImageSetConfigurations reference as http://localhost:5002.
	helmRepositoryPort = 5002
	// sourceRegistryAlias and helmRepositoryAlias are host names, other than
	// localhost, of the source registry and the Helm repository. They are for
	// tests that need traffic to go through a proxy, since Go never proxies
	// traffic to loopback addresses. Only the test proxy resolves them.
	sourceRegistryAlias = "source.oc-mirror.test"
	helmRepositoryAlias = "charts.oc-mirror.test"
)

var (
	// sourceRegistry serves every image the tests mirror from. It runs for the
	// whole session and holds the images built by testimages.BuildAndPush.
	sourceRegistry *registry.Registry
	// sourceImages resolves the digests of the images in sourceRegistry.
	sourceImages *testimages.Inventory
	// releaseSignaturesDir holds the GPG signatures of the release payloads in
	// sourceRegistry, which setupWorkDir copies into each working directory.
	releaseSignaturesDir string
	// helmRepository serves the charts in helmChartsDir.
	helmRepository *helmrepo.Server
	testRegistry   *registry.Registry
	runner         *ocmirror.Runner
	registryConfig string
	iscDir         string
	helmChartsDir  string
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
	graphDataDir = filepath.Join(artifactsDir, "graphdatas")
	keysDir = filepath.Join(artifactsDir, "keys")
	policiesDir = filepath.Join(artifactsDir, "policies")

	runner = ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY"))

	startSourceRegistry(artifactsDir)
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
	if helmRepository != nil {
		if err := helmRepository.Stop(); err != nil {
			GinkgoWriter.Printf("Failed to stop Helm repository: %v\n", err)
		}
	}
	if sourceRegistry != nil {
		if err := sourceRegistry.Stop(); err != nil {
			GinkgoWriter.Printf("Failed to stop source registry: %v\n", err)
		}
	}
	if cancel != nil {
		cancel()
	}
})

// startSourceRegistry starts the session-wide registry that serves every image
// the tests pull from, builds and pushes the images from testdata/images, and
// renders the ImageSetConfigurations and Helm charts so they can reference those
// images by digest. It also starts the Helm repository serving those charts.
//
// The registry serves HTTPS with a CA generated for the session, which oc-mirror
// trusts through the containers certs.d directory of the test HOME. This way tests
// pull from it with oc-mirror's default TLS verification, as from any remote registry.
func startSourceRegistry(artifactsDir string) {
	var err error
	sourceRegistry, err = registry.Start(ctx, registryConfig, testimages.SourceRegistryPort,
		registry.WithSelfSignedTLS(sourceRegistryAlias))
	Expect(err).NotTo(HaveOccurred(), "Failed to start source registry")
	Expect(sourceRegistry.WaitReady(ctx, 30*time.Second)).To(Succeed(), "Source registry not ready")

	caCert, err := os.ReadFile(sourceRegistry.CACertPath())
	Expect(err).NotTo(HaveOccurred())
	for _, host := range []string{sourceRegistry.Endpoint(), sourceRegistryAlias} {
		certsDir := filepath.Join(os.Getenv("HOME"), ".config", "containers", "certs.d", host)
		Expect(os.MkdirAll(certsDir, 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(certsDir, "ca.crt"), caCert, 0o600)).To(Succeed())
	}

	releaseSignaturesDir = filepath.Join(GinkgoT().TempDir(), "release-signatures")
	Expect(os.MkdirAll(releaseSignaturesDir, 0o755)).To(Succeed())
	releasePublicKey := filepath.Join(GinkgoT().TempDir(), "release-pk.asc")
	sourceImages, err = testimages.BuildAndPush(ctx, sourceRegistry, testimages.Options{
		ImagesDir:            filepath.Join(artifactsDir, "images"),
		CosignKeyPath:        filepath.Join(keysDir, "cosign.key"),
		ReleaseSignaturesDir: releaseSignaturesDir,
		ReleasePublicKeyPath: releasePublicKey,
	})
	Expect(err).NotTo(HaveOccurred(), "Failed to build test images")
	GinkgoT().Setenv("OCP_SIGNATURE_VERIFICATION_PK", releasePublicKey)

	iscDir = filepath.Join(GinkgoT().TempDir(), "imagesetconfigs")
	Expect(sourceImages.RenderDir(filepath.Join(artifactsDir, "imagesetconfigs"), iscDir)).
		To(Succeed(), "Failed to render ImageSetConfigurations")
	// Charts are rendered before Helm processes them, so their templates must not use
	// Helm's own {{ }} actions.
	helmChartsDir = filepath.Join(GinkgoT().TempDir(), "helm-charts")
	Expect(sourceImages.RenderDir(filepath.Join(artifactsDir, "helm-charts"), helmChartsDir)).
		To(Succeed(), "Failed to render Helm charts")
	helmRepository, err = helmrepo.Start(helmChartsDir, helmRepositoryPort)
	Expect(err).NotTo(HaveOccurred(), "Failed to start the Helm repository")
}

// sourceImageDigest returns the digest of a source image, identified by
// "<repository>:<tag>" (e.g. "oc-mirror/oc-mirror-dev:operand").
func sourceImageDigest(repoTag string) string {
	digest, err := sourceImages.Digest(repoTag)
	Expect(err).NotTo(HaveOccurred())
	return digest
}

// sourceImageRef returns the full reference of a source image, identified by
// "<repository>:<tag>" (e.g. "localhost:5001/oc-mirror/oc-mirror-dev:operand").
func sourceImageRef(repoTag string) string {
	ref, err := sourceImages.Ref(repoTag)
	Expect(err).NotTo(HaveOccurred())
	return ref
}

// sourceImageDigestRef returns the digest reference of a source image, identified by
// "<repository>:<tag>" (e.g. "localhost:5001/oc-mirror/oc-mirror-dev@sha256:...").
func sourceImageDigestRef(repoTag string) string {
	repo := repoTag[:strings.LastIndex(repoTag, ":")]
	return testimages.SourceRegistry + "/" + repo + "@" + sourceImageDigest(repoTag)
}

func suiteTimeout() time.Duration {
	if v := os.Getenv("TEST_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 30 * time.Minute
}
