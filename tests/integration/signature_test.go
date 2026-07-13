package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
)

var _ = Describe("signatures", func() {
	var workDir string

	iscSignatures := filepath.Join("signatures", "isc-signatures.yaml")
	discSignatures := filepath.Join("signatures", "disc-signatures.yaml")

	BeforeEach(func() {
		workDir = setupWorkDir()
	})

	AfterEach(func() {
		cleanupWorkDir(workDir)
	})

	It("should not mirror signatures when using --remove-signatures=true", func() {
		By("running mirrorToDisk with --remove-signatures=true")
		result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscSignatures), workDir, "--remove-signatures=true")
		expectOcMirrorCommandSuccess(result, err)

		By("running diskToMirror with --remove-signatures=true")
		result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscSignatures), workDir, testRegistry.Endpoint(),
			"--remove-signatures=true", "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying no signatures are present in the local registry")
		expectNoSignaturesInRegistry(*testRegistry)
	})

	It("should mirror with signatures preserved and delete them", func() {
		deleteYaml := filepath.Join(workDir, "working-dir", "delete", "delete-images.yaml")

		By("running mirrorToDisk")
		result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscSignatures), workDir)
		expectOcMirrorCommandSuccess(result, err)

		By("running diskToMirror")
		result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscSignatures), workDir, testRegistry.Endpoint(), "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying signatures are present in the local registry")
		expectSignaturesInRegistry(*testRegistry)

		By("verifying the signature configmap was generated correctly")
		expectSignatureConfigMapGenerated(workDir)

		By("running delete with --delete-signatures")
		result, err = runner.DeletePhaseOne(ctx, filepath.Join(iscDir, discSignatures), workDir, "", testRegistry.Endpoint(),
			"--delete-signatures")
		expectOcMirrorCommandSuccess(result, err)

		result, err = runner.DeletePhaseTwo(ctx, deleteYaml, testRegistry.Endpoint(),
			"--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying non-catalog signatures are deleted")
		expectNonCatalogSignaturesDeleted(*testRegistry)
	})

	It("should keep signature tags when delete does not use --delete-signatures", func() {
		deleteYaml := filepath.Join(workDir, "working-dir", "delete", "delete-images.yaml")

		By("running mirrorToDisk")
		result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscSignatures), workDir)
		expectOcMirrorCommandSuccess(result, err)

		By("running diskToMirror")
		result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscSignatures), workDir, testRegistry.Endpoint(), "--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying signatures are present in the local registry")
		expectSignaturesInRegistry(*testRegistry)

		By("running delete without --delete-signatures")
		result, err = runner.DeletePhaseOne(ctx, filepath.Join(iscDir, discSignatures), workDir, "", testRegistry.Endpoint())
		expectOcMirrorCommandSuccess(result, err)

		result, err = runner.DeletePhaseTwo(ctx, deleteYaml, testRegistry.Endpoint(),
			"--dest-tls-verify=false")
		expectOcMirrorCommandSuccess(result, err)

		By("verifying signature tags are still present")
		expectOnlySignatureTagsRemain(*testRegistry)
	})

	It("fails gracefully when it fails to retrieve the release signature", func() {
		// Use a plain, unseeded working directory so no cached signature is found,
		// forcing oc-mirror to fetch it from OCP_SIGNATURE_URL.
		unseededWorkDir, err := os.MkdirTemp("", "oc-mirror-test-*")
		Expect(err).NotTo(HaveOccurred())
		defer cleanupWorkDir(unseededWorkDir)

		By("running mirrorToDisk with an unreachable signature server")
		failingSigRunner := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY")).
			WithEnv([]string{"OCP_SIGNATURE_URL=http://127.0.0.1:1/"})
		result, err := failingSigRunner.MirrorToDisk(ctx, filepath.Join(iscDir, iscSignatures), unseededWorkDir)

		By("verifying oc-mirror failed gracefully instead of panicking")
		expectOcMirrorExitCode(result, err, 2, "collection error", "http request")
		expectNoTarArchive(unseededWorkDir)
	})

	Describe("release signature configmap", func() {
		It("should not generate a signature configmap when --ignore-release-signature is used", func() {
			// Use a plain, unseeded working directory to guarantee no signature is ever
			// cached, regardless of --ignore-release-signature.
			unseededWorkDir, err := os.MkdirTemp("", "oc-mirror-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer cleanupWorkDir(unseededWorkDir)

			By("running mirrorToDisk with --ignore-release-signature")
			result, err := runner.MirrorToDisk(ctx, filepath.Join(iscDir, iscSignatures), unseededWorkDir, "--ignore-release-signature")
			expectOcMirrorCommandSuccess(result, err)

			By("running diskToMirror with --ignore-release-signature")
			result, err = runner.DiskToMirror(ctx, filepath.Join(iscDir, iscSignatures), unseededWorkDir, testRegistry.Endpoint(),
				"--ignore-release-signature", "--dest-tls-verify=false")
			expectOcMirrorCommandSuccess(result, err)

			By("verifying no signature configmap was generated since no signature was ever cached")
			expectNoSignatureConfigMap(unseededWorkDir)
		})
	})
})

