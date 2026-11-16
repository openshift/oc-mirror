package integration_test

import (
	"fmt"
	"os"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
)

var _ = Describe("platform target repositories with graph data", Label("platform-target-repo"), func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
		cacheDir = filepath.Join(workDir, "cache", ".oc-mirror", ".cache")
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	DescribeTable("generates UpdateService references matching the mirrored repositories", func(archive bool) {
		By("selecting a release through a local Cincinnati server")
		server := startReleaseChannelServer()
		channelRunner := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).WithEnv([]string{"UPDATE_URL_OVERRIDE=" + server.URL})
		isc := filepath.Join(iscDir, "platform_target_repo", "isc-graph.yaml")

		By("providing a local graph image for the overridden Cincinnati endpoint")
		graphLayout, err := layout.Write(filepath.Join(workDir, dirWorkingDir, "graph-preparation"), empty.Index)
		Expect(err).NotTo(HaveOccurred())
		graphConfig, err := empty.Image.ConfigFile()
		Expect(err).NotTo(HaveOccurred())
		graphConfig.OS = "linux"
		graphConfig.Architecture = "amd64"
		graphImage, err := mutate.ConfigFile(empty.Image, graphConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(graphLayout.AppendImage(graphImage, layout.WithPlatform(v1.Platform{OS: "linux", Architecture: "amd64"}))).To(Succeed())

		destination := testRegistry.Endpoint() + "/team"
		resourceWorkDir := workDir
		if archive {
			By("archiving release images and graph data")
			result, err := channelRunner.MirrorToDisk(ctx, isc, workDir,
				"--cache-dir", filepath.Join(workDir, "cache"), "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("transporting only the archives into a fresh workspace")
			restoreDir := filepath.Join(workDir, "restore")
			Expect(os.MkdirAll(restoreDir, 0700)).To(Succeed())
			for _, archivePath := range findTarArchives(workDir) {
				Expect(copyFile(archivePath, filepath.Join(restoreDir, filepath.Base(archivePath)))).To(Succeed())
			}
			resourceWorkDir = restoreDir

			By("restoring with a fresh cache and outbound access blocked")
			server.Close()
			offlineRunner := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).WithEnv([]string{
				"UPDATE_URL_OVERRIDE=" + server.URL,
				"HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1",
				"http_proxy=http://127.0.0.1:1", "https_proxy=http://127.0.0.1:1",
				"NO_PROXY=localhost,127.0.0.1", "no_proxy=localhost,127.0.0.1",
			})
			result, err = offlineRunner.DiskToMirror(ctx, isc, restoreDir, destination,
				"--cache-dir", filepath.Join(workDir, "restore-cache"), "--remove-signatures=true",
				"--dest-tls-verify=false", "--max-nested-paths=3", "--retry-times=1")
			expectOcMirrorCommandSuccess(result, err)
		} else {
			By("mirroring directly with a destination nesting limit")
			result, err := channelRunner.MirrorToMirror(ctx, isc, workDir, destination,
				"--cache-dir", filepath.Join(workDir, "cache"), "--remove-signatures=true",
				"--dest-tls-verify=false", "--max-nested-paths=3")
			expectOcMirrorCommandSuccess(result, err)
		}

		By("checking that payloads, components, and graph images exist at the generated paths")
		repositories, err := testRegistry.ListRepositories(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(repositories).To(ConsistOf("team/releases", "team/components", "team/openshift/graph-image"))
		tags, err := testRegistry.ListTags(ctx, "team/releases")
		Expect(err).NotTo(HaveOccurred())
		Expect(tags).To(ConsistOf("v0.0.1"))
		componentTags, err := testRegistry.ListTags(ctx, "team/components")
		Expect(err).NotTo(HaveOccurred(), "list independently mirrored release components")
		Expect(componentTags).To(ConsistOf("v0.0.1-alpine"), "components must not be placed in the payload repository")
		graphRef := destination + "/openshift/graph-image:latest"
		Expect(registryImageDigest(graphRef)).NotTo(BeEmpty())

		By("checking that UpdateService points to the custom payload repository and unchanged graph repository")
		content, err := os.ReadFile(filepath.Join(resourceWorkDir, dirWorkingDir, dirClusterResources, "updateService.yaml"))
		Expect(err).NotTo(HaveOccurred())
		var updateService struct {
			Spec struct {
				Releases       string `yaml:"releases"`
				GraphDataImage string `yaml:"graphDataImage"`
			} `yaml:"spec"`
		}
		Expect(yaml.Unmarshal(content, &updateService)).To(Succeed())
		Expect(updateService.Spec.Releases).To(Equal(destination + "/releases"))
		Expect(updateService.Spec.GraphDataImage).To(Equal(graphRef))
	},
		Entry("direct mirroring", false),
		Entry("offline archive round trip", true),
	)

	DescribeTable("reserves the graph repository before collection can push it", func(field, repo string, limit int) {
		isc := filepath.Join(workDir, "graph-conflict.yaml")
		Expect(os.WriteFile(isc, fmt.Appendf(nil, `kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v2alpha1
mirror:
  platform:
    %s: %s
    graph: true
`, field, repo), 0600)).To(Succeed())
		result, err := runner.MirrorToMirror(ctx, isc, workDir, testRegistry.Endpoint()+"/team",
			"--cache-dir", filepath.Join(workDir, "cache"), "--dest-tls-verify=false", fmt.Sprintf("--max-nested-paths=%d", limit))
		expectOcMirrorExitCode(result, err, 1, "conflicts with the Cincinnati graph repository")
		expectEmptyRegistry(*testRegistry)
	},
		Entry("payload original path", "targetImageRepo", "openshift/graph-image", 0),
		Entry("payload flattened path", "targetImageRepo", "openshift-graph-image", 2),
		Entry("component original path", "targetComponentRepo", "openshift/graph-image", 0),
		Entry("component flattened path", "targetComponentRepo", "openshift-graph-image", 2),
	)
})
