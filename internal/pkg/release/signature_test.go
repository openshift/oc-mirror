package release

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	//nolint:staticcheck
	"golang.org/x/crypto/openpgp"
	//nolint:staticcheck
	"golang.org/x/crypto/openpgp/armor"
	//nolint:staticcheck
	"golang.org/x/crypto/openpgp/packet"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func TestReleaseSignature(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()
	_ = os.MkdirAll(tempDir+"/"+SignatureDir, 0o755)
	defer os.RemoveAll(tempDir)

	global := &mirror.GlobalOptions{
		SecurePolicy: false,
		WorkingDir:   tempDir,
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "docker://localhost:5000/test",
		Dev:                 false,
		Mode:                mirror.DiskToMirror,
	}

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Platform: v2alpha1.Platform{
					Architectures: []string{"amd64"},
					Channels: []v2alpha1.ReleaseChannel{
						{
							Name:       "stable-4.13",
							MinVersion: "4.13.10",
							MaxVersion: "4.13.10",
						},
					},
					Graph: true,
				},
			},
		},
	}

	t.Run("Testing ReleaseSignature - should pass", func(t *testing.T) {
		ex := NewSignatureClient(log, cfg, opts)
		var imgs []v2alpha1.CopyImageSchema
		var newImgs []v2alpha1.CopyImageSchema

		imgs = append(imgs, v2alpha1.CopyImageSchema{
			Source:      "quay.io/openshift-release-dev/ocp-release-4.13.10-x86_64",
			Destination: "localhost:9999/ocp-release:4.13.10-x86_64",
		})

		_, err := ex.GenerateReleaseSignatures(context.Background(), imgs)
		assert.Equal(t, "[GenerateReleaseSignatures] parsing image digest", err.Error())

		newImgs = append(newImgs, v2alpha1.CopyImageSchema{
			Source:      "quay.io/openshift-release-dev/ocp-release@sha256:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34531",
			Destination: "localhost:9999/ocp-release:4.13.10-x86_64",
		})

		res, err := ex.GenerateReleaseSignatures(context.Background(), newImgs)
		assert.NoError(t, err)
		assert.Contains(t, res[0].Source, "quay.io/openshift-release-dev/ocp-release:4.11.46-aarch64")

		// signature not found
		newImgs[0].Source = "quay.io/openshift-release-dev/ocp-release@sha256:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34577"
		_, err = ex.GenerateReleaseSignatures(context.Background(), newImgs)
		assert.Equal(t, "[GenerateReleaseSignatures] no signature found for 37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34577 image quay.io/openshift-release-dev/ocp-release@sha256:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34577", err.Error())

		// write file error
		opts.Global.WorkingDir = "none"
		newImgs[0].Source = "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34531"

		_, err = ex.GenerateReleaseSignatures(context.Background(), newImgs)
		assert.Error(t, err, "should fail")
	})

	t.Run("Testing ReleaseSignature with custom PGP key - should pass", func(t *testing.T) {
		t.Setenv("OCP_SIGNATURE_VERIFICATION_PK", common.TestFolder+"custom-ocp-sig-key.asc")
		tmpDir := t.TempDir()
		workingDir := tmpDir + "/" + "working-dir"
		err := os.MkdirAll(workingDir+SignatureDir, 0o755)
		assert.NoError(t, err)
		defer os.RemoveAll(workingDir)
		opts.Global.WorkingDir = workingDir
		ex := NewSignatureClient(log, cfg, opts)

		imgs := []v2alpha1.CopyImageSchema{
			{
				Source:      "quay.io/openshift-release-dev/ocp-release@sha256:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34531",
				Destination: "localhost:9999/ocp-release:4.13.10-x86_64",
			},
		}

		res, err := ex.GenerateReleaseSignatures(context.Background(), imgs)
		assert.NoError(t, err)
		assert.Contains(t, res[0].Source, "quay.io/openshift-release-dev/ocp-release:4.11.46-aarch64")
	})

	t.Run("Testing ReleaseSignature with custom but buggy PGP key - should fail", func(t *testing.T) {
		t.Setenv("OCP_SIGNATURE_VERIFICATION_PK", common.TestFolder+"buggy-ocp-sig-key.asc")

		ex := NewSignatureClient(log, cfg, opts)

		imgs := []v2alpha1.CopyImageSchema{
			{
				Source:      "quay.io/openshift-release-dev/ocp-release@sha256:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34531",
				Destination: "localhost:9999/ocp-release:4.13.10-x86_64",
			},
		}

		_, err := ex.GenerateReleaseSignatures(context.Background(), imgs)

		assert.Error(t, err)
	})

	t.Run("Testing ReleaseSignature with custom but inexisting PGP key - should pass", func(t *testing.T) {
		t.Setenv("OCP_SIGNATURE_VERIFICATION_PK", common.TestFolder+"inexisting-ocp-sig-key.asc")

		tmpDir := t.TempDir()
		workingDir := tmpDir + "/" + "working-dir"
		err := os.MkdirAll(workingDir+SignatureDir, 0o755)
		assert.NoError(t, err)
		defer os.RemoveAll(workingDir)
		opts.Global.WorkingDir = workingDir
		ex := NewSignatureClient(log, cfg, opts)

		imgs := []v2alpha1.CopyImageSchema{
			{
				Source:      "quay.io/openshift-release-dev/ocp-release@sha256:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34531",
				Destination: "localhost:9999/ocp-release:4.13.10-x86_64",
			},
		}

		res, err := ex.GenerateReleaseSignatures(context.Background(), imgs)
		assert.NoError(t, err)
		assert.Contains(t, res[0].Source, "quay.io/openshift-release-dev/ocp-release:4.11.46-aarch64")
	})
}

