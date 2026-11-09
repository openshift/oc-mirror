package integration_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

var _ = Describe("platform target repositories", Label("platform-target-repo"), func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
		cacheDir = filepath.Join(workDir, "cache", ".oc-mirror", ".cache")
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	DescribeTable("mirrors and deletes payloads and components at their independently selected paths", func(imageRepo, componentRepo string, archive bool) {
		writeConfig := func(kind, section string) string {
			configPath := filepath.Join(workDir, kind+".yaml")
			data := fmt.Appendf(nil, `kind: %s
apiVersion: mirror.openshift.io/v2alpha1
%s:
  platform:
    targetImageRepo: %q
    targetComponentRepo: %q
    release: quay.io/oc-mirror/release/test-release-index:v0.0.1
`, kind, section, imageRepo, componentRepo)
			Expect(os.WriteFile(configPath, data, 0600)).To(Succeed(), "write %s for independent repository paths", kind)
			return configPath
		}
		isc := writeConfig("ImageSetConfiguration", "mirror")
		disc := writeConfig("DeleteImageSetConfiguration", "delete")
		wantImageRepo := imageRepo
		if wantImageRepo == "" {
			wantImageRepo = "openshift/release-images"
		}
		wantComponentRepo := componentRepo
		if wantComponentRepo == "" {
			wantComponentRepo = "openshift/release"
		}
		const namespace = "mirrors/quay.io"
		destination := testRegistry.Endpoint() + "/" + namespace
		resourceWorkDir := workDir
		activeCache := filepath.Join(workDir, "cache")
		if archive {
			By("archiving both repositories and transporting them into a fresh workspace")
			result, err := runner.MirrorToDisk(ctx, isc, workDir, "--cache-dir", activeCache, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)
			resourceWorkDir = filepath.Join(workDir, "restore")
			Expect(os.MkdirAll(resourceWorkDir, 0700)).To(Succeed(), "create fresh archive restore workspace")
			for _, archivePath := range findTarArchives(workDir) {
				Expect(copyFile(archivePath, filepath.Join(resourceWorkDir, filepath.Base(archivePath)))).To(Succeed(), "transport archive %s", archivePath)
			}
			activeCache = filepath.Join(workDir, "restore-cache")
			result, err = runner.DiskToMirror(ctx, isc, resourceWorkDir, destination,
				"--cache-dir", activeCache, "--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)
		} else {
			By("mirroring directly into a registry namespace")
			result, err := runner.MirrorToMirror(ctx, isc, workDir, destination,
				"--cache-dir", activeCache, "--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)
		}

		By("checking each role uses only its own override or its own default")
		repositories, err := testRegistry.ListRepositories(ctx)
		Expect(err).NotTo(HaveOccurred(), "list mirrored repositories")
		Expect(repositories).To(ConsistOf(namespace+"/"+wantImageRepo, namespace+"/"+wantComponentRepo), "payload and component repositories must be independent")
		for _, expected := range []struct{ repo, tag string }{
			{wantImageRepo, "v0.0.1"},
			{wantComponentRepo, "v0.0.1-alpine"},
		} {
			tags, err := testRegistry.ListTags(ctx, namespace+"/"+expected.repo)
			Expect(err).NotTo(HaveOccurred(), "list tags for %s", expected.repo)
			Expect(tags).To(ConsistOf(expected.tag), "repository %s must contain only the expected role", expected.repo)
		}
		idmsData, err := os.ReadFile(filepath.Join(resourceWorkDir, dirWorkingDir, dirClusterResources, idmsFileName))
		Expect(err).NotTo(HaveOccurred(), "read generated IDMS")
		var idms configv1.ImageDigestMirrorSet
		Expect(yaml.Unmarshal(idmsData, &idms)).To(Succeed(), "decode generated IDMS")
		Expect(idms.Spec.ImageDigestMirrors).To(ConsistOf(configv1.ImageDigestMirrors{
			Source:  "quay.io/oc-mirror/release/test-release-index",
			Mirrors: []configv1.ImageMirror{configv1.ImageMirror(destination + "/" + wantImageRepo)},
		}), "IDMS must map the payload to its exact repository")
		itmsData, err := os.ReadFile(filepath.Join(resourceWorkDir, dirWorkingDir, dirClusterResources, "itms-oc-mirror.yaml"))
		Expect(err).NotTo(HaveOccurred(), "read generated ITMS for the tagged fixture images")
		var itms configv1.ImageTagMirrorSet
		Expect(yaml.Unmarshal(itmsData, &itms)).To(Succeed(), "decode generated ITMS")
		Expect(itms.Spec.ImageTagMirrors).To(ConsistOf(
			configv1.ImageTagMirrors{
				Source:  "quay.io/oc-mirror/release/test-release-index",
				Mirrors: []configv1.ImageMirror{configv1.ImageMirror(destination + "/" + wantImageRepo)},
			},
			configv1.ImageTagMirrors{
				Source:  "quay.io/oc-mirror/release/test-image",
				Mirrors: []configv1.ImageMirror{configv1.ImageMirror(destination + "/" + wantComponentRepo)},
			},
		), "ITMS must map payload and component sources to their own exact repositories")

		By("generating deletion metadata for the same independently configured repositories")
		result, err := runner.DeletePhaseOne(ctx, disc, resourceWorkDir, "split", destination,
			"--cache-dir", activeCache, "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		deleteFile := filepath.Join(resourceWorkDir, dirWorkingDir, "delete", "delete-images-split.yaml")
		data, err := os.ReadFile(deleteFile)
		Expect(err).NotTo(HaveOccurred(), "read generated delete references")
		var deleteList struct {
			Items []struct {
				ImageReference string `json:"imageReference"`
			} `json:"items"`
		}
		Expect(yaml.Unmarshal(data, &deleteList)).To(Succeed(), "decode generated delete references")
		references := make([]string, 0, len(deleteList.Items))
		for _, item := range deleteList.Items {
			references = append(references, item.ImageReference)
		}
		Expect(references).To(ConsistOf(
			"docker://"+destination+"/"+wantImageRepo+":v0.0.1",
			"docker://"+destination+"/"+wantComponentRepo+":v0.0.1-alpine",
		), "delete metadata must preserve both destination paths")
		result, err = runner.DeletePhaseTwo(ctx, deleteFile, destination,
			"--cache-dir", activeCache, "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		expectEmptyRegistry(*testRegistry)
	},
		Entry("direct defaults", "", "", false),
		Entry("archive defaults", "", "", true),
		Entry("direct payload override", "openshift-release-dev/ocp-release", "", false),
		Entry("archive payload override", "openshift-release-dev/ocp-release", "", true),
		Entry("direct component override", "", "openshift-release-dev/ocp-v4.0-art-dev", false),
		Entry("archive component override", "", "openshift-release-dev/ocp-v4.0-art-dev", true),
		Entry("direct separate overrides", "openshift-release-dev/ocp-release", "openshift-release-dev/ocp-v4.0-art-dev", false),
		Entry("archive separate overrides", "openshift-release-dev/ocp-release", "openshift-release-dev/ocp-v4.0-art-dev", true),
	)

	It("mirrors release payload and components through an archive into a custom repository", func() {
		isc := filepath.Join(iscDir, "platform_target_repo", "isc.yaml")
		repo := "openshift4/ocp-release"
		tags := []string{"v0.0.1", "v0.0.1-alpine"}

		By("mirroring payload and components to the custom cache repository")
		result, err := runner.MirrorToDisk(ctx, isc, workDir, "--cache-dir", filepath.Join(workDir, "cache"), "--remove-signatures=true")
		expectOcMirrorCommandSuccess(result, err)
		cachedDigests := make(map[string]string)
		for _, tag := range tags {
			cachedDigests[tag] = localCacheTagDigest(cacheDir, repo, tag)
		}

		By("checking the custom repository is included in the archive")
		var entries []string
		for _, archive := range findTarArchives(workDir) {
			entries = append(entries, listTarEntries(archive)...)
		}
		expectTarContainsPath(entries, tarRepositoriesPath+repo)

		By("restoring into a fresh cache and pushing to a destination namespace")
		restoreCache := filepath.Join(workDir, "restore-cache")
		result, err = runner.DiskToMirror(ctx, isc, workDir, testRegistry.Endpoint()+"/team",
			"--cache-dir", restoreCache, "--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		repositories, err := testRegistry.ListRepositories(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(repositories).To(ConsistOf("team/" + repo))
		for _, tag := range tags {
			Expect(registryImageDigest(testRegistry.Endpoint() + "/team/" + repo + ":" + tag)).To(Equal(cachedDigests[tag]))
		}

		By("checking generated mirror resources use the custom repository")
		expectCorrectIDMS(workDir, isc)
		for _, filename := range []string{idmsFileName, "itms-oc-mirror.yaml"} {
			content, err := os.ReadFile(filepath.Join(workDir, dirWorkingDir, dirClusterResources, filename))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring(testRegistry.Endpoint() + "/team/" + repo))
			Expect(string(content)).NotTo(ContainSubstring(testRegistry.Endpoint() + "/team/openshift/release"))
		}

		By("checking direct-mirror dry-run mappings use the same destination")
		result, err = runner.MirrorToMirror(ctx, isc, workDir, testRegistry.Endpoint()+"/team",
			"--cache-dir", filepath.Join(workDir, "cache"), "--dry-run", "--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		Expect(mappingDestinations(workDir)).To(ConsistOf(
			"docker://"+testRegistry.Endpoint()+"/team/"+repo+":"+tags[0],
			"docker://"+testRegistry.Endpoint()+"/team/"+repo+":"+tags[1],
		))

		By("generating delete metadata for the custom release repository")
		disc := filepath.Join(iscDir, "platform_target_repo", "disc.yaml")
		result, err = runner.DeletePhaseOne(ctx, disc, workDir, "", testRegistry.Endpoint()+"/team",
			"--cache-dir", filepath.Join(workDir, "cache"), "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		expectValidDeleteImagesFiles(workDir, "")
		deleteYaml := filepath.Join(workDir, dirWorkingDir, "delete", "delete-images.yaml")
		data, err := os.ReadFile(deleteYaml)
		Expect(err).NotTo(HaveOccurred())
		var deleteList struct {
			Items []struct {
				ImageReference string `json:"imageReference"`
			} `json:"items"`
		}
		Expect(yaml.Unmarshal(data, &deleteList)).To(Succeed())
		var references []string
		for _, item := range deleteList.Items {
			references = append(references, item.ImageReference)
		}
		Expect(references).To(ConsistOf(
			"docker://"+testRegistry.Endpoint()+"/team/"+repo+":"+tags[0],
			"docker://"+testRegistry.Endpoint()+"/team/"+repo+":"+tags[1],
		))
		savedConfig, err := os.ReadFile(filepath.Join(workDir, dirWorkingDir, "delete", "delete-imageset-config.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(savedConfig)).To(ContainSubstring("targetImageRepo: " + repo))
		Expect(string(savedConfig)).To(ContainSubstring("targetComponentRepo: " + repo))

		By("deleting release payload and components from the registry and local cache")
		result, err = runner.DeletePhaseTwo(ctx, deleteYaml, testRegistry.Endpoint()+"/team",
			"--cache-dir", filepath.Join(workDir, "cache"), "--dest-tls-verify=false", "--force-cache-delete=true")
		expectOcMirrorCommandSuccess(result, err)
		expectEmptyRegistry(*testRegistry)
		expectRepositoriesAbsentFromLocalCache(cacheDir, []string{repo})
	})

	It("copies directly with signatures and repeats safely", func() {
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

	It("rejects an additional image that would overwrite the release payload", func() {
		isc := filepath.Join(iscDir, "platform_target_repo", "isc-conflict.yaml")
		result, err := runner.MirrorToMirror(ctx, isc, workDir, testRegistry.Endpoint(),
			"--cache-dir", filepath.Join(workDir, "cache"), "--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorExitCode(result, err, 1, "conflicting destination", "/releases:v0.0.1")
		expectEmptyRegistry(*testRegistry)
	})

	It("rejects a destination namespace that would flatten a target repository", func() {
		result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, "platform_target_repo", "isc.yaml"), workDir, testRegistry.Endpoint()+"/team",
			"--cache-dir", filepath.Join(workDir, "cache"), "--dest-tls-verify=false", "--max-nested-paths=2")
		expectOcMirrorExitCode(result, err, 1, "platform.targetImageRepo", "exceeds --max-nested-paths=2")
		expectEmptyRegistry(*testRegistry)
	})
})

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
