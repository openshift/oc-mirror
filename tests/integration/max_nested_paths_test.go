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
			releaseImagesRepo + ":v0.0.1":         "quay.io/oc-mirror/release/test-release-index:v0.0.1",
			releaseRepo + ":v0.0.1-alpine":        "quay.io/oc-mirror/release/test-image:v0.0.1",
			operatorRepo + ":test-catalog-latest": "",
			operatorRepo + ":baz-bundle-v1.1.0":   "quay.io/oc-mirror/oc-mirror-dev:baz-bundle-v1.1.0",
			operatorRepo + ":foo-bundle-v0.2.0":   "quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.2.0",
			operatorRepo + ":sha256-1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58": operandImage,
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

	It("should delete after mirror when --max-nested-paths flattened the destinations", func() {
		isc := filepath.Join(iscDir, "max_nested_paths", "isc-max-nested-paths.yaml")
		disc := filepath.Join(iscDir, "max_nested_paths", "disc-max-nested-paths.yaml")
		endpoint := testRegistry.Endpoint()
		dest := endpoint + "/nested/path/levels"
		deleteId := "mnp-delete"
		deleteYaml := filepath.Join(workDir, dirWorkingDir, "delete", "delete-images-"+deleteId+".yaml")

		releaseImagesRepo := "nested/path-levels-openshift-release-images"
		releaseRepo := "nested/path-levels-openshift-release"
		operatorRepo := "nested/path-levels-oc-mirror-oc-mirror-dev"

		// Use m2d→d2m so delete --generate can collect the operator catalog from
		// local cache (same pattern as delete_test.go). Flattening is applied on d2m.
		By("mirroring to disk")
		result, err := runner.MirrorToDisk(ctx, isc, workDir, "--remove-signatures=true")
		expectOcMirrorCommandSuccess(result, err)

		By("pushing to registry with --max-nested-paths=2")
		result, err = runner.DiskToMirror(ctx, isc, workDir, dest,
			"--remove-signatures=true", "--dest-tls-verify=false", "--max-nested-paths=2")
		expectOcMirrorCommandSuccess(result, err)

		By("generating delete YAML with the same --max-nested-paths")
		result, err = runner.DeletePhaseOne(ctx, disc, workDir, deleteId, dest,
			"--max-nested-paths=2")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying delete YAML uses flattened imageReferences and records unflattenedImageReference")
		data, err := os.ReadFile(deleteYaml)
		Expect(err).NotTo(HaveOccurred())
		var deleteList struct {
			Items []struct {
				ImageReference            string `json:"imageReference"`
				UnflattenedImageReference string `json:"unflattenedImageReference"`
			} `json:"items"`
		}
		Expect(yaml.Unmarshal(data, &deleteList)).To(Succeed())
		Expect(deleteList.Items).NotTo(BeEmpty())
		var sawFlattened, sawUnflattened bool
		for _, item := range deleteList.Items {
			ref := item.ImageReference
			if strings.Contains(ref, releaseImagesRepo) || strings.Contains(ref, releaseRepo) || strings.Contains(ref, operatorRepo) {
				sawFlattened = true
			}
			Expect(ref).NotTo(ContainSubstring("/nested/path/levels/"),
				"imageReference should be flattened, got %s", ref)
			if item.UnflattenedImageReference != "" {
				sawUnflattened = true
				Expect(item.UnflattenedImageReference).To(ContainSubstring("/nested/path/levels/"))
			}
		}
		Expect(sawFlattened).To(BeTrue(), "expected flattened release/operator refs in delete YAML")
		Expect(sawUnflattened).To(BeTrue(), "expected unflattenedImageReference when max-nested-paths is set")

		By("running delete phase 2")
		result, err = runner.DeletePhaseTwo(ctx, deleteYaml, dest,
			"--dest-tls-verify=false", "--max-nested-paths=2")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying flattened repositories were removed from the registry")
		repos, err := testRegistry.ListRepositories(ctx)
		Expect(err).NotTo(HaveOccurred())
		for _, repo := range repos {
			Expect(repo).NotTo(Equal(releaseImagesRepo))
			Expect(repo).NotTo(Equal(releaseRepo))
			Expect(repo).NotTo(Equal(operatorRepo))
		}
	})
})
