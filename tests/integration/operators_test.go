package integration_test

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	gcrregistry "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"gopkg.in/yaml.v3"

	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

var _ = Describe("operators", func() {
	var workDir string
	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	Describe("operator with version range (minVersion != maxVersion)", func() {
		iscFile := filepath.Join("operators", "isc-operator-version-range.yaml")

		It("should mirror only operator bundle versions within the range", func() {
			By("running mirrorToMirror with a version range")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the operator catalog is mirrored in the registry")
			expectSuccessfulMirrorInRegistry(filepath.Join(iscDir, iscFile), *testRegistry)

			By("verifying the catalog contains exactly the bundles within the version range")
			expectCatalogBundlesMatchISC(ctx, *testRegistry, filepath.Join(iscDir, iscFile),
				map[string][]string{
					"foo": {"foo.v0.2.0", "foo.v0.3.0", "foo.v0.3.1"},
				})
		})
	})

	Describe("operator with pinned version (minVersion == maxVersion)", func() {
		iscFile := filepath.Join("operators", "isc-operator-pinned-version.yaml")

		It("should mirror only the pinned operator bundle version and generate correct cluster resources", func() {
			By("running mirrorToMirror with a pinned operator version")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying the operator catalog is mirrored in the registry")
			expectSuccessfulMirrorInRegistry(filepath.Join(iscDir, iscFile), *testRegistry)

			By("verifying the catalog contains only the pinned bundle version")
			expectCatalogBundlesMatchISC(ctx, *testRegistry, filepath.Join(iscDir, iscFile),
				map[string][]string{
					"foo": {"foo.v0.3.1"},
				})

			By("verifying the rebuilt catalog excludes packages that were not requested")
			expectCatalogContainsOnlyExpectedPackages(ctx, *testRegistry, filepath.Join(iscDir, iscFile))

			By("verifying IDMS contains all the expected images and no empty fields")
			expectCorrectIDMS(workDir, filepath.Join(iscDir, iscFile))

			By("verifying CatalogSource YAML references the rebuilt catalog in the target registry")
			expectCorrectCatalogSourceYAMLs(workDir, testRegistry.Endpoint(), filepath.Join(iscDir, iscFile))
		})
	})

	Describe("operator with invalid version range (minVersion > maxVersion)", func() {
		iscFile := filepath.Join("operators", "isc-operator-invalid-version-range.yaml")

		It("should fail when minVersion is greater than maxVersion", func() {
			By("running mirrorToMirror with an invalid version range")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandFailure(result, err)

			By("verifying no content was mirrored to the registry")
			expectNoRepositoriesInRegistry(*testRegistry)
		})
	})

	Describe("file permissions after catalog rebuild", func() {
		iscFile := filepath.Join("operators", "isc-operator-version-range.yaml")

		It("should only set executable permissions on graph-preparation and filtered-catalog-image files", func() {
			By("running mirrorToDisk to trigger catalog rebuild")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscFile), workDir, "--remove-signatures=true")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying only allowed files have executable permissions")
			expectOnlyAllowedExecutableFiles(workDir)
		})
	})

	Describe("catalog digest rebuild tagging", func() {
		iscFile := filepath.Join("operators", "isc-operator-catalog-digest.yaml")

		It("should tag rebuilt catalog images with a value matching the manifest digest", func() {
			By("running mirrorToMirror with digest-pinned catalog references")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying mirrored content exists in the registry")
			expectSuccessfulMirrorInRegistry(filepath.Join(iscDir, iscFile), *testRegistry)

			By("verifying the rebuilt catalog tag matches the fetched manifest digest")
			expectRebuiltTagMatchesDigest(ctx, *testRegistry, filepath.Join(iscDir, iscFile))
		})
	})

	// OCPBUGS-33081: a catalog may contain bundles with invalid related images (missing
	// name, missing tag/digest, unsupported oci:// scheme, ...).
	Describe("catalog with a bundle containing an invalid related image", func() {
		iscFile := filepath.Join("operators", "isc-operator-invalid-images.yaml")

		It("handles invalid related image references in a catalog", func() {
			By("running mirrorToMirror against a catalog with one valid and one invalid bundle")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--dest-tls-verify=false")
			expectOcMirrorExitCode(result, err, 4, "collection error", "tag and digest are empty")

			By("verifying no content was mirrored, even though one of the two bundles was valid")
			expectNoRepositoriesInRegistry(*testRegistry)
		})
	})

	Describe("operand platform workaround", Label("operand-platforms"), func() {
		BeforeEach(func() {
			Expect(testRegistry.Stop()).To(Succeed())
			var err error
			testRegistry, err = registry.Start(ctx, filepath.Join(filepath.Dir(registryConfig), "registry-config-sparse.yaml"), 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(testRegistry.WaitReady(ctx, 30*time.Second)).To(Succeed())
		})

		for _, offline := range []bool{false, true} {
			workflow := "mirrorToMirror"
			if offline {
				workflow = "mirrorToDisk then offline diskToMirror"
			}
			It("mirrors available operand platforms and retains the bundle in "+workflow, func() {
				source, isc := operandPlatformCatalog(workDir, "")
				flags := []string{"--src-tls-verify=false", "--dest-tls-verify=false", "--remove-signatures=true"}
				if offline {
					By("archiving a partial operand and skipping an operand with no requested architecture")
					result, err := runner.MirrorToDisk(ctx, isc, workDir, flags...)
					expectOcMirrorCommandSuccess(result, err)
					By("removing online state and disconnecting the source before diskToMirror")
					Expect(os.RemoveAll(filepath.Join(workDir, dirWorkingDir))).To(Succeed())
					Expect(os.RemoveAll(cacheDir)).To(Succeed())
					source.Close()
					result, err = runner.DiskToMirror(ctx, isc, workDir, testRegistry.Endpoint(), flags...)
					expectOcMirrorCommandSuccess(result, err)
					Expect(result.Stdout + result.Stderr).To(ContainSubstring("Excluding 1 image(s)"))
				} else {
					result, err := runner.MirrorToMirror(ctx, isc, workDir, testRegistry.Endpoint(), flags...)
					expectOcMirrorCommandSuccess(result, err)
				}
				By("verifying only requested, available instances were copied, including the bundle")
				expectOnlyPlatformBlobsPresent(ctx, testRegistry, "operand-test/operand", "latest", []string{"linux/amd64"})
				for _, repo := range []string{"controller", "bundle"} {
					expectOnlyPlatformBlobsPresent(ctx, testRegistry, "operand-test/"+repo, "latest", []string{"linux/amd64", "linux/arm64"})
				}
				repos, err := testRegistry.ListRepositories(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(repos).NotTo(ContainElement("operand-test/optional"))
			})
		}

		for _, required := range []string{"bundle", "controller"} {
			It("fails rather than skipping a "+required+" missing a requested platform", func() {
				_, isc := operandPlatformCatalog(workDir, required)
				result, err := runner.MirrorToMirror(ctx, isc, workDir, testRegistry.Endpoint(),
					"--src-tls-verify=false", "--dest-tls-verify=false", "--remove-signatures=true")
				expectOcMirrorCommandFailure(result, err)
				Expect(result.Stdout + result.Stderr).To(ContainSubstring("no instances found for platform linux/arm64"))
			})
		}
	})
})

