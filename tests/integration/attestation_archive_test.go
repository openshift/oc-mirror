package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
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

	DescribeTable("M2D + D2M should keep available attestations and fall back without missing ones",
		func(attestationAtSource bool) {
			scenario := fmt.Sprintf("attestation available=%t", attestationAtSource)
			workDir := GinkgoT().TempDir()
			destinationConfig := registryConfig
			if !attestationAtSource {
				destinationConfig = filepath.Join(filepath.Dir(registryConfig), "registry-config-sparse.yaml")
			}
			By(scenario + ": starting the destination registry")
			destinationRegistry, err := registry.Start(ctx, destinationConfig, 0)
			Expect(err).NotTo(HaveOccurred(), "%s destination startup config=%s", scenario, destinationConfig)
			DeferCleanup(destinationRegistry.Stop)
			Expect(destinationRegistry.WaitReady(ctx, 30*time.Second)).To(Succeed(), "%s destination readiness", scenario)
			var listenConfig net.ListenConfig
			cacheListener, err := listenConfig.Listen(ctx, "tcp", ":0")
			Expect(err).NotTo(HaveOccurred(), "%s reserve cache port after both fixtures are bound", scenario)
			DeferCleanup(func() {
				if cacheListener != nil {
					Expect(cacheListener.Close()).To(Succeed(), "%s release cache port reservation", scenario)
				}
			})
			cachePort := strconv.Itoa(cacheListener.Addr().(*net.TCPAddr).Port)
			Expect(cachePort).NotTo(Equal("0"), "%s CLI cache port must be explicit and nonzero", scenario)
			for _, endpoint := range []string{testRegistry.Endpoint(), destinationRegistry.Endpoint()} {
				_, fixturePort, splitErr := net.SplitHostPort(endpoint)
				Expect(splitErr).NotTo(HaveOccurred(), "%s fixture endpoint %s", scenario, endpoint)
				Expect(cachePort).NotTo(Equal(fixturePort), "%s cache must not collide with bound fixture %s", scenario, endpoint)
			}
			cacheArgs := []string{"--port", cachePort}
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
			// Expected references are pinned to the pushed index digest, so they resolve
			// without contacting the source after it is stopped.
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
			expectAttestationCommandSuccess(result, err, scenario+": mirrorToDisk")
			expectAttestationFallback(result, !attestationAtSource, scenario+": mirrorToDisk")

			By(scenario + ": verifying the archive holds the original index, the runnable images and only an available attestation")
			expectArchiveContainsExactly(workDir, expected)
			for _, image := range runnable {
				expectTarContainsBlobForDigests(workDir, testImageBlobDigests(image, scenario+": archive runnable closure")...)
			}
			if attestationAtSource {
				expectTarContainsBlobForDigests(workDir, testImageBlobDigests(attestation, scenario+": archive attestation closure")...)
			} else {
				entries := listTarEntries(findSingleMirrorTar(workDir, "mirrorToDisk"))
				for _, digest := range testImageBlobDigests(attestation, scenario+": archive attestation closure") {
					expectTarDoesNotContainPath(entries, strings.TrimPrefix(digest, "sha256:"))
				}
			}

			By(scenario + ": stopping the source registry")
			// Like the suite cleanup, tolerate a slow graceful shutdown: the listener
			// is closed either way, which the reachability check below confirms.
			if err := testRegistry.Stop(); err != nil {
				GinkgoWriter.Printf("Failed to stop source registry: %v\n", err)
			}
			_, err = remote.Head(sourceRef, remote.WithContext(ctx))
			Expect(err).To(HaveOccurred(), "%s source registry must be offline before diskToMirror", scenario)

			if attestationAtSource {
				By(scenario + ": verifying the standard destination rejects a parent before its children")
				parent, manifestErr := index.RawManifest()
				Expect(manifestErr).NotTo(HaveOccurred(), "%s raw parent for destination validation control", scenario)
				expectAttestationParentRejected(ctx, destinationRegistry.Endpoint(), repoTag, parent, scenario)
			}
			By(scenario + ": running diskToMirror to the selected destination with fresh cache and explicit port " + cachePort)
			result, err = runner.DiskToMirror(ctx, isc, workDir, destinationRegistry.Endpoint(),
				append(cacheArgs, "--cache-dir", GinkgoT().TempDir(), "--dest-tls-verify=false", "--remove-signatures=true")...)
			expectAttestationCommandSuccess(result, err, scenario+": diskToMirror")
			expectAttestationFallback(result, !attestationAtSource, scenario+": diskToMirror")

			By(scenario + ": verifying the original index bytes and digest, the runnable images and the attestation only if it was available")
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
			if attestationAtSource {
				By(scenario + ": checking restored attestation manifest/config/layers " + attestationDigest.String())
				expectImageReadableByDigest(ctx, *destinationRegistry, targetRef.RepositoryStr(), attestationDigest.String())
			} else {
				exists, err := destinationRegistry.ManifestExistsByDigest(ctx, targetRef.RepositoryStr(), attestationDigest.String())
				Expect(err).NotTo(HaveOccurred(), "%s check missing attestation remains absent", scenario)
				Expect(exists).To(BeFalse(), "%s missing attestation %s must remain absent", scenario, attestationDigest)
			}
		},
		Entry("attestation available at the source", true),
		Entry("attestation missing from the source", false),
	)
})

