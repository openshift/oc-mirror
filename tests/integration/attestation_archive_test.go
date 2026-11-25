package integration_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	gcrregistry "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

const attestationISC = `apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  additionalImages:
    - name: %s
`

var _ = Describe("attestation manifests", Label("attestation"), func() {
	const repoTag = "attestation/image:latest"

	DescribeTable("M2D + D2M should omit attestations only when removal is enabled",
		func(attestationAtSource, removeOciArtifacts bool) {
			scenario := fmt.Sprintf("attestation available=%t remove-oci-artifacts=%t", attestationAtSource, removeOciArtifacts)
			workDir := GinkgoT().TempDir()
			destinationConfig := registryConfig
			if removeOciArtifacts {
				destinationConfig = filepath.Join(filepath.Dir(registryConfig), "registry-config-sparse.yaml")
			}
			By(scenario + ": starting the destination registry")
			destinationRegistry, err := registry.Start(ctx, destinationConfig, 0)
			Expect(err).NotTo(HaveOccurred(), "%s destination startup config=%s", scenario, destinationConfig)
			DeferCleanup(destinationRegistry.Stop)
			Expect(destinationRegistry.WaitReady(ctx, 30*time.Second)).To(Succeed(), "%s destination readiness", scenario)
			var negativeDestinationRegistry *registry.Registry
			if removeOciArtifacts {
				By(scenario + ": starting a separate destination registry for the no-opt-in restore")
				negativeDestinationRegistry, err = registry.Start(ctx, destinationConfig, 0)
				Expect(err).NotTo(HaveOccurred(), "%s negative-control destination startup", scenario)
				DeferCleanup(negativeDestinationRegistry.Stop)
				Expect(negativeDestinationRegistry.WaitReady(ctx, 30*time.Second)).To(Succeed(), "%s negative-control destination readiness", scenario)
			}
			var listenConfig net.ListenConfig
			cacheListener, err := listenConfig.Listen(ctx, "tcp", ":0")
			Expect(err).NotTo(HaveOccurred(), "%s reserve cache port after all registry fixtures are bound", scenario)
			DeferCleanup(func() {
				if cacheListener != nil {
					Expect(cacheListener.Close()).To(Succeed(), "%s release cache port reservation", scenario)
				}
			})
			cachePort := strconv.Itoa(cacheListener.Addr().(*net.TCPAddr).Port)
			cacheArgs := []string{"--port", cachePort}
			if removeOciArtifacts {
				cacheArgs = append(cacheArgs, "--remove-oci-artifacts")
			}
			GinkgoWriter.Printf("%s cache port=%s destination=%s config=%s\n", scenario, cachePort, destinationRegistry.Endpoint(), destinationConfig)
			By(scenario + ": pushing an index with two runnable images and an annotated attestation to the source registry")
			runnable := []v1.Image{singleLayerTestImage("amd64", scenario), singleLayerTestImage("arm64", scenario)}
			attestation := singleLayerTestImage("unknown", scenario)
			index := mutate.AppendManifests(empty.Index,
				mutate.IndexAddendum{Add: runnable[0], Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}}},
				mutate.IndexAddendum{Add: runnable[1], Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "arm64"}}},
				mutate.IndexAddendum{Add: attestation, Descriptor: v1.Descriptor{
					Platform:    &v1.Platform{OS: "unknown", Architecture: "unknown"},
					Annotations: map[string]string{"vnd.docker.reference.type": "attestation-manifest"},
				}},
			)
			source := testRegistry.Endpoint() + "/" + repoTag
			sourceRef, err := name.NewTag(source, name.Insecure)
			Expect(err).NotTo(HaveOccurred(), "%s parse source tag", scenario)
			Expect(remote.WriteIndex(sourceRef, index, remote.WithContext(ctx))).To(Succeed(), "%s publish source index and complete content", scenario)
			indexDigest, err := index.Digest()
			Expect(err).NotTo(HaveOccurred(), "%s compute original index digest", scenario)
			// Pin expectations before taking the source offline.
			expected := map[string]string{repoTag: sourceRef.Context().Digest(indexDigest.String()).String()}
			attestationDigest, err := attestation.Digest()
			Expect(err).NotTo(HaveOccurred(), "%s compute attestation digest", scenario)
			if !attestationAtSource {
				By(scenario + ": deleting the attestation manifest from the source registry")
				Expect(remote.Delete(sourceRef.Context().Digest(attestationDigest.String()), remote.WithContext(ctx))).To(Succeed(), "%s delete only source attestation", scenario)
			}

			isc := filepath.Join(workDir, "isc-attestation.yaml")
			Expect(os.WriteFile(isc, fmt.Appendf(nil, attestationISC, source), 0o600)).To(Succeed(), "%s write image set configuration", scenario)

			By(scenario + ": running mirrorToDisk with explicit cache port " + cachePort)
			Expect(cacheListener.Close()).To(Succeed(), "%s release reserved port immediately before CLI bind", scenario)
			cacheListener = nil
			result, err := runner.MirrorToDisk(ctx, isc, workDir, append(cacheArgs, "--src-tls-verify=false", "--remove-signatures=true")...)
			if !attestationAtSource && !removeOciArtifacts {
				expectOcMirrorCommandFailure(result, err)
				Expect(result.Stdout + result.Stderr).To(And(
					ContainSubstring(attestationDigest.String()), ContainSubstring("manifest unknown")))
				return
			}
			expectOcMirrorCommandSuccess(result, err)

			By(scenario + ": verifying archive contents follow the removal flag")
			expectArchiveContainsExactly(workDir, expected)
			for _, image := range runnable {
				expectTarContainsBlobForDigests(workDir, testImageBlobDigests(image, scenario+": archive runnable closure")...)
			}
			if !removeOciArtifacts {
				expectTarContainsBlobForDigests(workDir, testImageBlobDigests(attestation, scenario+": archive attestation closure")...)
			} else {
				entries := listTarEntries(findSingleMirrorTar(workDir, "mirrorToDisk"))
				for _, digest := range testImageBlobDigests(attestation, scenario+": archive attestation closure") {
					expectTarDoesNotContainPath(entries, strings.TrimPrefix(digest, "sha256:"))
				}
			}

			By(scenario + ": stopping the source registry")
			// Shutdown may time out after closing the listener.
			if err := testRegistry.Stop(); err != nil {
				GinkgoWriter.Printf("Failed to stop source registry: %v\n", err)
			}
			_, err = remote.Head(sourceRef, remote.WithContext(ctx))
			Expect(err).To(HaveOccurred(), "%s source registry must be offline before diskToMirror", scenario)

			if negativeDestinationRegistry != nil {
				By(scenario + ": verifying diskToMirror also requires explicit opt-in for a sparse archive")
				result, err = runner.DiskToMirror(ctx, isc, workDir, negativeDestinationRegistry.Endpoint(),
					"--port", cachePort, "--cache-dir", GinkgoT().TempDir(), "--dest-tls-verify=false", "--remove-signatures=true")
				expectOcMirrorCommandFailure(result, err)
				Expect(result.Stdout + result.Stderr).To(And(
					ContainSubstring(attestationDigest.String()), ContainSubstring("manifest unknown")))
			}
			By(scenario + ": running diskToMirror to the selected destination with fresh cache and explicit port " + cachePort)
			result, err = runner.DiskToMirror(ctx, isc, workDir, destinationRegistry.Endpoint(),
				append(cacheArgs, "--cache-dir", GinkgoT().TempDir(), "--dest-tls-verify=false", "--remove-signatures=true")...)
			expectOcMirrorCommandSuccess(result, err)

			By(scenario + ": verifying the original index and all retained content")
			expectRegistryContainsExactly(*destinationRegistry, expected)
			targetRef, err := name.NewTag(destinationRegistry.Endpoint()+"/"+repoTag, name.Insecure)
			Expect(err).NotTo(HaveOccurred(), "%s parse restored target tag", scenario)
			mirrored, err := remote.Get(targetRef, remote.WithContext(ctx))
			Expect(err).NotTo(HaveOccurred(), "%s fetch restored parent", scenario)
			parent, err := index.RawManifest()
			Expect(err).NotTo(HaveOccurred(), "%s read original parent bytes", scenario)
			Expect(mirrored.Digest).To(Equal(indexDigest), "%s restored index digest", scenario)
			Expect(mirrored.Manifest).To(Equal(parent), "%s restored parent bytes/digest must be unchanged", scenario)
			for _, image := range runnable {
				digest, err := image.Digest()
				Expect(err).NotTo(HaveOccurred(), "%s compute restored runnable digest", scenario)
				By(scenario + ": checking restored runnable manifest/config/layers " + digest.String())
				expectImageReadableByDigest(ctx, *destinationRegistry, targetRef.RepositoryStr(), digest.String())
			}
			if !removeOciArtifacts {
				By(scenario + ": checking restored attestation manifest/config/layers " + attestationDigest.String())
				expectImageReadableByDigest(ctx, *destinationRegistry, targetRef.RepositoryStr(), attestationDigest.String())
			} else {
				exists, err := destinationRegistry.ManifestExistsByDigest(ctx, targetRef.RepositoryStr(), attestationDigest.String())
				Expect(err).NotTo(HaveOccurred(), "%s check missing attestation remains absent", scenario)
				Expect(exists).To(BeFalse(), "%s omitted attestation %s must remain absent", scenario, attestationDigest)
			}
		},
		Entry("attestation available at the source without opt-in", true, false),
		Entry("attestation available at the source with opt-in", true, true),
		Entry("attestation missing from the source without opt-in", false, false),
		Entry("attestation missing from the source with opt-in", false, true),
	)

	It("M2M should exclude available attestations before reads and preserve platform selection", func() {
		runnable := singleLayerTestImage("amd64", "artifact removal")
		otherPlatform := singleLayerTestImage("arm64", "artifact removal")
		attestation := singleLayerTestImage("unknown", "artifact removal")
		attestationDigest, err := attestation.Digest()
		Expect(err).NotTo(HaveOccurred())
		index := mutate.AppendManifests(empty.Index,
			mutate.IndexAddendum{Add: runnable, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}}},
			mutate.IndexAddendum{Add: otherPlatform, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "arm64"}}},
			mutate.IndexAddendum{Add: attestation, Descriptor: v1.Descriptor{
				Platform:    &v1.Platform{OS: "linux", Architecture: "amd64"},
				Annotations: map[string]string{"vnd.docker.reference.type": "attestation-manifest"},
			}},
		)
		var attestationReads atomic.Int64
		handler := gcrregistry.New()
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/manifests/"+attestationDigest.String()) {
				attestationReads.Add(1)
			}
			handler.ServeHTTP(w, r)
		}))
		DeferCleanup(source.Close)
		sourceRef, err := name.NewTag(strings.TrimPrefix(source.URL, "http://")+"/"+repoTag, name.Insecure)
		Expect(err).NotTo(HaveOccurred())
		Expect(remote.WriteIndex(sourceRef, index, remote.WithContext(ctx))).To(Succeed())
		attestationReads.Store(0)

		destination, err := registry.Start(ctx, filepath.Join(filepath.Dir(registryConfig), "registry-config-sparse.yaml"), 0)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(destination.Stop)
		Expect(destination.WaitReady(ctx, 30*time.Second)).To(Succeed())
		workDir := GinkgoT().TempDir()
		isc := filepath.Join(workDir, "isc.yaml")
		const platformISC = attestationISC + "      platforms:\n        - os: linux\n          architecture: amd64\n"
		Expect(os.WriteFile(isc, fmt.Appendf(nil, platformISC, sourceRef.String()), 0o600)).To(Succeed())
		result, err := runner.MirrorToMirror(ctx, isc, workDir, destination.Endpoint(),
			"--remove-oci-artifacts", "--retry-times=0", "--src-tls-verify=false", "--dest-tls-verify=false", "--remove-signatures=true")
		expectOcMirrorCommandSuccess(result, err)
		Expect(attestationReads.Load()).To(BeZero(), "artifact removal must not request the attestation manifest")
		_, err = remote.Get(sourceRef.Context().Digest(attestationDigest.String()), remote.WithContext(ctx))
		Expect(err).NotTo(HaveOccurred(), "the attestation must remain readable at the source")

		targetRef, err := name.NewTag(destination.Endpoint()+"/"+repoTag, name.Insecure)
		Expect(err).NotTo(HaveOccurred())
		mirrored, err := remote.Get(targetRef, remote.WithContext(ctx))
		Expect(err).NotTo(HaveOccurred())
		parent, err := index.RawManifest()
		Expect(err).NotTo(HaveOccurred())
		Expect(mirrored.Manifest).To(Equal(parent))
		runnableDigest, err := runnable.Digest()
		Expect(err).NotTo(HaveOccurred())
		expectImageReadableByDigest(ctx, *destination, targetRef.RepositoryStr(), runnableDigest.String())
		exists, err := destination.ManifestExistsByDigest(ctx, targetRef.RepositoryStr(), attestationDigest.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeFalse(), "artifact removal must not publish the available attestation")
		otherDigest, err := otherPlatform.Digest()
		Expect(err).NotTo(HaveOccurred())
		exists, err = destination.ManifestExistsByDigest(ctx, targetRef.RepositoryStr(), otherDigest.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeFalse(), "artifact removal must not broaden the requested platforms")
	})
})

