package integration_test

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("operator CatalogSource templates", func() {
	var workDir string
	BeforeEach(func() { workDir = setupWorkDir() })
	AfterEach(func() { cleanupWorkDir(workDir) })

	DescribeTable("preserves custom fields while replacing template placeholders", func(specCtx SpecContext, viaDisk bool) {
		op := operatorFilter("isc-package-heads.yaml")
		var err error
		op.TargetCatalogSourceTemplate, err = filepath.Abs(filepath.Join(filepath.Dir(iscDir), "catalog-source-templates", "custom.yaml"))
		Expect(err).NotTo(HaveOccurred())
		op.TargetCatalog = "custom/catalog"
		op.TargetTag = "selected"
		isc := writeOperatorConfig(workDir, op)
		expected := []catalogExpectation{{Destination: "custom/catalog:selected", Source: happyPathCatalog,
			Packages: map[string]catalogPackageExpectation{"foo": headCatalogPackages()["foo"]}}}
		mirrorOperatorCatalogs(specCtx, workDir, isc, viaDisk, expected, catalogWorkflowOptions{})
		sources := expectOperatorCatalogs(workDir, *testRegistry, nil, expected...)
		image := testRegistry.Endpoint() + "/custom/catalog:selected"
		cs := sources[image]
		Expect(cs.Name).To(Equal("cs-catalog-selected"))
		Expect(cs.Namespace).To(Equal("openshift-marketplace"))
		Expect(cs.Labels).To(HaveKeyWithValue("integration.oc-mirror/custom", "preserved"))
		Expect(cs.Annotations).To(HaveKeyWithValue("integration.oc-mirror/note", "preserved"))
		Expect(cs.Spec.DisplayName).To(Equal("Integration Catalog"))
		Expect(cs.Spec.Publisher).To(Equal("oc-mirror tests"))
		Expect(cs.Spec.UpdateStrategy).NotTo(BeNil())
		Expect(cs.Spec.UpdateStrategy.RegistryPoll).NotTo(BeNil())
		Expect(cs.Spec.UpdateStrategy.RegistryPoll.Interval).NotTo(BeNil())
		Expect(cs.Spec.UpdateStrategy.RegistryPoll.Interval.Duration).To(Equal(30 * time.Minute))
	},
		Entry("in mirrorToMirror", SpecTimeout(4*time.Minute), false),
		Entry("in mirrorToDisk + diskToMirror", SpecTimeout(4*time.Minute), true),
	)

	It("succeeds with a default CatalogSource and a diagnostic when the template is missing", SpecTimeout(3*time.Minute), func(specCtx SpecContext) {
		op := operatorFilter("isc-package-heads.yaml")
		op.TargetCatalogSourceTemplate = filepath.Join(workDir, "missing-template.yaml")
		isc := writeOperatorConfig(workDir, op)
		expected := []catalogExpectation{{Destination: testCatalogRepo + ":test-catalog-latest", Source: happyPathCatalog,
			Packages: map[string]catalogPackageExpectation{"foo": headCatalogPackages()["foo"]}}}
		result, err := runner.MirrorToMirror(specCtx, isc, workDir, testRegistry.Endpoint(),
			"--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		Expect(result.Stdout+result.Stderr).To(ContainSubstring(op.TargetCatalogSourceTemplate),
			"the diagnostic should identify the missing template")
		sources := expectOperatorCatalogs(workDir, *testRegistry, nil, expected...)
		image := testRegistry.Endpoint() + "/" + expected[0].Destination
		cs := sources[image]
		Expect(cs.Spec.DisplayName).To(BeEmpty())
		Expect(cs.Spec.Publisher).To(BeEmpty())
		Expect(cs.Spec.UpdateStrategy).To(BeNil())
	})
})
