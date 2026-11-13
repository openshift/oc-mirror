package integration_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	requiredHelmRepo = "openshifttest/hello-openshift"

	iscHelmValuesMissing = "isc-required-values-missing.yaml"
	iscHelmValuesFiles   = "isc-required-values-files.yaml"
	iscHelmValuesInline  = "isc-required-values-inline.yaml"
)

var _ = Describe("helm chart values for image discovery", func() {
	var workDir string
	var chartPath string

	BeforeEach(func() {
		workDir = setupWorkDir()
		chartPath = filepath.Join(integrationArtifactsDir(), "helm", "required-values-chart-0.1.0.tgz")
		Expect(chartPath).To(BeAnExistingFile())
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	It("should fail mirror-to-disk when a chart requires values that are not provided", func() {
		isc := materializeHelmISC(workDir, iscHelmValuesMissing, map[string]string{
			"__CHART_PATH__": chartPath,
		})

		By("running M2D against a chart that uses required() without values")
		result, err := runner.MirrorToDisk(ctx, isc, workDir)
		expectOcMirrorExitCode(result, err, 16, "secretName is required!")

		By("verifying no tar archive was produced")
		expectNoTarArchive(workDir)
	})

	It("should discover and mirror images when required values are provided via valuesFiles", func() {
		valuesFile := copyHelmValuesFile(workDir, "values-secret.yaml")
		isc := materializeHelmISC(workDir, iscHelmValuesFiles, map[string]string{
			"__CHART_PATH__":  chartPath,
			"__VALUES_FILE__": valuesFile,
		})

		By("running mirrorToDisk with valuesFiles")
		result, err := runner.MirrorToDisk(ctx, isc, workDir, "--remove-signatures=true")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying the chart image was cached")
		expectLocalCacheContains(cacheDir, requiredHelmRepo)

		By("verifying valuesFiles were copied into the working directory for disk-to-mirror")
		matches, err := filepath.Glob(filepath.Join(workDir, dirWorkingDir, dirHelm, "values", "*", "*-values-secret.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(matches).NotTo(BeEmpty(), "expected persisted helm values file under working-dir/helm/values")

		By("removing the original values file so D2M must use the persisted copy")
		Expect(os.Remove(valuesFile)).To(Succeed())

		By("running diskToMirror")
		result, err = runner.DiskToMirror(ctx, isc, workDir, testRegistry.Endpoint(),
			"--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying the discovered image is in the destination registry")
		expectRepositoriesExist(*testRegistry, []string{requiredHelmRepo})
	})

	It("should let inline values override valuesFiles during dry-run image discovery", func() {
		valuesFile := copyHelmValuesFile(workDir, "values-file-image.yaml")
		isc := materializeHelmISC(workDir, iscHelmValuesInline, map[string]string{
			"__CHART_PATH__":  chartPath,
			"__VALUES_FILE__": valuesFile,
		})

		By("running mirrorToDisk --dry-run")
		result, err := runner.MirrorToDisk(ctx, isc, workDir, "--remove-signatures=true", "--dry-run")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying mapping.txt contains the inline image, not the valuesFile image")
		mappingPath := filepath.Join(workDir, dirWorkingDir, "dry-run", "mapping.txt")
		data, err := os.ReadFile(mappingPath)
		Expect(err).NotTo(HaveOccurred(), "mapping.txt not found at: %s", mappingPath)
		Expect(string(data)).To(ContainSubstring(requiredHelmRepo))
		Expect(string(data)).NotTo(ContainSubstring("library/nginx"))
	})
})

func integrationArtifactsDir() string {
	if dir := os.Getenv("ARTIFACTS_DIR"); dir != "" {
		return dir
	}
	return "testdata"
}

func copyHelmValuesFile(workDir, name string) string {
	src := filepath.Join(integrationArtifactsDir(), "helm", name)
	data, err := os.ReadFile(src)
	Expect(err).NotTo(HaveOccurred())
	dst := filepath.Join(workDir, name)
	Expect(os.WriteFile(dst, data, 0o600)).To(Succeed())
	return dst
}

// materializeHelmISC copies a checked-in ImageSetConfiguration and fills in
// paths that depend on the test workspace. The chart and values structure stays
// in testdata rather than being assembled in Go.
func materializeHelmISC(workDir, name string, replacements map[string]string) string {
	src := filepath.Join(iscDir, "helm_values", name)
	data, err := os.ReadFile(src)
	Expect(err).NotTo(HaveOccurred())
	text := string(data)
	for key, value := range replacements {
		text = strings.ReplaceAll(text, key, value)
	}
	Expect(text).NotTo(ContainSubstring("__CHART_PATH__"))
	Expect(text).NotTo(ContainSubstring("__VALUES_FILE__"))
	dst := filepath.Join(workDir, name)
	Expect(os.WriteFile(dst, []byte(text), 0o644)).To(Succeed())
	return dst
}

func expectLocalCacheContains(cacheDir, repoSubstring string) {
	repos, err := listLocalCacheRepositories(cacheDir)
	Expect(err).NotTo(HaveOccurred())
	Expect(repos).NotTo(BeEmpty(), "local cache has no repositories")
	found := false
	for _, repo := range repos {
		if strings.Contains(repo, repoSubstring) {
			found = true
			break
		}
	}
	Expect(found).To(BeTrue(), "missing cache repository %q, got: %v", repoSubstring, repos)
}
