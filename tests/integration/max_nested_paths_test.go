package integration_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	ocv1 "github.com/operator-framework/operator-controller/api/v1"
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
			releaseImagesRepo + ":v0.0.1":       testReleaseImage,
			releaseRepo + ":v0.0.1-alpine":      releaseComponentImage,
			operatorRepo + ":" + testCatalogTag: "",
			operatorRepo + ":baz-bundle-v1.1.0": bundleImage("baz-bundle-v1.1.0"),
			// An additional image, published on Quay.
			operatorRepo + ":foo-bundle-v0.2.0":          "quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.2.0",
			operatorRepo + ":" + digestTag(operandImage): operandImage,
		})
		repos, err := testRegistry.ListRepositories(ctx)
		Expect(err).NotTo(HaveOccurred())
		for _, repo := range repos {
			Expect(strings.Split(repo, "/")).To(HaveLen(2), "repository %q has more than two path levels", repo)
		}

		By("verifying IDMS and ITMS map each source repository to its flattened repository")
		releaseIndex := referenceWithoutTagOrDigest(testReleaseImage)
		releaseComponent := referenceWithoutTagOrDigest(releaseComponentImage)
		operatorImages := originRegistry.Endpoint() + "/" + testCatalogRepo
		expectMirrorSets(workDir, endpoint,
			map[string]string{
				releaseIndex:   releaseImagesRepo,
				operatorImages: operatorRepo,
			},
			map[string]string{
				releaseIndex:     releaseImagesRepo,
				releaseComponent: releaseRepo,
				operatorImages:   operatorRepo,
				// The additional image is published on Quay, and referenced by tag.
				"quay.io/oc-mirror/oc-mirror-dev": operatorRepo,
			})

		By("verifying CatalogSource and ClusterCatalog reference the flattened catalog repository")
		isCatalog, err := testRegistry.IsCatalog(ctx, operatorRepo, testCatalogTag)
		Expect(err).NotTo(HaveOccurred())
		Expect(isCatalog).To(BeTrue())
		resourcesDir := filepath.Join(workDir, dirWorkingDir, dirClusterResources)
		resourceName := "path-levels-oc-mirror-oc-mirror-dev-test-catalog.yaml"
		catalogImage := endpoint + "/" + operatorRepo + ":" + testCatalogTag
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
