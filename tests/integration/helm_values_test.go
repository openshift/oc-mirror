package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	requiredHelmImage = "quay.io/openshifttest/hello-openshift@sha256:61b8f5e1a3b5dbd9e2c35fd448dc5106337d7a299873dd3a6f0cd8d4891ecc27"
	requiredHelmRepo  = "openshifttest/hello-openshift"
)

var _ = Describe("helm chart values for image discovery", func() {
	var workDir string
	var chartPath string

	BeforeEach(func() {
		workDir = setupWorkDir()
		chartPath = requiredValuesChartPath()
		Expect(chartPath).To(BeAnExistingFile())
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	It("should fail mirror-to-disk when a chart requires values that are not provided", func() {
		isc := writeHelmValuesISC(workDir, chartPath, nil, "")

		By("running M2D against a chart that uses required() without values")
		result, err := runner.MirrorToDisk(ctx, isc, workDir)
		expectOcMirrorExitCode(result, err, 16, "secretName is required!")

		By("verifying no tar archive was produced")
		expectNoTarArchive(workDir)
	})

	It("should discover and mirror images when required values are provided via valuesFiles", func() {
		valuesFile := filepath.Join(workDir, "values-for-mirroring.yaml")
		Expect(os.WriteFile(valuesFile, []byte("secretName: dummy-secret\n"), 0o600)).To(Succeed())
		isc := writeHelmValuesISC(workDir, chartPath, []string{valuesFile}, "")

		By("running mirrorToDisk with valuesFiles")
		result, err := runner.MirrorToDisk(ctx, isc, workDir, "--remove-signatures=true")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying the chart image was cached")
		expectLocalCacheContains(cacheDir, requiredHelmRepo)

		By("verifying valuesFiles were copied into the working directory for disk-to-mirror")
		matches, err := filepath.Glob(filepath.Join(workDir, dirWorkingDir, dirHelm, "values", "*", "*-values-for-mirroring.yaml"))
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
		valuesFile := filepath.Join(workDir, "values-for-mirroring.yaml")
		Expect(os.WriteFile(valuesFile, []byte("secretName: from-file\nimage: docker.io/library/nginx:1.16.0\n"), 0o600)).To(Succeed())
		isc := writeHelmValuesISC(workDir, chartPath, []string{valuesFile}, "")
		content, err := os.ReadFile(isc)
		Expect(err).NotTo(HaveOccurred())
		isc = filepath.Join(workDir, "isc-helm-inline-override.yaml")
		Expect(os.WriteFile(isc, []byte(string(content)+
			"        values:\n"+
			"          image: "+requiredHelmImage+"\n"), 0o644)).To(Succeed())

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

func requiredValuesChartPath() string {
	_, file, _, ok := runtime.Caller(0)
	Expect(ok).To(BeTrue())
	return filepath.Join(filepath.Dir(file), "testdata", "helm", "required-values-chart-0.1.0.tgz")
}

func writeHelmValuesISC(dir, chartPath string, valuesFiles []string, inlineSecret string) string {
	var b strings.Builder
	b.WriteString("kind: ImageSetConfiguration\n")
	b.WriteString("apiVersion: mirror.openshift.io/v2alpha1\n")
	b.WriteString("mirror:\n")
	b.WriteString("  helm:\n")
	b.WriteString("    local:\n")
	b.WriteString("      - name: required-values-chart\n")
	fmt.Fprintf(&b, "        path: %s\n", chartPath)
	if len(valuesFiles) > 0 {
		b.WriteString("        valuesFiles:\n")
		for _, vf := range valuesFiles {
			fmt.Fprintf(&b, "          - %s\n", vf)
		}
	}
	if inlineSecret != "" {
		b.WriteString("        values:\n")
		fmt.Fprintf(&b, "          secretName: %s\n", inlineSecret)
	}
	p := filepath.Join(dir, "isc-helm-required-values.yaml")
	Expect(os.WriteFile(p, []byte(b.String()), 0o644)).To(Succeed())
	return p
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
