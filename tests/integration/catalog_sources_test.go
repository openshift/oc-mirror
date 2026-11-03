package integration_test

import (
	"archive/tar"
	"bytes"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
)

var _ = Describe("operator catalog sources", func() {
	var workDir string
	BeforeEach(func() { workDir = setupWorkDir() })
	AfterEach(func() { cleanupWorkDir(workDir) })

	for _, viaDisk := range []bool{false, true} {
		workflow := "mirrorToMirror"
		if viaDisk {
			workflow = "mirrorToDisk + diskToMirror"
		}
		Context(workflow, func() {
			It("mirrors mixed registry and OCI catalogs with independent selections", SpecTimeout(4*time.Minute), func(specCtx SpecContext) {
				dir := writeLocalCatalog(workDir, "local-catalog", publishedCatalogImage())
				registryCatalog := operatorFilter("isc-heads.yaml")
				full := true
				registryCatalog.Full = &full
				localCatalog := operatorFilter("isc-full-packages.yaml")
				localCatalog.Catalog = "oci://" + dir
				expected := []catalogExpectation{
					{Destination: testCatalogRepo + ":test-catalog-latest", Source: happyPathCatalog, Full: true, Packages: fullCatalogPackages()},
					{Destination: "local-catalog:latest", Source: happyPathCatalog,
						Packages: map[string]catalogPackageExpectation{"foo": fullCatalogPackages()["foo"]}},
				}
				isc := writeOperatorConfig(workDir, registryCatalog, localCatalog)
				mirrorOperatorCatalogs(specCtx, workDir, isc, viaDisk, expected, catalogWorkflowOptions{
					BeforeImport: func() { Expect(os.RemoveAll(dir)).To(Succeed()) },
				})
				expectOperatorCatalogs(workDir, *testRegistry, nil, expected...)
			})

			It("filters a local OCI catalog without requiring its source directory during import", SpecTimeout(4*time.Minute), func(specCtx SpecContext) {
				dir := writeLocalCatalog(workDir, "local-catalog", publishedCatalogImage())
				op := operatorFilter("isc-package-channel-range.yaml")
				op.Catalog = "oci://" + dir
				expected := []catalogExpectation{{Destination: "local-catalog:latest", Source: happyPathCatalog,
					Packages: map[string]catalogPackageExpectation{
						"bar": {"alpha", map[string][]string{"alpha": {"bar.v0.2.0", "bar.v1.0.0"}}},
					}}}
				isc := writeOperatorConfig(workDir, op)
				mirrorOperatorCatalogs(specCtx, workDir, isc, viaDisk, expected, catalogWorkflowOptions{
					BeforeImport: func() { Expect(os.RemoveAll(dir)).To(Succeed()) },
				})
				expectOperatorCatalogs(workDir, *testRegistry, nil, expected...)
			})

			It("filters an alternate-layout catalog with flat declarations at a non-default config root", SpecTimeout(4*time.Minute), func(specCtx SpecContext) {
				image := repackCatalog(publishedCatalogImage(), "/catalog/data")
				digest, err := image.Digest()
				Expect(err).NotTo(HaveOccurred())
				dir := writeLocalCatalog(workDir, "alternate-catalog", image)
				op := operatorFilter("isc-package-range.yaml")
				op.Catalog = "oci://" + dir
				expected := []catalogExpectation{{Destination: "alternate-catalog:latest",
					Source: "quay.io/" + testCatalogRepo + "@" + digest.String(),
					Packages: map[string]catalogPackageExpectation{
						"baz": {"stable", map[string][]string{"stable": {"baz.v1.0.0", "baz.v1.0.1"}}},
					}}}
				isc := writeOperatorConfig(workDir, op)
				mirrorOperatorCatalogs(specCtx, workDir, isc, viaDisk, expected, catalogWorkflowOptions{
					BeforeImport: func() { Expect(os.RemoveAll(dir)).To(Succeed()) },
				})
				expectOperatorCatalogs(workDir, *testRegistry, nil, expected...)
			})
		})
	}
})

func publishedCatalogImage() v1.Image {
	ref, err := name.ParseReference(happyPathCatalog)
	Expect(err).NotTo(HaveOccurred())
	img, err := remote.Image(ref, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))
	Expect(err).NotTo(HaveOccurred())
	return img
}

func writeLocalCatalog(workDir, basename string, img v1.Image) string {
	dir := filepath.Join(workDir, "sources", basename)
	path, err := layout.Write(dir, empty.Index)
	Expect(err).NotTo(HaveOccurred())
	Expect(path.AppendImage(img)).To(Succeed())
	abs, err := filepath.Abs(dir)
	Expect(err).NotTo(HaveOccurred())
	return abs
}

// Repackage our catalog's declarations at a different config root.
func repackCatalog(img v1.Image, root string) v1.Image {
	configs := extractCatalogImageConfigs(img)
	dc, err := declcfg.LoadFS(ctx, os.DirFS(configs))
	Expect(err).NotTo(HaveOccurred())
	var data bytes.Buffer
	Expect(declcfg.WriteYAML(*dc, &data)).To(Succeed())
	// Preserve readable permissions so these inputs also work in non-root CI jobs.
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	// A separate config root leaves the original files untouched and avoids whiteouts.
	var directory string
	for _, component := range strings.Split(strings.Trim(root, "/"), "/") {
		directory += component + "/"
		Expect(tw.WriteHeader(&tar.Header{Name: directory, Mode: 0o755, Typeflag: tar.TypeDir})).To(Succeed())
	}
	Expect(tw.WriteHeader(&tar.Header{
		Name: directory + "catalog.yaml", Mode: 0o644, Typeflag: tar.TypeReg, Size: int64(data.Len()),
	})).To(Succeed())
	_, err = tw.Write(data.Bytes())
	Expect(err).NotTo(HaveOccurred())
	Expect(tw.Close()).To(Succeed())
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(archive.Bytes())), nil
	})
	Expect(err).NotTo(HaveOccurred())
	variant, err := mutate.AppendLayers(img, layer)
	Expect(err).NotTo(HaveOccurred())
	config, err := variant.ConfigFile()
	Expect(err).NotTo(HaveOccurred())
	config.Config.Labels = maps.Clone(config.Config.Labels)
	config.Config.Labels["operators.operatorframework.io.index.configs.v1"] = root
	config.Config.Cmd = []string{"serve", root}
	variant, err = mutate.Config(variant, config.Config)
	Expect(err).NotTo(HaveOccurred())
	return variant
}
