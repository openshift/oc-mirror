package integration_test

import (
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	ocv1 "github.com/operator-framework/operator-controller/api/v1"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"gopkg.in/yaml.v3"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

type catalogPackageExpectation struct {
	DefaultChannel string
	Channels       map[string][]string
}

type catalogExpectation struct {
	Destination string // repository:tag, without the destination registry
	Source      string // original catalog reference, or a digest reference for a local variant
	Full        bool   // the original catalog must be copied unchanged
	Packages    map[string]catalogPackageExpectation
}

// Expectations are independent of ISC parsing and of the catalog returned by oc-mirror.
func headCatalogPackages() map[string]catalogPackageExpectation {
	return map[string]catalogPackageExpectation{
		"foo": {"beta", map[string][]string{"beta": {"foo.v0.3.1"}}},
		"bar": {"stable", map[string][]string{"alpha": {"bar.v1.0.0"}, "stable": {"bar.v1.0.0"}}},
		"baz": {"stable", map[string][]string{"stable": {"baz.v1.1.0"}}},
	}
}

func fullCatalogPackages() map[string]catalogPackageExpectation {
	return map[string]catalogPackageExpectation{
		"foo": {"beta", map[string][]string{"beta": {"foo.v0.1.0", "foo.v0.2.0", "foo.v0.3.0", "foo.v0.3.1"}}},
		"bar": {"stable", map[string][]string{"alpha": {"bar.v0.1.0", "bar.v0.2.0", "bar.v1.0.0"}, "stable": {"bar.v1.0.0"}}},
		"baz": {"stable", map[string][]string{"stable": {"baz.v1.0.0", "baz.v1.0.1", "baz.v1.1.0"}}},
	}
}

func catalogBundleNames(packages map[string]catalogPackageExpectation) []string {
	versions := map[string]bool{}
	for _, pkg := range packages {
		for _, entries := range pkg.Channels {
			for _, entry := range entries {
				versions[entry] = true
			}
		}
	}
	return slices.Sorted(maps.Keys(versions))
}

func catalogBundleSource(bundle string) string {
	pkg, version, found := strings.Cut(bundle, ".v")
	Expect(found).To(BeTrue(), "invalid expected bundle name %s", bundle)
	return "quay.io/" + testCatalogRepo + ":" + pkg + "-bundle-v" + version
}

func operatorImages(expected []catalogExpectation) map[string]string {
	images := map[string]string{}
	for _, catalog := range expected {
		images[catalog.Destination] = "" // a rebuilt catalog has a new digest
		if catalog.Full {
			images[catalog.Destination] = catalog.Source
		}
		for _, bundle := range catalogBundleNames(catalog.Packages) {
			source := catalogBundleSource(bundle)
			images[strings.TrimPrefix(source, "quay.io/")] = source
		}
	}
	// Every bundle in this fixture catalog references the same operand image.
	images[testCatalogRepo+":sha256-"+strings.TrimPrefix(fooOperandDigest, "sha256:")] = operandImage
	return images
}

// expectOperatorCatalogs checks the final FBC, not the unfiltered collection artifacts.
// extraImages is used by the existing full-catalog signature test, which also mirrors a release.
func expectOperatorCatalogs(workDir string, reg registry.Registry, extraImages map[string]string, expected ...catalogExpectation) map[string]operatorv1alpha1.CatalogSource {
	GinkgoHelper()
	images := operatorImages(expected)
	maps.Copy(images, extraImages)
	expectRegistryContainsExactly(reg, images)
	wantedRepos := map[string]bool{}
	for destination := range images {
		repo, _, _ := strings.Cut(destination, ":")
		wantedRepos[repo] = true
	}
	repos, err := reg.ListRepositories(ctx)
	Expect(err).NotTo(HaveOccurred())
	Expect(repos).To(ConsistOf(slices.Collect(maps.Keys(wantedRepos))), "unexpected destination repositories")
	selected := map[string]bool{}
	for _, catalog := range expected {
		ref, err := name.NewTag(reg.Endpoint()+"/"+catalog.Destination, name.Insecure)
		Expect(err).NotTo(HaveOccurred())
		img, err := remote.Image(ref, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))
		Expect(err).NotTo(HaveOccurred())
		configs := extractCatalogImageConfigs(img)
		dc, err := declcfg.LoadFS(ctx, os.DirFS(configs))
		Expect(err).NotTo(HaveOccurred())
		expectCatalogSelection(dc, catalog)
		for _, bundle := range catalogBundleNames(catalog.Packages) {
			selected[bundle] = true
		}
	}
	for destination, source := range images {
		if source == "" {
			continue
		}
		repo, _, _ := strings.Cut(destination, ":")
		// Release payloads may be indexes; their existing assertions cover their contents.
		if repo == testCatalogRepo {
			expectImageReadableByDigest(ctx, reg, repo, remoteDigest(source))
		}
	}
	for _, bundle := range catalogBundleNames(fullCatalogPackages()) {
		if !selected[bundle] {
			exists, err := reg.ManifestExistsByDigest(ctx, testCatalogRepo, remoteDigest(catalogBundleSource(bundle)))
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse(), "unselected bundle %s was published", bundle)
		}
	}
	var catalogImages []string
	for _, catalog := range expected {
		catalogImages = append(catalogImages, reg.Endpoint()+"/"+catalog.Destination)
	}
	return expectCatalogResources(workDir, catalogImages)
}