// Build tiny local fixtures instead of depending on the large, authenticated
// kubevirt catalog. The optional image models the PR's architecture-only operand.
func operandPlatformCatalog(workDir, missing string) (*httptest.Server, string) {
	source := httptest.NewServer(gcrregistry.New(gcrregistry.Logger(log.New(io.Discard, "", 0))))
	DeferCleanup(source.Close)
	host := strings.TrimPrefix(source.URL, "http://")
	arches := []string{"amd64", "arm64"}
	controllerArches, bundleArches := arches, arches
	if missing == "controller" {
		controllerArches = []string{"amd64"}
	}
	if missing == "bundle" {
		bundleArches = []string{"amd64"}
	}
	controller := pushOperatorPlatformImage(host, "controller", controllerArches, nil, nil)
	operand := pushOperatorPlatformImage(host, "operand", []string{"amd64", "s390x"}, nil, nil)
	optional := pushOperatorPlatformImage(host, "optional", []string{"s390x"}, nil, nil)
	csv := fmt.Sprintf(`apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: operand-test.v1.0.0
spec:
  install:
    strategy: deployment
    spec:
      deployments:
      - name: controller
        spec:
          template:
            spec:
              containers:
              - name: controller
                image: %s
`, controller)
	bundle := pushOperatorPlatformImage(host, "bundle", bundleArches,
		map[string]string{"manifests/test.clusterserviceversion.yaml": csv},
		map[string]string{"operators.operatorframework.io.bundle.manifests.v1": "manifests"})
	catalog := fmt.Sprintf(`schema: olm.package
name: operand-test
defaultChannel: stable
---
schema: olm.channel
name: stable
package: operand-test
entries:
- name: operand-test.v1.0.0
---
schema: olm.bundle
name: operand-test.v1.0.0
package: operand-test
image: %s
properties:
- type: olm.package
  value: {packageName: operand-test, version: 1.0.0}
relatedImages:
- {name: bundle, image: %s}
- {name: controller, image: %s}
- {name: operand, image: %s}
- {name: optional, image: %s}
`, bundle, bundle, controller, operand, optional)
	catalogRef := pushOperatorPlatformImage(host, "catalog", arches,
		map[string]string{"configs/operand-test/catalog.yaml": catalog},
		map[string]string{"operators.operatorframework.io.index.configs.v1": "/configs"})
	isc := fmt.Sprintf(`kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v2alpha1
mirror:
  operators:
  - catalog: %s
    packages: [{name: operand-test}]
    platforms:
    - {os: linux, architecture: amd64}
    - {os: linux, architecture: arm64}
`, catalogRef)
	iscPath := filepath.Join(workDir, "isc.yaml")
	Expect(os.WriteFile(iscPath, []byte(isc), 0o600)).To(Succeed())
	return source, iscPath
}

