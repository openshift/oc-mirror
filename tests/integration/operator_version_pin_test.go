package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	gcrregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
)

var _ = Describe("operator exact version selection", func() {
	var workDir string
	BeforeEach(func() { workDir = setupWorkDir() })
	AfterEach(func() { cleanupWorkDir(workDir) })

	for _, viaDisk := range []bool{false, true} {
		workflow := "mirrorToMirror"
		if viaDisk {
			workflow = "mirrorToDisk + diskToMirror"
		}
		Context(workflow, func() {
			for _, packageRange := range []bool{false, true} {
				scope := "channel-level"
				if packageRange {
					scope = "package-level"
				}
				DescribeTable("honors "+scope+" equal min/max bounds", func(specCtx SpecContext, scenario, metadataVersion string) {
					By("building a local catalog with matching and excluded versions")
					source := httptest.NewServer(gcrregistry.New(gcrregistry.Logger(log.New(io.Discard, "", 0))))
					DeferCleanup(source.Close)
					host := strings.TrimPrefix(source.URL, "http://")
					dc, selected, version := versionPinFixture(host, scenario, metadataVersion)
					imageDigests := map[string]string{}
					for _, bundle := range dc.Bundles {
						ref, err := name.ParseReference(bundle.Image)
						Expect(err).NotTo(HaveOccurred())
						img, err := remote.Image(ref, remote.WithContext(specCtx))
						Expect(err).NotTo(HaveOccurred())
						digest, err := img.Digest()
						Expect(err).NotTo(HaveOccurred())
						imageDigests[bundle.Name] = digest.String()
						remoteDigest(bundle.Image)
					}
					_, err := declcfg.ConvertToModel(dc)
					Expect(err).NotTo(HaveOccurred(), "source fixture must be a valid OLM catalog")
					var data bytes.Buffer
					Expect(declcfg.WriteYAML(dc, &data)).To(Succeed())
					catalog := pushOperatorPlatformImage(host, "catalog", []string{"amd64"},
						map[string]string{"configs/catalog.yaml": data.String()},
						map[string]string{"operators.operatorframework.io.index.configs.v1": "/configs"})
					pkg := OperatorPackage{Name: dc.Packages[0].Name}
					for _, channel := range dc.Channels {
						if channel.Name != "other" {
							pkg.Channels = append(pkg.Channels, OperatorChannel{Name: channel.Name})
						}
					}
					if packageRange {
						pkg.MinVersion, pkg.MaxVersion = version, version
					} else {
						for i := range pkg.Channels {
							pkg.Channels[i].MinVersion, pkg.Channels[i].MaxVersion = version, version
						}
					}
					isc := writeOperatorConfig(workDir, OperatorConfig{
						Catalog: catalog, TargetCatalog: "selected-catalog", TargetTag: "pin", Packages: []OperatorPackage{pkg},
					})
					flags := []string{"--src-tls-verify=false", "--dest-tls-verify=false", "--remove-signatures=true"}
					if viaDisk {
						By("archiving the selection, then disconnecting the source")
						result, err := runner.MirrorToDisk(specCtx, isc, workDir, flags...)
						expectOcMirrorCommandSuccess(result, err)
						Expect(findTarArchives(workDir)).NotTo(BeEmpty())
						Expect(os.RemoveAll(filepath.Join(workDir, dirWorkingDir))).To(Succeed())
						Expect(os.RemoveAll(cacheDir)).To(Succeed())
						source.Close()
						result, err = runner.DiskToMirror(specCtx, isc, workDir, testRegistry.Endpoint(), flags...)
						expectOcMirrorCommandSuccess(result, err)
					} else {
						By("mirroring the selection directly")
						result, err := runner.MirrorToMirror(specCtx, isc, workDir, testRegistry.Endpoint(), flags...)
						expectOcMirrorCommandSuccess(result, err)
					}

					By("checking exact channel entries, bundle metadata and OLM validity in the rebuilt catalog")
					ref, err := name.NewTag(testRegistry.Endpoint()+"/selected-catalog:pin", name.Insecure)
					Expect(err).NotTo(HaveOccurred())
					img, err := remote.Image(ref, remote.WithContext(specCtx))
					Expect(err).NotTo(HaveOccurred())
					actual, err := declcfg.LoadFS(specCtx, os.DirFS(extractCatalogImageConfigs(img)))
					Expect(err).NotTo(HaveOccurred())
					Expect(actual.Packages).To(Equal(dc.Packages))
					var channels []declcfg.Channel
					for _, channel := range dc.Channels {
						if channel.Name == "other" {
							continue
						}
						channel.Entries = slices.DeleteFunc(slices.Clone(channel.Entries), func(e declcfg.ChannelEntry) bool {
							return !slices.Contains(selected, e.Name)
						})
						channels = append(channels, channel)
					}
					Expect(actual.Channels).To(ConsistOf(channels))
					var bundles []declcfg.Bundle
					images := map[string]string{"selected-catalog:pin": ""}
					repositories := []string{"selected-catalog"}
					for _, bundle := range dc.Bundles {
						if slices.Contains(selected, bundle.Name) {
							bundles = append(bundles, bundle)
							repositories = append(repositories, "operand-test/"+bundle.Name)
							images[strings.TrimPrefix(bundle.Image, host+"/")] = bundle.Image
							// The source is deliberately offline during diskToMirror.
							expectImageReadableByDigest(specCtx, *testRegistry, "operand-test/"+bundle.Name, imageDigests[bundle.Name])
						}
					}
					Expect(actual.Bundles).To(ConsistOf(bundles))
					// Metadata belongs only to its original matching bundle. In particular,
					// the excluded 1.13.0 metadata must not be recovered or copied elsewhere.
					expectedMetadata := map[string][]property.Property{}
					actualMetadata := map[string][]property.Property{}
					for _, bundle := range bundles {
						for _, prop := range bundle.Properties {
							if prop.Type == property.TypeCSVMetadata {
								expectedMetadata[bundle.Name] = append(expectedMetadata[bundle.Name], prop)
							}
						}
					}
					for _, bundle := range actual.Bundles {
						for _, prop := range bundle.Properties {
							if prop.Type == property.TypeCSVMetadata {
								actualMetadata[bundle.Name] = append(actualMetadata[bundle.Name], prop)
							}
						}
					}
					Expect(actualMetadata).To(Equal(expectedMetadata))
					_, err = declcfg.ConvertToModel(*actual)
					Expect(err).NotTo(HaveOccurred())
					By("checking that only selected images were copied and catalog resources were generated")
					expectRegistryContainsExactly(*testRegistry, images)
					actualRepositories, err := testRegistry.ListRepositories(specCtx)
					Expect(err).NotTo(HaveOccurred())
					Expect(actualRepositories).To(ConsistOf(repositories))
					expectCatalogResources(workDir, []string{ref.Name()})
				},
					Entry("retains every same-version rebuild with metadata on 1.12.1", SpecTimeout(3*time.Minute), "rebuilds", "1.12.1"),
					Entry("retains every same-version rebuild with metadata on 1.12.1+0.1742484227.p", SpecTimeout(3*time.Minute), "rebuilds", "1.12.1+0.1742484227.p"),
					Entry("retains every same-version rebuild with metadata on 1.12.1+0.1743657317.p", SpecTimeout(3*time.Minute), "rebuilds", "1.12.1+0.1743657317.p"),
					Entry("retains every same-version rebuild with metadata on 1.12.1+0.1745393748.p", SpecTimeout(3*time.Minute), "rebuilds", "1.12.1+0.1745393748.p"),
					Entry("excludes higher Keycloak versions that skip the pinned prerelease", SpecTimeout(3*time.Minute), "keycloak", ""),
					Entry("excludes Redis 7.8.6-1.1 when pinning 7.4.6-6.1", SpecTimeout(3*time.Minute), "redis", ""),
				)
			}
		})
	}
})

