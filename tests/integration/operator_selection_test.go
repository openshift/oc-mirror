package integration_test

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("operator catalog selection", func() {
	var workDir string
	BeforeEach(func() { workDir = setupWorkDir() })
	AfterEach(func() { cleanupWorkDir(workDir) })

	DescribeTable("mirrors the selected packages, channels and images", func(specCtx SpecContext, fixture string, packages map[string]catalogPackageExpectation) {
		isc := filepath.Join(iscDir, "operator_selection", fixture)
		expected := []catalogExpectation{{
			Destination: testCatalogRepo + ":test-catalog-latest", Source: happyPathCatalog, Packages: packages,
		}}
		By("mirroring the selection directly to the destination registry")
		result, err := runner.MirrorToMirror(specCtx, isc, workDir, testRegistry.Endpoint(),
			"--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		By("checking the exact final catalog, copied images, exclusions and resources")
		expectOperatorCatalogs(workDir, *testRegistry, nil, expected...)
	},
		Entry("mirrors channel heads when no filters are specified", SpecTimeout(3*time.Minute),
			"isc-heads.yaml", headCatalogPackages()),
		Entry("mirrors all versions of selected packages with full: true", SpecTimeout(3*time.Minute),
			"isc-full-packages.yaml", map[string]catalogPackageExpectation{"foo": fullCatalogPackages()["foo"]}),
		Entry("selects a channel and replaces the original default channel", SpecTimeout(3*time.Minute),
			"isc-default-channel.yaml", map[string]catalogPackageExpectation{
				"bar": {"alpha", map[string][]string{"alpha": {"bar.v1.0.0"}}},
			}),
		Entry("applies a package-level version range and excludes the original head", SpecTimeout(3*time.Minute),
			"isc-package-range.yaml", map[string]catalogPackageExpectation{
				"baz": {"stable", map[string][]string{"stable": {"baz.v1.0.0", "baz.v1.0.1"}}},
			}),
		Entry("applies a package-level range to a named channel", SpecTimeout(3*time.Minute),
			"isc-package-channel-range.yaml", map[string]catalogPackageExpectation{
				"bar": {"alpha", map[string][]string{"alpha": {"bar.v0.2.0", "bar.v1.0.0"}}},
			}),
	)
})