// TestVerifySignature exercises SignatureSchema.verifySignature in isolation, now that
// signature verification is its own function independent of caching/HTTP concerns. It
// uses a throwaway PGP identity generated at test time, so none of these cases depend on
// network access or the real OCP signing key.
func TestVerifySignature(t *testing.T) {
	log := clog.New("trace")

	hashConfig := &packet.Config{DefaultHash: crypto.SHA256}
	entity, err := openpgp.NewEntity("oc-mirror test signer", "", "test@example.com", hashConfig)
	assert.NoError(t, err)

	var pubKeyBuf bytes.Buffer
	armorWriter, err := armor.Encode(&pubKeyBuf, openpgp.PublicKeyType, nil)
	assert.NoError(t, err)
	assert.NoError(t, entity.Serialize(armorWriter))
	assert.NoError(t, armorWriter.Close())

	// sign returns an OpenPGP-signed message wrapping the JSON encoding of content,
	// matching the format of a real "atomic container signature".
	sign := func(t *testing.T, content v2alpha1.SignatureContentSchema) []byte {
		t.Helper()
		payload, err := json.Marshal(content)
		assert.NoError(t, err)

		var signedBuf bytes.Buffer
		signer, err := openpgp.Sign(&signedBuf, entity, nil, hashConfig)
		assert.NoError(t, err)
		_, err = signer.Write(payload)
		assert.NoError(t, err)
		assert.NoError(t, signer.Close())
		return signedBuf.Bytes()
	}

	digestHex := fmt.Sprintf("%x", sha256.Sum256([]byte("verify-signature-test")))
	signedRef := "quay.io/openshift-release-dev/ocp-release@sha256:" + digestHex

	var validContent v2alpha1.SignatureContentSchema
	validContent.Critical.Type = "atomic container signature"
	validContent.Critical.Identity.DockerReference = signedRef
	validContent.Critical.Image.DockerManifestDigest = "sha256:" + digestHex

	ex := SignatureSchema{Log: log, pgpKey: pubKeyBuf.String()}

	t.Run("valid signature is accepted", func(t *testing.T) {
		data := sign(t, validContent)

		src, err := ex.verifySignature("sha256", digestHex, data)
		assert.NoError(t, err)
		assert.Equal(t, signedRef, src)
	})

	t.Run("tampered signature is rejected", func(t *testing.T) {
		// This guards against a regression where openpgp.MessageDetails.SignatureError
		// was checked immediately after openpgp.ReadMessage() returned, before
		// md.UnverifiedBody had been read: per the openpgp API, the signature can only
		// be verified once the body has been fully consumed.
		data := sign(t, validContent)

		// Tamper with the very last byte of the message: the trailing Signature packet
		// is always serialized last (after the literal data content, regardless of any
		// chunking), so this corrupts only the signature's cryptographic material while
		// leaving the signed JSON payload completely intact and parseable.
		data[len(data)-1] ^= 0xFF

		_, err := ex.verifySignature("sha256", digestHex, data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature error")
	})

	t.Run("mismatched digest is rejected", func(t *testing.T) {
		// The payload is validly signed, but it attests to a different image digest
		// than the one we're asking to verify.
		mismatched := validContent
		mismatched.Critical.Image.DockerManifestDigest = "sha256:" + fmt.Sprintf("%x", sha256.Sum256([]byte("a-different-image")))
		data := sign(t, mismatched)

		_, err := ex.verifySignature("sha256", digestHex, data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mismatched digest")
	})
}

// TestSaveSignatureSanitizesPathTraversal verifies that saveSignature cannot be made to
// write outside of the signature cache directory, even when the docker-reference tag it
// derives the destination filename from is attacker/signer-controlled.
//
// image.ParseRef does not validate its "tag" component against the Docker reference
// grammar, so a docker-reference such as "repo:../pwned" yields Tag == "../pwned"
// verbatim. Without sanitization, joining that into a file path lets a maliciously (or
// accidentally) signed release payload write its cached signature one directory level
// above the intended <workingDir>/signatures/ cache dir - i.e. directly into workingDir,
// where it could collide with/overwrite other oc-mirror state (CWE-22). A tag with more
// "../" segments could escape arbitrarily further up the filesystem the same way.
func TestSaveSignatureSanitizesPathTraversal(t *testing.T) {
	log := clog.New("trace")

	workingDir := t.TempDir()
	err := os.MkdirAll(filepath.Join(workingDir, SignatureDir), 0o755)
	assert.NoError(t, err)

	ex := SignatureSchema{Log: log, Opts: mirror.CopyOptions{Global: &mirror.GlobalOptions{WorkingDir: workingDir}}}

	maliciousRef := "quay.io/openshift-release-dev/ocp-release:../pwned"
	err = ex.saveSignature(maliciousRef, "deadbeef", []byte("payload"))
	assert.NoError(t, err)

	// The file must never be written directly into workingDir (i.e. one level above
	// the signature cache dir, which is what the unsanitized "../pwned" tag targets).
	_, statErr := os.Stat(filepath.Join(workingDir, "pwned-sha256-deadbeef"))
	assert.True(t, os.IsNotExist(statErr), "signature file must not escape the signature cache directory")

	// It must instead land safely inside the signature cache directory, with the
	// traversal sequence stripped out of the filename entirely.
	entries, err := os.ReadDir(filepath.Join(workingDir, SignatureDir))
	assert.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "pwned-sha256-deadbeef", entries[0].Name())
}