func pushOperatorPlatformImage(host, repo string, arches []string, files, labels map[string]string) string {
	var contents bytes.Buffer
	tw := tar.NewWriter(&contents)
	for path, data := range files {
		Expect(tw.WriteHeader(&tar.Header{Name: path, Mode: 0o644, Size: int64(len(data))})).To(Succeed())
		_, err := tw.Write([]byte(data))
		Expect(err).NotTo(HaveOccurred())
	}
	Expect(tw.Close()).To(Succeed())
	layer, err := tarball.LayerFromReader(bytes.NewReader(contents.Bytes()))
	Expect(err).NotTo(HaveOccurred())
	var instances []mutate.IndexAddendum
	for _, arch := range arches {
		img, err := mutate.AppendLayers(empty.Image, layer)
		Expect(err).NotTo(HaveOccurred())
		cfg, err := img.ConfigFile()
		Expect(err).NotTo(HaveOccurred())
		cfg.OS, cfg.Architecture, cfg.Config.Labels = "linux", arch, labels
		img, err = mutate.ConfigFile(img, cfg)
		Expect(err).NotTo(HaveOccurred())
		instances = append(instances, mutate.IndexAddendum{Add: img, Descriptor: v1.Descriptor{
			Platform: &v1.Platform{OS: "linux", Architecture: arch},
		}})
	}
	ref, err := name.NewTag(host+"/operand-test/"+repo+":latest", name.Insecure)
	Expect(err).NotTo(HaveOccurred())
	index := mutate.IndexMediaType(mutate.AppendManifests(empty.Index, instances...), types.DockerManifestList)
	Expect(remote.WriteIndex(ref, index, remote.WithContext(ctx))).To(Succeed())
	return ref.Name()
}

