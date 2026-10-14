package integration_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/sets"
)

var _ = Describe("delete", func() {
	var workDir string

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	Describe("delete functionality tests", func() {
		iscHappyPath := filepath.Join("happy_path", "isc-happy-path.yaml")
		discHappyPath := filepath.Join("happy_path", "disc-happy-path.yaml")
		deleteId := "delete-test"

		It("should create delete yaml files with the delete-id in their names", func() {
			deleteYaml := filepath.Join(workDir, "working-dir", "delete", "delete-images-"+deleteId+".yaml")

			By("running mirrorToDisk")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscHappyPath), workDir, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying images are cached locally")
			expectSuccessfulMirrorInLocalCache(filepath.Join(iscDir, iscHappyPath), cacheDir)

			By("running diskToMirror")
			result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscHappyPath), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("running delete phase 1 with --delete-id")
			result, err = runner.DeletePhaseOne(ctx, filepath.Join(iscDir, discHappyPath), workDir, deleteId, testRegistry.Endpoint())
			expectOcMirrorCommandSuccess(result, err)

			By("verifying delete images files are correct")
			expectValidDeleteImagesFiles(workDir, deleteId)

			By("running delete phase 2")
			result, err = runner.DeletePhaseTwo(ctx, deleteYaml, testRegistry.Endpoint(),
				"--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying local registry is empty after delete")
			expectEmptyRegistry(*testRegistry)

			By("verifying local cache is not deleted")
			expectSuccessfulMirrorInLocalCache(filepath.Join(iscDir, iscHappyPath), cacheDir)
		})

		// CLID-253 / OCP-77693 / OCP-86931
		It("should also delete images from the local cache when --force-cache-delete=true is set", func() {
			deleteYaml := filepath.Join(workDir, "working-dir", "delete", "delete-images.yaml")
			cfg := parseImageSetConfig(filepath.Join(iscDir, iscHappyPath))
			nonCatalogRepos := collectExpectedNonCatalogRepos(cfg)

			By("running mirrorToDisk")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscHappyPath), workDir, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying images are cached locally")
			expectSuccessfulMirrorInLocalCache(filepath.Join(iscDir, iscHappyPath), cacheDir)

			By("running diskToMirror")
			result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscHappyPath), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("generating the delete image list")
			result, err = runner.DeletePhaseOne(ctx, filepath.Join(iscDir, discHappyPath), workDir, "", testRegistry.Endpoint())
			expectOcMirrorCommandSuccess(result, err)

			By("verifying delete images files are correct")
			expectValidDeleteImagesFiles(workDir, "")

			By("executing the delete with --force-cache-delete=true")
			result, err = runner.DeletePhaseTwo(ctx, deleteYaml, testRegistry.Endpoint(),
				"--dest-tls-verify=false", "--force-cache-delete=true")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying local registry is empty after delete")
			expectEmptyRegistry(*testRegistry)

			By("verifying non-catalog images were also removed from the local cache")
			expectRepositoriesAbsentFromLocalCache(cacheDir, nonCatalogRepos)
		})
	})

	// OCPBUGS-61236: related images shared with a remaining sibling version
	// must not be listed for delete. Uses the in-repo test-catalog-latest
	// catalog (foo.v0.2.0 / v0.3.0 / v0.3.1 share the same operator image).
	Describe("delete related images shared with sibling operator versions", func() {
		iscFile := filepath.Join("operators", "isc-operator-shared-related.yaml")
		discFile := filepath.Join("operators", "disc-operator-shared-related.yaml")
		const sharedRelatedDigest = "sha256:1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58"

		It("should keep related images still referenced by a sibling version", func() {
			deleteYaml := filepath.Join(workDir, "working-dir", "delete", "delete-images.yaml")

			By("mirroring foo 0.2.0 through 0.3.1 from the custom test catalog")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("generating a delete list for foo 0.2.0 only")
			result, err = runner.DeletePhaseOne(ctx, filepath.Join(iscDir, discFile), workDir, "", testRegistry.Endpoint(),
				"--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)
			expectValidDeleteImagesFiles(workDir, "")

			By("verifying the deleted bundle is listed and the shared related image is not")
			items := parseDeleteImageList(deleteYaml)
			Expect(items).NotTo(BeEmpty(), "expected delete-images.yaml to contain at least one item")

			var sawDeletedBundle bool
			for _, item := range items {
				combined := item.ImageName + " " + item.ImageReference
				if strings.Contains(combined, "foo-bundle-v0.2.0") {
					sawDeletedBundle = true
				}
				Expect(combined).NotTo(ContainSubstring(sharedRelatedDigest),
					"shared related image %s is still referenced by foo 0.3.x and must not be deleted", sharedRelatedDigest)
			}
			Expect(sawDeletedBundle).To(BeTrue(), "expected foo-bundle-v0.2.0 in the delete list")
		})
	})
})