func expectCatalogSelection(dc *declcfg.DeclarativeConfig, expected catalogExpectation) {
	GinkgoHelper()
	var packages, channels, bundles []string
	for _, pkg := range dc.Packages {
		packages = append(packages, pkg.Name)
		wanted, ok := expected.Packages[pkg.Name]
		Expect(ok).To(BeTrue(), "unexpected package %s", pkg.Name)
		Expect(pkg.DefaultChannel).To(Equal(wanted.DefaultChannel))
	}
	Expect(packages).To(ConsistOf(slices.Collect(maps.Keys(expected.Packages))))
	var wantedChannels []string
	for pkg, wanted := range expected.Packages {
		for channel := range wanted.Channels {
			wantedChannels = append(wantedChannels, pkg+"/"+channel)
		}
	}
	for _, channel := range dc.Channels {
		channels = append(channels, channel.Package+"/"+channel.Name)
		var entries []string
		for _, entry := range channel.Entries {
			entries = append(entries, entry.Name)
		}
		Expect(entries).To(ConsistOf(expected.Packages[channel.Package].Channels[channel.Name]),
			"unexpected entries in %s/%s", channel.Package, channel.Name)
	}
	Expect(channels).To(ConsistOf(wantedChannels))
	for _, bundle := range dc.Bundles {
		bundles = append(bundles, bundle.Name)
		source := catalogBundleSource(bundle.Name)
		Expect(bundle.Image).To(Equal(source))
		var related []string
		for _, image := range bundle.RelatedImages {
			related = append(related, image.Image)
		}
		Expect(related).To(ConsistOf(source, operandImage))
	}
	Expect(bundles).To(ConsistOf(catalogBundleNames(expected.Packages)))
}

// Decode every document and reject extra resources, including duplicate image references.
func expectCatalogResources(workDir string, expectedImages []string) map[string]operatorv1alpha1.CatalogSource {
	GinkgoHelper()
	sources := map[string]operatorv1alpha1.CatalogSource{}
	for _, kind := range []string{"cs", "cc"} {
		files, err := filepath.Glob(filepath.Join(workDir, dirWorkingDir, dirClusterResources, kind+"-*.yaml"))
		Expect(err).NotTo(HaveOccurred())
		var images []string
		for _, file := range files {
			f, err := os.Open(file)
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(f.Close()).To(Succeed()) }()
			decoder := yamlutil.NewYAMLOrJSONDecoder(f, 4096)
			for {
				var cs operatorv1alpha1.CatalogSource
				var cc ocv1.ClusterCatalog
				var resource any = &cs
				if kind == "cc" {
					resource = &cc
				}
				if err := decoder.Decode(resource); err == io.EOF {
					break
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
				if kind == "cs" {
					images = append(images, cs.Spec.Image)
					sources[cs.Spec.Image] = cs
				} else {
					Expect(cc.Spec.Source.Image).NotTo(BeNil())
					images = append(images, cc.Spec.Source.Image.Ref)
				}
			}
		}
		Expect(images).To(ConsistOf(expectedImages), "unexpected %s resources", kind)
	}
	return sources
}

func operatorFilter(fixture string) OperatorConfig {
	cfg := parseImageSetConfig(filepath.Join(iscDir, "operator_selection", fixture))
	Expect(cfg.Mirror.Operators).To(HaveLen(1))
	return cfg.Mirror.Operators[0]
}

func writeOperatorConfig(workDir string, operators ...OperatorConfig) string {
	data, err := yaml.Marshal(map[string]any{
		"apiVersion": "mirror.openshift.io/v2alpha1", "kind": "ImageSetConfiguration",
		"mirror": map[string]any{"operators": operators},
	})
	Expect(err).NotTo(HaveOccurred())
	file := filepath.Join(workDir, "isc.yaml")
	Expect(os.WriteFile(file, data, 0o600)).To(Succeed())
	return file
}

// The archive carries the original catalog as well as its filtered copy.
func expectOperatorArchive(workDir string, expected []catalogExpectation) {
	GinkgoHelper()
	archived := operatorImages(expected)
	var rebuilt []string
	for _, catalog := range expected {
		archived[catalog.Destination] = catalog.Source
		if !catalog.Full {
			repo, _, _ := strings.Cut(catalog.Destination, ":")
			rebuilt = append(rebuilt, repo)
		}
	}
	expectArchiveContainsExactly(workDir, archived, rebuilt...)
}

type catalogWorkflowOptions struct {
	Env          []string
	BeforeImport func()
}

// Disk workflows check the archive before importing it into a fresh, offline cache.
// BeforeImport lets a spec remove its OCI source without changing the direct workflow.
func mirrorOperatorCatalogs(specCtx SpecContext, workDir, isc string, viaDisk bool, expected []catalogExpectation, options catalogWorkflowOptions) {
	GinkgoHelper()
	command := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).WithEnv(options.Env)
	if !viaDisk {
		By("mirroring directly to the destination registry")
		result, err := command.MirrorToMirror(specCtx, isc, workDir, testRegistry.Endpoint(),
			"--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)
		return
	}

	By("mirroring to disk and checking the archive")
	result, err := command.MirrorToDisk(specCtx, isc, workDir, "--remove-signatures=true")
	expectOcMirrorCommandSuccess(result, err)
	expectOperatorArchive(workDir, expected)
	expectNoRepositoriesInRegistry(*testRegistry)
	expectNoClusterResources(workDir)
	if options.BeforeImport != nil {
		options.BeforeImport()
	}

	By("importing into a fresh cache without access to the original source")
	env := append(slices.Clone(options.Env),
		"HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1", "NO_PROXY=localhost,127.0.0.1")
	restore := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).WithEnv(env)
	result, err = restore.DiskToMirror(specCtx, isc, workDir, testRegistry.Endpoint(),
		"--cache-dir", filepath.Join(workDir, "restore-cache"), "--remove-signatures=true", "--dest-tls-verify=false", "--retry-times=1")
	expectOcMirrorCommandSuccess(result, err)
}
