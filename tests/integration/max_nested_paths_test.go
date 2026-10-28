package integration_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	operatorv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	ocv1 "github.com/operator-framework/operator-controller/api/v1"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

var _ = Describe("max-nested-paths", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	It("should flatten destination repositories and point IDMS, ITMS and catalog resources to them", func() {
		isc := filepath.Join(iscDir, "max_nested_paths", "isc-max-nested-paths.yaml")
		endpoint := testRegistry.Endpoint()

		By("running mirrorToMirror to a destination with three path levels and --max-nested-paths=2")
		result, err := runner.MirrorToMirror(ctx, isc, workDir, endpoint+"/nested/path/levels",
			"--remove-signatures=true", "--dest-tls-verify=false", "--max-nested-paths=2")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying every repository has at most two path levels")
		// The first path level is kept and the rest are joined with "-".
		releaseImagesRepo := "nested/path-levels-openshift-release-images"
		releaseRepo := "nested/path-levels-openshift-release"
		operatorRepo := "nested/path-levels-oc-mirror-oc-mirror-dev"
		expectRegistryContainsExactly(*testRegistry, map[string]string{
			releaseImagesRepo + ":v0.0.1":         "quay.io/oc-mirror/release/test-release-index:v0.0.1",
			releaseRepo + ":v0.0.1-alpine":        "quay.io/oc-mirror/release/test-image:v0.0.1",
			operatorRepo + ":test-catalog-latest": "",
			operatorRepo + ":baz-bundle-v1.1.0":   "quay.io/oc-mirror/oc-mirror-dev:baz-bundle-v1.1.0",
			operatorRepo + ":foo-bundle-v0.2.0":   "quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.2.0",
			operatorRepo + ":sha256-1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58": "quay.io/oc-mirror/oc-mirror-dev@sha256:1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58",
		})
		repos, err := testRegistry.ListRepositories(ctx)
		Expect(err).NotTo(HaveOccurred())
		for _, repo := range repos {
			Expect(strings.Split(repo, "/")).To(HaveLen(2), "repository %q has more than two path levels", repo)
		}

		By("verifying IDMS and ITMS map each source repository to its flattened repository")
		expectMirrorSets(workDir, endpoint,
			map[string]string{
				"quay.io/oc-mirror/release/test-release-index": releaseImagesRepo,
				"quay.io/oc-mirror/oc-mirror-dev":              operatorRepo,
			},
			map[string]string{
				"quay.io/oc-mirror/release/test-release-index": releaseImagesRepo,
				"quay.io/oc-mirror/release/test-image":         releaseRepo,
				"quay.io/oc-mirror/oc-mirror-dev":              operatorRepo,
			})

		By("verifying CatalogSource and ClusterCatalog reference the flattened catalog repository")
		isCatalog, err := testRegistry.IsCatalog(ctx, operatorRepo, "test-catalog-latest")
		Expect(err).NotTo(HaveOccurred())
		Expect(isCatalog).To(BeTrue())
		resourcesDir := filepath.Join(workDir, dirWorkingDir, dirClusterResources)
		resourceName := "path-levels-oc-mirror-oc-mirror-dev-test-catalog.yaml"
		catalogImage := endpoint + "/" + operatorRepo + ":test-catalog-latest"
		data, err := os.ReadFile(filepath.Join(resourcesDir, "cs-"+resourceName))
		Expect(err).NotTo(HaveOccurred())
		var catalogSource operatorv1alpha1.CatalogSource
		Expect(yaml.Unmarshal(data, &catalogSource)).To(Succeed())
		Expect(catalogSource.Kind).To(Equal("CatalogSource"))
		Expect(catalogSource.Spec.Image).To(Equal(catalogImage))

		data, err = os.ReadFile(filepath.Join(resourcesDir, "cc-"+resourceName))
		Expect(err).NotTo(HaveOccurred())
		var clusterCatalog ocv1.ClusterCatalog
		Expect(yaml.Unmarshal(data, &clusterCatalog)).To(Succeed())
		Expect(clusterCatalog.Kind).To(Equal("ClusterCatalog"))
		Expect(clusterCatalog.Spec.Source.Image).NotTo(BeNil())
		Expect(clusterCatalog.Spec.Source.Image.Ref).To(Equal(catalogImage))
	})
})

// expectMirrorSets checks source/destination pairs for the new output-related cases.
func expectMirrorSets(workDir, endpoint string, idms, itms map[string]string) {
	GinkgoHelper()
	for filename, expected := range map[string]map[string]string{
		"idms-oc-mirror.yaml": idms,
		"itms-oc-mirror.yaml": itms,
	} {
		data, err := os.ReadFile(filepath.Join(workDir, dirWorkingDir, dirClusterResources, filename))
		Expect(err).NotTo(HaveOccurred())
		actual := map[string]bool{}
		decoder := yamlutil.NewYAMLOrJSONDecoder(strings.NewReader(string(data)), 4096)
		for {
			var digestSet configv1.ImageDigestMirrorSet
			var tagSet configv1.ImageTagMirrorSet
			var document any = &digestSet
			if filename == "itms-oc-mirror.yaml" {
				document = &tagSet
			}
			if err := decoder.Decode(document); err == io.EOF {
				break
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
			for _, entry := range digestSet.Spec.ImageDigestMirrors {
				for _, destination := range entry.Mirrors {
					actual[entry.Source+"="+string(destination)] = true
				}
			}
			for _, entry := range tagSet.Spec.ImageTagMirrors {
				for _, destination := range entry.Mirrors {
					actual[entry.Source+"="+string(destination)] = true
				}
			}
		}
		wanted := map[string]bool{}
		for source, repo := range expected {
			wanted[source+"="+endpoint+"/"+repo] = true
		}
		Expect(actual).To(Equal(wanted), "unexpected mirrors in %s", filename)
	}
}
