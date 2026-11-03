package integration_test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("operator catalog destinations", func() {
	var workDir string
	BeforeEach(func() { workDir = setupWorkDir() })
	AfterEach(func() { cleanupWorkDir(workDir) })

	for _, viaDisk := range []bool{false, true} {
		workflow := "mirrorToMirror"
		if viaDisk {
			workflow = "mirrorToDisk + diskToMirror"
		}
		Context(workflow, func() {
			DescribeTable("copies catalogs to the requested repository and tag", func(specCtx SpecContext, local bool, targetCatalog, targetTag, destination string) {
				op := operatorFilter("isc-package-heads.yaml")
				var ociDir string
				if local {
					ociDir = writeLocalCatalog(workDir, "local-catalog", publishedCatalogImage())
					op.Catalog = "oci://" + ociDir
				}
				op.TargetCatalog = targetCatalog
				op.TargetTag = targetTag
				isc := writeOperatorConfig(workDir, op)
				expected := []catalogExpectation{{Destination: destination, Source: happyPathCatalog,
					Packages: map[string]catalogPackageExpectation{"foo": headCatalogPackages()["foo"]}}}
				By("mirroring the catalog with the requested destination overrides")
				mirrorOperatorCatalogs(specCtx, workDir, isc, viaDisk, expected, catalogWorkflowOptions{
					BeforeImport: func() {
						if local {
							Expect(os.RemoveAll(ociDir)).To(Succeed())
						}
					},
				})
				By("checking the requested catalog destination, absent default destinations and exact resource references")
				expectOperatorCatalogs(workDir, *testRegistry, nil, expected...)
			},
				Entry("changes only the repository for a registry catalog", SpecTimeout(4*time.Minute),
					false, "custom/catalog", "", "custom/catalog:test-catalog-latest"),
				Entry("changes only the tag for a registry catalog", SpecTimeout(4*time.Minute),
					false, "", "selected", testCatalogRepo+":selected"),
				Entry("changes repository and tag for a registry catalog", SpecTimeout(4*time.Minute),
					false, "custom/catalog", "selected", "custom/catalog:selected"),
				Entry("changes only the repository for an OCI catalog", SpecTimeout(4*time.Minute),
					true, "custom/catalog", "", "custom/catalog:latest"),
				Entry("changes only the tag for an OCI catalog", SpecTimeout(4*time.Minute),
					true, "", "selected", "local-catalog:selected"),
				Entry("changes repository and tag for an OCI catalog", SpecTimeout(4*time.Minute),
					true, "custom/catalog", "selected", "custom/catalog:selected"),
			)
		})
	}
})
