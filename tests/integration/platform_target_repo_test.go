package integration_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

var _ = Describe("platform targetRepo", Label("platform-target-repo"), func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
		cacheDir = filepath.Join(workDir, "cache", ".oc-mirror", ".cache")
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

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
				ImageReference string `yaml:"imageReference"`
			} `yaml:"items"`
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
		Expect(string(savedConfig)).To(ContainSubstring("targetRepo: " + repo))

		By("deleting release payload and components from the registry and local cache")
		result, err = runner.DeletePhaseTwo(ctx, deleteYaml, testRegistry.Endpoint()+"/team",
			"--cache-dir", filepath.Join(workDir, "cache"), "--dest-tls-verify=false", "--force-cache-delete=true")
		expectOcMirrorCommandSuccess(result, err)
		expectEmptyRegistry(*testRegistry)
		expectRepositoriesAbsentFromLocalCache(cacheDir, []string{repo})
	})
})