// expectAttestationFallback checks whether oc-mirror retried a failed copy without
// attestation manifests: it must only do so when an attestation was unavailable.
func expectAttestationFallback(result *ocmirror.Result, expected bool, phase string) {
	GinkgoHelper()
	matcher := ContainSubstring("retrying once without")
	if !expected {
		matcher = Not(matcher)
	}
	Expect(result.Stdout+result.Stderr).To(matcher, "%s fallback warning expected=%t", phase, expected)
}

// singleLayerTestImage returns a small single-layer OCI image for the given architecture.
//
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

// testImageBlobDigests returns the manifest, config and layer digests of image.
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

func expectAttestationCommandSuccess(result *ocmirror.Result, err error, phase string) {
	GinkgoHelper()
	Expect(err).NotTo(HaveOccurred(), "%s execute CLI", phase)
	Expect(result).NotTo(BeNil(), "%s CLI result", phase)
	Expect(result.ExitCode).To(BeZero(), "%s exit code; stdout:\n%s\nstderr:\n%s", phase, result.Stdout, result.Stderr)
}

func expectAttestationParentRejected(ctx context.Context, endpoint, repoTag string, parent []byte, scenario string) {
	GinkgoHelper()
	repository, tag, found := strings.Cut(repoTag, ":")
	Expect(found).To(BeTrue(), "%s parent validation fixture requires a tag", scenario)
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://"+endpoint+"/v2/"+repository+"/manifests/"+tag, bytes.NewReader(parent))
	Expect(err).NotTo(HaveOccurred(), "%s create raw parent PUT without uploading children", scenario)
	request.Header.Set("Content-Type", string(types.OCIImageIndex))
	response, err := http.DefaultClient.Do(request)
	Expect(err).NotTo(HaveOccurred(), "%s raw parent PUT must reach standard destination", scenario)
	body, readErr := io.ReadAll(response.Body)
	Expect(response.Body.Close()).To(Succeed(), "%s close validation response", scenario)
	Expect(readErr).NotTo(HaveOccurred(), "%s read validation response", scenario)
	Expect(response.StatusCode).To(Equal(http.StatusBadRequest), "%s missing-child validation response: %s", scenario, body)
	var failure struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	Expect(json.Unmarshal(body, &failure)).To(Succeed(), "%s decode registry validation error: %s", scenario, body)
	var index v1.IndexManifest
	Expect(json.Unmarshal(parent, &index)).To(Succeed(), "%s decode expected child descriptors", scenario)
	Expect(failure.Errors).To(HaveLen(len(index.Manifests)), "%s every missing child must be validated: %s", scenario, body)
	for _, entry := range failure.Errors {
		Expect(entry.Code).To(Equal("MANIFEST_BLOB_UNKNOWN"), "%s reject specifically missing child manifests, not auth/network errors: %s", scenario, body)
	}
	GinkgoWriter.Printf("%s standard destination rejected parent with %d MANIFEST_BLOB_UNKNOWN errors\n", scenario, len(failure.Errors))
	for _, child := range index.Manifests {
		Expect(body).To(ContainSubstring(child.Digest.String()), "%s validation must identify missing child %s", scenario, child.Digest)
	}
}
