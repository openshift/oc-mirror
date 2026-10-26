package integration_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

var _ = Describe("platform targetRepo workflow combinations", Label("platform-target-repo"), func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
		cacheDir = filepath.Join(workDir, "cache", ".oc-mirror", ".cache")
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	It("copies directly with signatures, repeats safely, and deletes without a populated image cache", func() {
		By("resolving a release channel through Cincinnati")
		server := startReleaseChannelServer()
		channelRunner := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).WithEnv([]string{"UPDATE_URL_OVERRIDE=" + server.URL})
		isc := filepath.Join(iscDir, "platform_target_repo", "isc-channel.yaml")
		repo := "team/openshift4/ocp-release"
		for attempt := 0; attempt < 2; attempt++ {
			result, err := channelRunner.MirrorToMirror(ctx, isc, workDir, testRegistry.Endpoint()+"/team",
				"--cache-dir", filepath.Join(workDir, "cache"), "--dest-tls-verify=false", "--max-nested-paths=3")
			expectOcMirrorCommandSuccess(result, err)
			tags, err := testRegistry.ListTags(ctx, repo)
			Expect(err).NotTo(HaveOccurred())
			Expect(tags).To(ContainElements("v0.0.1", "v0.0.1-alpine"))
			expectSignaturesInRegistry(*testRegistry)
			expectSignatureConfigMapGenerated(workDir)
		}

		By("generating deletion without release images in the local registry cache")
		Expect(localCacheHasTag(cacheDir, "openshift4/ocp-release", "v0.0.1")).To(BeFalse())
		result, err := runner.DeletePhaseOne(ctx, filepath.Join(iscDir, "platform_target_repo", "disc.yaml"), workDir, "direct", testRegistry.Endpoint()+"/team",
			"--cache-dir", filepath.Join(workDir, "cache"), "--dest-tls-verify=false", "--delete-signatures")
		expectOcMirrorCommandSuccess(result, err)
		result, err = runner.DeletePhaseTwo(ctx, filepath.Join(workDir, dirWorkingDir, "delete", "delete-images-direct.yaml"), testRegistry.Endpoint()+"/team",
			"--cache-dir", filepath.Join(workDir, "cache"), "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		expectEmptyRegistry(*testRegistry)
	})

	It("preserves signatures through a fresh-cache archive round trip", func() {
		isc := filepath.Join(iscDir, "platform_target_repo", "isc.yaml")
		result, err := runner.MirrorToDisk(ctx, isc, workDir, "--cache-dir", filepath.Join(workDir, "cache"))
		expectOcMirrorCommandSuccess(result, err)
		result, err = runner.DiskToMirror(ctx, isc, workDir, testRegistry.Endpoint(),
			"--cache-dir", filepath.Join(workDir, "restore-cache"), "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		expectSignaturesInRegistry(*testRegistry)
		expectSignatureConfigMapGenerated(workDir)
		repos, err := testRegistry.ListRepositories(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(repos).To(ConsistOf("openshift4/ocp-release"))
	})

	It("keeps incremental archives and dry-run mappings on the custom repository", func() {
		isc := filepath.Join(iscDir, "platform_target_repo", "isc.yaml")
		result, err := runner.MirrorToDisk(ctx, isc, workDir, "--cache-dir", filepath.Join(workDir, "cache"), "--remove-signatures=true")
		expectOcMirrorCommandSuccess(result, err)
		Expect(countBlobEntriesInArchives(findTarArchives(workDir))).To(BeNumerically(">", 0))
		cachedDigests := make(map[string]string)
		for _, tag := range []string{"v0.0.1", "v0.0.1-alpine"} {
			cachedDigests[tag] = localCacheTagDigest(cacheDir, "openshift4/ocp-release", tag)
		}
		restoreCache := filepath.Join(workDir, "restore-cache")
		result, err = runner.DiskToMirror(ctx, isc, workDir, testRegistry.Endpoint(), "--cache-dir", restoreCache, "--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("checking cached mirror-to-disk dry run resolves every image")
		result, err = runner.MirrorToDisk(ctx, isc, workDir, "--cache-dir", filepath.Join(workDir, "cache"), "--remove-signatures=true", "--dry-run")
		expectOcMirrorCommandSuccess(result, err)
		destinations := mappingDestinations(workDir)
		Expect(destinations).To(HaveLen(2))
		Expect(destinations).To(HaveEach(ContainSubstring("/openshift4/ocp-release:")))
		Expect(result.Stdout + result.Stderr).To(ContainSubstring("all 2 images required for mirroring are available"))

		By("creating a repeated archive without resending blobs")
		result, err = runner.MirrorToDisk(ctx, isc, workDir, "--cache-dir", filepath.Join(workDir, "cache"), "--remove-signatures=true")
		expectOcMirrorCommandSuccess(result, err)
		expectNoImageBlobsInArchives(findTarArchives(workDir))

		By("restoring the incremental archive on top of the first archive's cache")
		destination := testRegistry.Endpoint() + "/incremental"
		repositories, err := testRegistry.ListRepositories(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(repositories).NotTo(ContainElement("incremental/openshift4/ocp-release"))
		result, err = runner.DiskToMirror(ctx, isc, workDir, destination, "--cache-dir", restoreCache, "--remove-signatures=true", "--dest-tls-verify=false", "--dry-run")
		expectOcMirrorCommandSuccess(result, err)
		Expect(mappingDestinations(workDir)).To(ConsistOf(
			"docker://"+destination+"/openshift4/ocp-release:v0.0.1",
			"docker://"+destination+"/openshift4/ocp-release:v0.0.1-alpine",
		))
		result, err = runner.DiskToMirror(ctx, isc, workDir, destination, "--cache-dir", restoreCache, "--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		tags, err := testRegistry.ListTags(ctx, "incremental/openshift4/ocp-release")
		Expect(err).NotTo(HaveOccurred())
		Expect(tags).To(ConsistOf("v0.0.1", "v0.0.1-alpine"))
		for _, tag := range tags {
			Expect(registryImageDigest(destination + "/openshift4/ocp-release:" + tag)).To(Equal(cachedDigests[tag]))
		}
	})

	DescribeTable("preserves sparse multi-architecture payloads and components", func(archive bool) {
		sparseRegistry, err := registry.Start(ctx, filepath.Join(filepath.Dir(registryConfig), "registry-config-sparse.yaml"), 0)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(sparseRegistry.Stop()).To(Succeed()) })
		Expect(sparseRegistry.WaitReady(ctx, 30*time.Second)).To(Succeed())
		isc := filepath.Join(iscDir, "platform_target_repo", "isc-multi.yaml")
		if archive {
			result, err := runner.MirrorToDisk(ctx, isc, workDir, "--cache-dir", filepath.Join(workDir, "cache"), "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)
			result, err = runner.DiskToMirror(ctx, isc, workDir, sparseRegistry.Endpoint(), "--cache-dir", filepath.Join(workDir, "restore-cache"), "--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)
		} else {
			result, err := runner.MirrorToMirror(ctx, isc, workDir, sparseRegistry.Endpoint(), "--cache-dir", filepath.Join(workDir, "cache"), "--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)
		}
		for _, tag := range []string{"v0.0.1", "v0.0.1-alpine"} {
			expectOnlyPlatformBlobsPresent(ctx, sparseRegistry, "openshift4/ocp-release", tag, []string{"linux/amd64", "linux/arm64"})
		}
	},
		Entry("direct mirroring", false),
		Entry("archive round trip", true),
	)
})
