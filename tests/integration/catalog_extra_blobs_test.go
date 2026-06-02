package integration_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

var _ = Describe("extra blobs", func() {
	var workDir string
	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	Describe("catalog with extra blobs", func() {
		iscFile := filepath.Join("operators", "isc-operator-extra-blobs.yaml")
		hasExtraBlobs := map[string]bool{"foo": false, "bar": true, "baz": false, "": true}

		It("should keep blobs of filtered-in packages", func() {
			By("running mirrorToMirror")
			result, err := runner.MirrorToMirror(ctx, filepath.Join(iscDir, iscFile), workDir, testRegistry.Endpoint(),
				"--remove-signatures=true", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying mirrored content exists in the registry")
			expectSuccessfulMirrorInRegistry(filepath.Join(iscDir, iscFile), *testRegistry)

			By("verifying that extra blobs are kept in mirrored content")
			expectOtherBlobsInCatalog(filepath.Join(iscDir, iscFile), *testRegistry, hasExtraBlobs)
		})
	})
})

func expectOtherBlobsInCatalog(iscPath string, reg registry.Registry, pkgHasExtraBlobs map[string]bool) {
	cfg := parseImageSetConfig(iscPath)

	const lifecycleSchema string = "io.openshift.operators.lifecycles.v1alpha1"
	for _, op := range cfg.Mirror.Operators {
		configsDir := extractCatalogConfigs(ctx, reg, op.Catalog)

		for _, pkg := range op.Packages {
			extras := loadOtherBlobs(ctx, configsDir, pkg.Name)
			if hasExtras, ok := pkgHasExtraBlobs[pkg.Name]; ok && hasExtras {
				Expect(extras).NotTo(BeEmpty(), "%s extra blobs missing", pkg.Name)
				Expect(extras).Should(ContainElement(HaveField("Schema", lifecycleSchema)))
			} else {
				Expect(extras).To(BeEmpty(), "unexpected blobs found for %s", pkg.Name)
			}
		}

		// catalog extra blobs, not belonging to packages
		extras := loadOtherBlobs(ctx, configsDir, "")
		if hasExtras, ok := pkgHasExtraBlobs[""]; ok && hasExtras {
			Expect(extras).ToNot(BeEmpty(), "catalog extra blobs missing")
			Expect(extras).Should(ContainElement(HaveField("Schema", lifecycleSchema)))
		} else {
			Expect(extras).To(BeEmpty(), "unexpected catalog extra blobs found")
		}

		os.RemoveAll(configsDir)
	}
}