type deleteImageListItem struct {
	ImageName      string `yaml:"imageName"`
	ImageReference string `yaml:"imageReference"`
	Type           string `yaml:"type"`
}

func parseDeleteImageList(path string) []deleteImageListItem {
	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred(), "failed to read %s", path)
	var list struct {
		Items []deleteImageListItem `yaml:"items"`
	}
	Expect(yaml.Unmarshal(data, &list)).To(Succeed(), "failed to parse %s", path)
	return list.Items
}

// collectExpectedNonCatalogRepos returns the expected repos from an ImageSetConfig, excluding
// operator catalog repos. Catalog images are intentionally left behind by delete, both with and
// without --force-cache-delete, as already reflected in expectEmptyRegistry's catalog exception.
func collectExpectedNonCatalogRepos(cfg ImageSetConfiguration) []string {
	catalogs := sets.New[string]()
	for _, op := range cfg.Mirror.Operators {
		catalogs.Insert(extractRepositoryName(op.Catalog))
	}

	var nonCatalog []string
	for _, repo := range collectExpectedRepos(cfg) {
		if !catalogs.Has(repo) {
			nonCatalog = append(nonCatalog, repo)
		}
	}
	return nonCatalog
}

// expectRepositoriesAbsentFromLocalCache verifies that none of the given repositories have any
// remaining tags in the oc-mirror local cache, e.g. after running delete with
// --force-cache-delete=true. Deletion untags manifests but does not immediately garbage-collect
// the underlying blobs/manifests from disk (the same way registry deletion works, see
// expectEmptyRegistry), so this checks for tags rather than mere directory presence.
func expectRepositoriesAbsentFromLocalCache(cacheDir string, expected []string) {
	repos, err := listLocalCacheTaggedRepositories(cacheDir)
	Expect(err).NotTo(HaveOccurred())

	for _, exp := range expected {
		for _, repo := range repos {
			Expect(repo).NotTo(ContainSubstring(exp),
				"repository %q unexpectedly still has tags in local cache (%s)", exp, repo)
		}
	}
}

// listLocalCacheTaggedRepositories walks <cacheDir>/docker/registry/v2/repositories/ and
// returns the repository paths that still have at least one tag under _manifests/tags/. Unlike
// listLocalCacheRepositories, this reflects whether a repository is still reachable by tag,
// regardless of whether its blobs/manifests have been garbage-collected from disk yet.
func listLocalCacheTaggedRepositories(cacheDir string) ([]string, error) {
	reposRoot := filepath.Join(cacheDir, cacheRepositoriesSubdir)

	var repos []string
	err := filepath.Walk(reposRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !info.IsDir() || info.Name() != "tags" || filepath.Base(filepath.Dir(path)) != "_manifests" {
			return nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			repoPath, relErr := filepath.Rel(reposRoot, filepath.Dir(filepath.Dir(path)))
			if relErr != nil {
				return relErr
			}
			repos = append(repos, repoPath)
		}
		return nil
	})
	return repos, err
}