// expectSignatureConfigMapGenerated verifies that the release signature configmap
// (both JSON and YAML) was generated in working-dir/cluster-resources with the
// expected static name, and that its binaryData matches the cached signature files
// in working-dir/signatures byte-for-byte.
func expectSignatureConfigMapGenerated(workDir string) {
	crDir := filepath.Join(workDir, dirWorkingDir, dirClusterResources)

	var jsonCM corev1.ConfigMap
	jsonData, err := os.ReadFile(filepath.Join(crDir, "signature-configmap.json"))
	Expect(err).NotTo(HaveOccurred(), "signature-configmap.json not found")
	Expect(json.Unmarshal(jsonData, &jsonCM)).To(Succeed(), "failed to unmarshal signature-configmap.json")

	var yamlCM corev1.ConfigMap
	yamlData, err := os.ReadFile(filepath.Join(crDir, "signature-configmap.yaml"))
	Expect(err).NotTo(HaveOccurred(), "signature-configmap.yaml not found")
	Expect(yaml.Unmarshal(yamlData, &yamlCM)).To(Succeed(), "failed to unmarshal signature-configmap.yaml")

	for _, cm := range []corev1.ConfigMap{jsonCM, yamlCM} {
		Expect(cm.Name).To(Equal("mirrored-release-signatures"))
		Expect(cm.BinaryData).NotTo(BeEmpty())
		for key := range cm.BinaryData {
			Expect(key).To(MatchRegexp(`^sha256-[0-9a-f]{64}-\d+$`), "unexpected binaryData key format: %s", key)
		}
	}
	Expect(jsonCM.BinaryData).To(Equal(yamlCM.BinaryData), "json and yaml signature configmaps disagree on binaryData")

	sigDir := filepath.Join(workDir, dirWorkingDir, "signatures")
	entries, err := os.ReadDir(sigDir)
	Expect(err).NotTo(HaveOccurred())
	Expect(entries).NotTo(BeEmpty(), "no cached signature files found in %s", sigDir)

	for _, f := range entries {
		parts := strings.SplitN(f.Name(), "-sha256-", 2)
		Expect(parts).To(HaveLen(2), "unexpected signature file name: %s", f.Name())
		digest := parts[1]

		data, err := os.ReadFile(filepath.Join(sigDir, f.Name()))
		Expect(err).NotTo(HaveOccurred())

		found := false
		for key, val := range jsonCM.BinaryData {
			if strings.Contains(key, digest) {
				Expect(val).To(Equal(data), "binaryData content mismatch for key %s", key)
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), "no binaryData entry found for signature file %s", f.Name())
	}
}

// expectNoSignatureConfigMap verifies that no release signature configmap was generated.
func expectNoSignatureConfigMap(workDir string) {
	crDir := filepath.Join(workDir, dirWorkingDir, dirClusterResources)

	_, errJSON := os.Stat(filepath.Join(crDir, "signature-configmap.json"))
	Expect(os.IsNotExist(errJSON)).To(BeTrue(), "expected signature-configmap.json to not exist")

	_, errYAML := os.Stat(filepath.Join(crDir, "signature-configmap.yaml"))
	Expect(os.IsNotExist(errYAML)).To(BeTrue(), "expected signature-configmap.yaml to not exist")
}