// Synthetic graphs reproduce OCPBUGS-138615 and OCPBUGS-61497 without authenticated product catalogs.
// Distinct image contents ensure a missing rebuild cannot hide behind a shared digest.
func versionPinFixture(host, scenario, metadataVersion string) (declcfg.DeclarativeConfig, []string, string) {
	GinkgoHelper()
	rebuilds := scenario == "rebuilds"
	pkg, pin := "web-terminal", "1.12.1"
	versions := []string{"1.12.0", pin, pin + "+0.1742484227.p", pin + "+0.1743657317.p", pin + "+0.1745393748.p", "1.13.0", pin + "+other-channel"}
	if !rebuilds {
		pkg, pin = "rhbk-operator", "26.0.8-opr.1"
		versions = []string{"26.0.6-opr.1", "26.0.7-opr.1", pin, "26.0.9-opr.1", "26.0.15-opr.1", "26.0.17-opr.1"}
	}
	if scenario == "redis" {
		pkg, pin = "redis-enterprise-operator-cert", "7.4.6-6.1"
		versions = []string{"7.4.6-5.1", "7.4.6-6.0", pin, "7.4.6-7.1", "7.8.6-1.1", "7.8.6-2.1"}
	}
	dc := declcfg.DeclarativeConfig{
		Packages: []declcfg.Package{{Schema: declcfg.SchemaPackage, Name: pkg, DefaultChannel: "stable"}},
		Channels: []declcfg.Channel{{Schema: declcfg.SchemaChannel, Name: "stable", Package: pkg}},
	}
	var selected []string
	for i, version := range versions {
		bundleName := pkg + ".v" + strings.ReplaceAll(version, "+", "-")
		image := pushOperatorPlatformImage(host, bundleName, []string{"amd64"}, map[string]string{"identity": bundleName}, nil)
		value, err := json.Marshal(property.Package{PackageName: pkg, Version: version})
		Expect(err).NotTo(HaveOccurred())
		dc.Bundles = append(dc.Bundles, declcfg.Bundle{
			Schema: declcfg.SchemaBundle, Name: bundleName, Package: pkg, Image: image,
			Properties:    []property.Property{{Type: property.TypePackage, Value: value}},
			RelatedImages: []declcfg.RelatedImage{{Name: "bundle", Image: image}},
		})
		if rebuilds && (version == metadataVersion || version == "1.13.0") {
			// Typed metadata is valid without CSV manifests in these minimal images.
			// Only one matching rebuild carries it; the newer head has distinct metadata.
			dc.Bundles[i].Properties = append(dc.Bundles[i].Properties, property.MustBuild(&property.CSVMetadata{
				DisplayName: bundleName,
				Description: "CSV metadata belonging to " + bundleName,
				Labels:      map[string]string{"fixture-bundle": bundleName},
			}))
		}
		entry := declcfg.ChannelEntry{Name: bundleName}
		if i > 0 {
			entry.Replaces = dc.Bundles[i-1].Name
		}
		if rebuilds && i == len(versions)-1 {
			dc.Channels = append(dc.Channels, declcfg.Channel{Schema: declcfg.SchemaChannel, Name: "other", Package: pkg, Entries: []declcfg.ChannelEntry{{Name: bundleName}}})
			continue
		}
		dc.Channels[0].Entries = append(dc.Channels[0].Entries, entry)
		if version == pin || (rebuilds && strings.HasPrefix(version, pin+"+")) {
			selected = append(selected, bundleName)
		}
	}
	if !rebuilds {
		// A newer head skips the pin and its neighbors: exact bounds must not
		// retain the newer head just to preserve the original upgrade graph.
		dc.Channels[0].Entries[4].Replaces = ""
		for _, bundle := range dc.Bundles[:4] {
			dc.Channels[0].Entries[4].Skips = append(dc.Channels[0].Entries[4].Skips, bundle.Name)
		}
	}
	if scenario == "redis" {
		dc.Packages[0].DefaultChannel = "production"
		dc.Channels[0].Name = "production"
		dc.Channels = append(dc.Channels, declcfg.Channel{
			Schema: declcfg.SchemaChannel, Name: "7.4.6", Package: pkg,
			Entries: slices.Clone(dc.Channels[0].Entries[:4]),
		})
	}
	return dc, selected, pin
}