// expectCatalogContainsOnlyExpectedPackages verifies that the rebuilt catalog in the registry
// contains configs for exactly the packages selected in the ISC, and no others. This guards
// against a regression where oc-mirror copies the source catalog unfiltered instead of rebuilding
// a filtered index (OCPBUGS-56718).
func expectCatalogContainsOnlyExpectedPackages(ctx context.Context, reg registry.Registry, iscPath string) {
	cfg := parseImageSetConfig(iscPath)

	for _, op := range cfg.Mirror.Operators {
		configsDir := extractCatalogConfigs(ctx, reg, op.Catalog)
		DeferCleanup(func() {
			Expect(os.RemoveAll(configsDir)).To(Succeed(), "failed to clean up catalog configs dir %s", configsDir)
		})

		entries, err := os.ReadDir(configsDir)
		Expect(err).NotTo(HaveOccurred(), "failed to read catalog configs dir %s", configsDir)

		var actualPackages []string
		for _, entry := range entries {
			if entry.IsDir() {
				actualPackages = append(actualPackages, entry.Name())
			}
		}

		var expectedPackages []string
		for _, pkg := range op.Packages {
			expectedPackages = append(expectedPackages, pkg.Name)
		}

		Expect(actualPackages).To(ConsistOf(expectedPackages),
			"rebuilt catalog for %q should contain only the packages selected in the ISC", op.Catalog)
	}
}

// expectCorrectCatalogSourceYAMLs reads the CatalogSource YAML files generated by oc-mirror and
// verifies that every operator catalog in the ISC has a corresponding CatalogSource whose
// spec.image references the rebuilt catalog in the target registry (not the source registry).
func expectCorrectCatalogSourceYAMLs(workDir, targetRegistry, iscPath string) {
	pattern := filepath.Join(workDir, dirWorkingDir, dirClusterResources, "cs-*.yaml")
	matches, err := filepath.Glob(pattern)
	Expect(err).NotTo(HaveOccurred(), "failed to list CatalogSource files")
	Expect(matches).NotTo(BeEmpty(), "no CatalogSource YAML files found matching %s", pattern)

	var images []string
	for _, csFile := range matches {
		data, err := os.ReadFile(csFile)
		Expect(err).NotTo(HaveOccurred(), "failed to read CatalogSource file: %s", csFile)

		var raw map[string]any
		Expect(yaml.Unmarshal(data, &raw)).To(Succeed(), "failed to parse CatalogSource document: %s", csFile)

		jsonBytes, err := json.Marshal(raw)
		Expect(err).NotTo(HaveOccurred(), "failed to marshal CatalogSource document to JSON: %s", csFile)

		var cs operatorv1alpha1.CatalogSource
		Expect(json.Unmarshal(jsonBytes, &cs)).To(Succeed(), "failed to unmarshal CatalogSource: %s", csFile)

		Expect(cs.Kind).To(Equal("CatalogSource"), "unexpected kind in %s", csFile)
		Expect(cs.Spec.Image).NotTo(BeEmpty(), "CatalogSource %s has empty spec.image", csFile)

		images = append(images, cs.Spec.Image)
	}

	cfg := parseImageSetConfig(iscPath)
	for _, op := range cfg.Mirror.Operators {
		expectedRepo := targetRegistry + "/" + extractRepositoryName(op.Catalog)
		Expect(catalogSourceImagesCoverRepo(images, expectedRepo)).To(BeTrue(),
			"no CatalogSource image references the rebuilt catalog %q; images: %v", expectedRepo, images)
	}
}

// catalogSourceImagesCoverRepo returns true if any image reference points at the given
// repository, tagged or by digest.
func catalogSourceImagesCoverRepo(images []string, repo string) bool {
	for _, image := range images {
		if strings.HasPrefix(image, repo+":") || strings.HasPrefix(image, repo+"@") {
			return true
		}
	}
	return false
}
