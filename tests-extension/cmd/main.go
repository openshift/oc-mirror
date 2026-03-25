package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/component-base/logs"

	"github.com/openshift-eng/openshift-tests-extension/pkg/cmd"
	e "github.com/openshift-eng/openshift-tests-extension/pkg/extension"
	et "github.com/openshift-eng/openshift-tests-extension/pkg/extension/extensiontests"
	g "github.com/openshift-eng/openshift-tests-extension/pkg/ginkgo"
	"github.com/openshift/origin/test/extended/util"
	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	framework "k8s.io/kubernetes/test/e2e/framework"

	// Import testdata package from this module
	_ "github.com/openshift/oc-mirror-tests-extension/test/e2e/testdata"

	// Import test packages from this module
	_ "github.com/openshift/oc-mirror-tests-extension/test/e2e"
)

func main() {
	// Initialize test framework flags (required for kubeconfig, provider, etc.)
	util.InitStandardFlags()
	framework.AfterReadingAllFlags(&framework.TestContext)

	logs.InitLogs()
	defer logs.FlushLogs()

	registry := e.NewRegistry()
	ext := e.NewExtension("openshift", "payload", "oc-mirror")

	// Register test suites (parallel, serial, disruptive, all)
	registerSuites(ext)

	// Build test specs from Ginkgo
	allSpecs, err := g.BuildExtensionTestSpecsFromOpenShiftGinkgoSuite()
	if err != nil {
		panic(fmt.Sprintf("couldn't build extension test specs from ginkgo: %+v", err.Error()))
	}

	// Filter to only include tests from this module's test/e2e/ directory
	componentSpecs := allSpecs.Select(func(spec *et.ExtensionTestSpec) bool {
		for _, loc := range spec.CodeLocations {
			if strings.Contains(loc, "/test/e2e/") && !strings.Contains(loc, "/go/pkg/mod/") && !strings.Contains(loc, "/vendor/") {
				return true
			}
		}
		return false
	})

	// Initialize test framework before all tests.
	// util.WithCleanup sets testsStarted=true, which is required by util.requiresTestStart()
	// inside the BeforeEach registered by compat_otp.NewCLI (via SetupProject).
	componentSpecs.AddBeforeAll(func() {
		util.WithCleanup(func() {
			if err := compat_otp.InitTest(false); err != nil {
				panic(err)
			}
		})
	})

	// Process all specs
	componentSpecs.Walk(func(spec *et.ExtensionTestSpec) {
		spec.Lifecycle = et.LifecycleInforming
	})

	ext.AddSpecs(componentSpecs)
	registry.Register(ext)

	root := &cobra.Command{
		Long: "Oc-mirror Tests",
	}

	root.AddCommand(cmd.DefaultExtensionCommands(registry)...)

	if err := func() error {
		return root.Execute()
	}(); err != nil {
		os.Exit(1)
	}
}

func registerSuites(ext *e.Extension) {
	suites := []e.Suite{
		{
			Name:    "oc-mirror/conformance/parallel",
			Parents: []string{"openshift/conformance/parallel"},
			Description: "Parallel conformance tests (Level0, non-serial, non-disruptive)",
			Qualifiers: []string{
				`name.contains("[Level0]") && !(name.contains("[Serial]") || name.contains("[Disruptive]"))`,
			},
		},
		{
			Name:    "oc-mirror/conformance/serial",
			Parents: []string{"openshift/conformance/serial"},
			Description: "Serial conformance tests (must run sequentially)",
			Qualifiers: []string{
				`name.contains("[Level0]") && name.contains("[Serial]") && !name.contains("[Disruptive]")`,
			},
		},
		{
			Name:        "oc-mirror/disruptive",
			Parents:     []string{"openshift/disruptive"},
			Description: "Disruptive tests (may affect cluster state)",
			Qualifiers: []string{
				`name.contains("[Disruptive]")`,
			},
		},
		{
			Name:        "oc-mirror/non-disruptive",
			Description: "All non-disruptive tests (safe for development clusters)",
			Qualifiers: []string{
				`!name.contains("[Disruptive]")`,
			},
		},
		{
			Name:        "oc-mirror/all",
			Description: "All oc-mirror tests",
		},
	}

	for _, suite := range suites {
		ext.AddSuite(suite)
	}
}