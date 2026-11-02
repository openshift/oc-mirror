package integration_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

var _ = Describe("sparse manifest platform filtering", Label("sparse-manifest"), func() {
	var workDir string

	BeforeEach(func() { workDir = setupWorkDir() })
	AfterEach(func() { cleanupWorkDir(workDir) })

	Describe("config validation", func() {
		It("should fail when both platform.platforms and platform.architectures are set", func() {
			By("running mirrorToMirror with an ISC that uses both platforms and architectures")
			result, err := runner.MirrorToMirror(ctx,
				filepath.Join(iscDir, "sparse_manifest", "isc-platforms-and-archs.yaml"),
				workDir, testRegistry.Endpoint(),
				"--dest-tls-verify=false")
			By("verifying oc-mirror exits with an error containing the mutual-exclusivity message")
			expectOcMirrorExitCode(result, err, 1,
				"platform.platforms and platform.architectures cannot be used together")
		})
	})

	Describe("M2M", func() {
		Context("with platforms filter (M2M)", func() {
			var sparseRegistry *registry.Registry

			BeforeEach(func() {
				sparseRegistryConfigPath := filepath.Join(filepath.Dir(registryConfig), "registry-config-sparse.yaml")
				var err error
				sparseRegistry, err = registry.Start(ctx, sparseRegistryConfigPath, 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(sparseRegistry.WaitReady(ctx, 30*time.Second)).To(Succeed())
			})

			AfterEach(func() {
				Expect(sparseRegistry.Stop()).To(Succeed())
			})

			It("should mirror only declared arch blobs for each multi-arch image type", func() {
				By("mirroring with platforms filter on all multi-arch image types")
				result, err := runner.MirrorToMirror(ctx,
					filepath.Join(iscDir, "sparse_manifest", "isc-sparse-full-multi.yaml"),
					workDir, sparseRegistry.Endpoint(),
					"--dest-tls-verify=false")
				expectOcMirrorCommandSuccess(result, err)

				By("verifying release index has only amd64 and arm64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"openshift/release-images", "v0.0.1",
					[]string{"linux/amd64", "linux/arm64"})

				By("verifying release component has only amd64 and arm64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"openshift/release", "v0.0.1-alpine",
					[]string{"linux/amd64", "linux/arm64"})

				By("verifying operator catalog has only amd64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"oc-mirror/oc-mirror-dev", "test-catalog-multi",
					[]string{"linux/amd64"})

				By("verifying foo operator bundle has only amd64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"oc-mirror/oc-mirror-dev", "foo-bundle-v0.3.1-multi",
					[]string{"linux/amd64"})

				By("verifying bar operator bundle has only amd64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"oc-mirror/oc-mirror-dev", "bar-bundle-v1.0.0-multi",
					[]string{"linux/amd64"})

				By("verifying helm chart image has only amd64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"oc-mirror/test-chart", "1.0.0",
					[]string{"linux/amd64"})

				By("verifying additionalImage has only amd64 and arm64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"oc-mirror/multi-platform-image", "latest",
					[]string{"linux/amd64", "linux/arm64"})
			})
		})

		Context("without platforms filter (backward compat)", func() {
			It("should mirror all blobs without sparse filtering", func() {
				By("mirroring without platforms filter")
				result, err := runner.MirrorToMirror(ctx,
					filepath.Join(iscDir, "sparse_manifest", "isc-sparse-no-platforms.yaml"),
					workDir, testRegistry.Endpoint(),
					"--dest-tls-verify=false")
				expectOcMirrorCommandSuccess(result, err)

				By("verifying all platform blobs are present for multi-platform-image")
				expectAllPlatformBlobsPresent(ctx, testRegistry,
					"oc-mirror/multi-platform-image", "latest")

				By("verifying the release image was mirrored")
				expectRepositoriesExist(*testRegistry, []string{"openshift/release-images"})
			})
		})
	})

	Describe("M2D + D2M", func() {
		Context("with platforms filter (M2D+D2M)", func() {
			var sparseRegistry *registry.Registry

			BeforeEach(func() {
				sparseRegistryConfigPath := filepath.Join(filepath.Dir(registryConfig), "registry-config-sparse.yaml")
				var err error
				sparseRegistry, err = registry.Start(ctx, sparseRegistryConfigPath, 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(sparseRegistry.WaitReady(ctx, 30*time.Second)).To(Succeed())
			})

			AfterEach(func() {
				Expect(sparseRegistry.Stop()).To(Succeed())
			})

			It("should preserve blob-level platform filtering through disk archive for all multi-arch image types", func() {
				By("mirroring to disk with platforms filter on all multi-arch image types")
				result, err := runner.MirrorToDisk(ctx,
					filepath.Join(iscDir, "sparse_manifest", "isc-sparse-full-multi.yaml"),
					workDir)
				expectOcMirrorCommandSuccess(result, err)

				By("mirroring from disk to sparse registry")
				result, err = runner.DiskToMirror(ctx,
					filepath.Join(iscDir, "sparse_manifest", "isc-sparse-full-multi.yaml"),
					workDir, sparseRegistry.Endpoint(),
					"--dest-tls-verify=false")
				expectOcMirrorCommandSuccess(result, err)

				By("verifying release index has only amd64 and arm64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"openshift/release-images", "v0.0.1",
					[]string{"linux/amd64", "linux/arm64"})

				By("verifying release component has only amd64 and arm64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"openshift/release", "v0.0.1-alpine",
					[]string{"linux/amd64", "linux/arm64"})

				By("verifying operator catalog has only amd64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"oc-mirror/oc-mirror-dev", "test-catalog-multi",
					[]string{"linux/amd64"})

				By("verifying foo operator bundle has only amd64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"oc-mirror/oc-mirror-dev", "foo-bundle-v0.3.1-multi",
					[]string{"linux/amd64"})

				By("verifying bar operator bundle has only amd64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"oc-mirror/oc-mirror-dev", "bar-bundle-v1.0.0-multi",
					[]string{"linux/amd64"})

				By("verifying helm chart image has only amd64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"oc-mirror/test-chart", "1.0.0",
					[]string{"linux/amd64"})

				By("verifying additionalImage has only amd64 and arm64 blobs")
				expectOnlyPlatformBlobsPresent(ctx, sparseRegistry,
					"oc-mirror/multi-platform-image", "latest",
					[]string{"linux/amd64", "linux/arm64"})
			})
		})

		Context("without platforms filter (backward compat)", func() {
			It("should preserve all blobs through disk archive", func() {
				By("mirroring to disk without platforms filter")
				result, err := runner.MirrorToDisk(ctx,
					filepath.Join(iscDir, "sparse_manifest", "isc-sparse-no-platforms.yaml"),
					workDir)
				expectOcMirrorCommandSuccess(result, err)

				By("mirroring from disk to registry")
				result, err = runner.DiskToMirror(ctx,
					filepath.Join(iscDir, "sparse_manifest", "isc-sparse-no-platforms.yaml"),
					workDir, testRegistry.Endpoint(),
					"--dest-tls-verify=false")
				expectOcMirrorCommandSuccess(result, err)

				By("verifying all platform blobs are present for multi-platform-image")
				expectAllPlatformBlobsPresent(ctx, testRegistry,
					"oc-mirror/multi-platform-image", "latest")

				By("verifying the release image was mirrored")
				expectRepositoriesExist(*testRegistry, []string{"openshift/release-images"})
			})
		})
	})
})

var _ = Describe("deprecated platform.architectures field", Label("sparse-manifest"), func() {
	var workDir string

	BeforeEach(func() { workDir = setupWorkDir() })
	AfterEach(func() { cleanupWorkDir(workDir) })

	It("should mirror successfully when architectures includes 'multi'", func() {
		By("mirroring with deprecated architectures: [multi, amd64]")
		result, err := runner.MirrorToMirror(ctx,
			filepath.Join(iscDir, "sparse_manifest", "isc-archs-multi.yaml"),
			workDir, testRegistry.Endpoint(),
			"--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying the deprecation warning was printed")
		Expect(result.Stdout + result.Stderr).To(ContainSubstring(
			"platform.architectures is deprecated"))

		By("verifying multi-platform-image repository exists")
		expectRepositoriesExist(*testRegistry, []string{"oc-mirror/multi-platform-image"})
	})
})

var _ = Describe("negative", Label("sparse-manifest"), func() {
	var workDir string

	BeforeEach(func() { workDir = setupWorkDir() })
	AfterEach(func() { cleanupWorkDir(workDir) })

	Context("strict registry rejects sparse manifest", func() {
		It("should fail when pushing sparse manifest to a standard registry without platforms:none", func() {
			By("mirroring with amd64-only filter to a standard registry (no platforms:none config)")
			result, err := runner.MirrorToMirror(ctx,
				filepath.Join(iscDir, "sparse_manifest", "isc-additional-amd64.yaml"),
				workDir, testRegistry.Endpoint(),
				"--dest-tls-verify=false")
			By("verifying oc-mirror exits with an error related to manifest list validation")
			expectOcMirrorCommandFailure(result, err)
			Expect(result.Stdout+result.Stderr).To(SatisfyAny(
				ContainSubstring("Manifest list"),
				ContainSubstring("no instances found for platform"),
				ContainSubstring("preserve digests"),
			), "expected the failure to be related to manifest list platform filtering, not an unrelated error")
		})
	})
})

var _ = Describe("deprecated platform.architectures field with release channel", Label("sparse-manifest"), Ordered, func() {
	var workDir string
	var archRunner *ocmirror.Runner
	var cincinnatiServer *httptest.Server

	BeforeAll(func() {
		workDir = setupWorkDir()
		// Mock Cincinnati server: returns different release payloads depending on ?arch=
		const (
			releasePayload      = "oc-mirror/release/test-release-index:v0.0.1"
			multiReleasePayload = "oc-mirror/release/test-release-index-multi:v0.0.1"
		)
		cincinnatiServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") != "application/json" {
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}
			arch := r.URL.Query().Get("arch")
			var payload string
			switch arch {
			case "multi":
				// Use digest reference so GenerateReleaseSignatures can find the GPG signature
				payload = sourceImageDigestRef(multiReleasePayload)
			default: // amd64 and others — use digest reference for GPG signature lookup
				payload = sourceImageDigestRef(releasePayload)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"nodes":[{"version":"4.18.1","payload":"%s"}],"edges":[]}`, payload)
		}))
		binaryPath := os.Getenv("OC_MIRROR_BINARY")
		archRunner = ocmirror.NewRunner(binaryPath).
			WithEnv([]string{fmt.Sprintf("UPDATE_URL_OVERRIDE=%s", cincinnatiServer.URL)})
	})

	AfterAll(func() {
		cincinnatiServer.Close()
		cleanupWorkDir(workDir)
	})

	It("should make two Cincinnati calls (multi and amd64) and log deprecation warning", func() {
		By("mirroring with deprecated architectures [multi, amd64] and a release channel")
		result, err := archRunner.MirrorToMirror(ctx,
			filepath.Join(iscDir, "sparse_manifest", "isc-archs-multi-release.yaml"),
			workDir, testRegistry.Endpoint(),
			"--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying the deprecation warning was printed")
		Expect(result.Stdout + result.Stderr).To(ContainSubstring(
			"platform.architectures is deprecated"))

		By("verifying both release payloads and the additional image are in the registry")
		expectRepositoriesExist(*testRegistry, []string{
			"openshift/release-images",
			"oc-mirror/multi-platform-image",
		})
	})
})
