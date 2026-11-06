package integration_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("independent platform target repositories", Label("platform-target-repo"), func() {
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
})
