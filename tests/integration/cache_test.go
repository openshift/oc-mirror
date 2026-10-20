package integration_test

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
)

// fooOperandDigest is the foo operand, a related image of the foo.v0.3.1 bundle
// in test-catalog-latest. It has no stable tag, so its digest is pinned here.
const fooOperandDigest = "sha256:1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58"

var _ = Describe("cache and workspace configuration", func() {
	var workDir, operatorISC, port string

	BeforeEach(func() {
		workDir = GinkgoT().TempDir()
		var err error
		operatorISC, err = filepath.Abs(filepath.Join(iscDir, "operators", "isc-operator-pinned-version.yaml"))
		Expect(err).NotTo(HaveOccurred())
		// Find a free port for oc-mirror's local cache registry.
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		port = fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
		Expect(listener.Close()).To(Succeed())
	})

	cacheRunner := func(base string, useEnv bool, env ...string) (*ocmirror.Runner, []string) {
		command := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).WithDir(workDir)
		args := []string{"--remove-signatures", "-p", port}
		if useEnv {
			env = append(env, "OC_MIRROR_CACHE="+base)
		} else {
			args = append(args, "--cache-dir", base)
		}
		return command.WithEnv(env), args
	}

	It("honors --cache-dir in m2m", SpecTimeout(3*time.Minute), func(specCtx SpecContext) {
		defaultCache := defaultCacheDir()
		Expect(os.MkdirAll(defaultCache, 0o700)).To(Succeed())
		sentinel := filepath.Join(defaultCache, "untouched")
		Expect(os.WriteFile(sentinel, []byte("default cache control"), 0o600)).To(Succeed())

		base := filepath.Join(workDir, "custom-cache")
		effectiveCache := filepath.Join(base, ".oc-mirror", ".cache")
		command, cacheArgs := cacheRunner(base, false)
		workspace := filepath.Join(workDir, "workspace")
		const repo = "oc-mirror/oc-mirror-dev"
		ref, err := name.NewTag("quay.io/" + repo + ":foo-bundle-v0.3.1")
		Expect(err).NotTo(HaveOccurred())
		bundle, err := remote.Head(ref, remote.WithAuth(authn.Anonymous), remote.WithContext(specCtx))
		Expect(err).NotTo(HaveOccurred())

		By("mirroring our published catalog using the short config and port flags")
		args := []string{"--v2", "-c", operatorISC, "--workspace", "file://" + workspace,
			"docker://" + testRegistry.Endpoint(), "--dest-tls-verify=false"}
		result, err := command.Run(specCtx, append(args, cacheArgs...)...)
		logOcMirrorResult("custom cache m2m", result)
		expectOcMirrorCommandSuccess(result, err)

		By("verifying cached manifest bytes and the mirrored catalog, bundle, and related image")
		cacheDigest := localCacheTagDigest(effectiveCache, repo, "test-catalog-latest")
		expectImageInLocalCache(effectiveCache, ref.Context().Digest(cacheDigest))
		expectCatalogBundlesMatchISC(specCtx, *testRegistry, operatorISC, map[string][]string{"foo": {"foo.v0.3.1"}})
		expectCatalogContainsOnlyExpectedPackages(specCtx, *testRegistry, operatorISC)
		expectImageReadableByDigest(specCtx, *testRegistry, repo, bundle.Digest.String())
		expectImageReadableByDigest(specCtx, *testRegistry, repo, fooOperandDigest)

		By("verifying the requested workspace was used and the default cache was not changed")
		Expect(filepath.Join(workspace, "working-dir", "cluster-resources")).To(BeADirectory())
		Expect(filepath.Join(workDir, "working-dir")).NotTo(BeAnExistingFile())
		entries, err := os.ReadDir(defaultCache)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		data, err := os.ReadFile(sentinel)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("default cache control"))
	})

	It("honors OC_MIRROR_CACHE in m2d and d2m", SpecTimeout(3*time.Minute), func(specCtx SpecContext) {
		defaultCache := defaultCacheDir()
		Expect(os.MkdirAll(defaultCache, 0o700)).To(Succeed())
		sentinel := filepath.Join(defaultCache, "untouched")
		Expect(os.WriteFile(sentinel, []byte("default cache control"), 0o600)).To(Succeed())

		base := filepath.Join(workDir, "custom-cache")
		effectiveCache := filepath.Join(base, ".oc-mirror", ".cache")
		command, cacheArgs := cacheRunner(base, true)
		const repo = "oc-mirror/oc-mirror-dev"
		ref, err := name.NewTag("quay.io/" + repo + ":foo-bundle-v0.3.1")
		Expect(err).NotTo(HaveOccurred())
		bundle, err := remote.Head(ref, remote.WithAuth(authn.Anonymous), remote.WithContext(specCtx))
		Expect(err).NotTo(HaveOccurred())

		By("mirroring into the custom cache and creating transport archives")
		transport := filepath.Join(workDir, "transport")
		result, err := command.MirrorToDisk(specCtx, operatorISC, transport, cacheArgs...)
		logOcMirrorResult("custom cache m2d", result)
		expectOcMirrorCommandSuccess(result, err)
		expectSuccessfulMirrorInLocalCache(operatorISC, effectiveCache)
		expectNoRepositoriesInRegistry(*testRegistry)
		expectCorrectTarArchiveContents(operatorISC, transport)
		expectImageInLocalCache(effectiveCache, ref.Context().Digest(bundle.Digest.String()))

		By("removing the original cache and moving the transport directory before restoring")
		Expect(os.RemoveAll(base)).To(Succeed())
		Expect(base).NotTo(BeAnExistingFile())
		workspace := filepath.Join(workDir, "relocated-transport")
		Expect(os.Rename(transport, workspace)).To(Succeed())
		restoreBase := filepath.Join(workDir, "restore-cache")
		Expect(restoreBase).NotTo(BeAnExistingFile())
		restoreCommand, restoreArgs := cacheRunner(restoreBase, true,
			"HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1",
			"NO_PROXY=localhost,127.0.0.1")
		restoreArgs = append(restoreArgs, "--dest-tls-verify=false", "--retry-times=1")
		result, err = restoreCommand.DiskToMirror(specCtx, operatorISC, workspace, testRegistry.Endpoint(), restoreArgs...)
		logOcMirrorResult("custom cache d2m", result)
		expectOcMirrorCommandSuccess(result, err)
		expectImageInLocalCache(filepath.Join(restoreBase, ".oc-mirror", ".cache"), ref.Context().Digest(bundle.Digest.String()))

		By("verifying the mirrored catalog, bundle, and related image are complete")
		expectCatalogBundlesMatchISC(specCtx, *testRegistry, operatorISC, map[string][]string{"foo": {"foo.v0.3.1"}})
		expectCatalogContainsOnlyExpectedPackages(specCtx, *testRegistry, operatorISC)
		expectImageReadableByDigest(specCtx, *testRegistry, repo, bundle.Digest.String())
		expectImageReadableByDigest(specCtx, *testRegistry, repo, fooOperandDigest)

		By("verifying the requested workspace was used and the default cache was not changed")
		Expect(filepath.Join(workspace, "working-dir", "cluster-resources")).To(BeADirectory())
		Expect(filepath.Join(workDir, "working-dir")).NotTo(BeAnExistingFile())
		entries, err := os.ReadDir(defaultCache)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		data, err := os.ReadFile(sentinel)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("default cache control"))
	})

	It("rejects an occupied storage port before copying any images", SpecTimeout(time.Minute), func(specCtx SpecContext) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(listener.Close)
		port := fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
		command := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY"))
		workspace := filepath.Join(workDir, "workspace")
		result, err := command.MirrorToMirror(specCtx, operatorISC, workspace, testRegistry.Endpoint(),
			"--port", port, "--dest-tls-verify=false")
		expectOcMirrorExitCode(result, err, 1, port+" is already bound and cannot be used")
		expectNoRepositoriesInRegistry(*testRegistry)
		Expect(defaultCacheDir()).NotTo(BeAnExistingFile())
		Expect(filepath.Join(workspace, dirWorkingDir, dirClusterResources)).NotTo(BeAnExistingFile())
	})
})