//nolint:ireturn // go-containerregistry represents images as the v1.Image interface
func singleLayerTestImage(architecture, scenario string) v1.Image {
	GinkgoHelper()
	layer, err := random.Layer(256, types.OCILayer)
	Expect(err).NotTo(HaveOccurred(), "%s fixture architecture=%s: create random layer", scenario, architecture)
	image, err := mutate.AppendLayers(empty.Image, layer)
	Expect(err).NotTo(HaveOccurred(), "%s fixture architecture=%s: append image layer", scenario, architecture)
	image = mutate.ConfigMediaType(mutate.MediaType(image, types.OCIManifestSchema1), types.OCIConfigJSON)
	config, err := image.ConfigFile()
	Expect(err).NotTo(HaveOccurred(), "%s fixture architecture=%s: read config", scenario, architecture)
	config.OS, config.Architecture = "linux", architecture
	image, err = mutate.ConfigFile(image, config)
	Expect(err).NotTo(HaveOccurred(), "%s fixture architecture=%s: set architecture config", scenario, architecture)
	return image
}

func testImageBlobDigests(image v1.Image, phase string) []string {
	GinkgoHelper()
	manifest, err := image.Manifest()
	Expect(err).NotTo(HaveOccurred(), "%s read image manifest", phase)
	digest, err := image.Digest()
	Expect(err).NotTo(HaveOccurred(), "%s compute image digest", phase)
	blobs := make([]string, 0, 2+len(manifest.Layers))
	blobs = append(blobs, digest.String(), manifest.Config.Digest.String())
	for _, layer := range manifest.Layers {
		blobs = append(blobs, layer.Digest.String())
	}
	return blobs
}
