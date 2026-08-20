package release

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/folder"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func TestReleaseSignature(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()
	err := folder.CreateFolders(filepath.Join(tempDir, SignatureDir))
	assert.NoError(t, err)

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
		Destination:         consts.DockerProtocol + "localhost:5000/test",
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
		if err != nil {
			t.Fatal(err)
		}
		assert.Contains(t, res[0].Source, "quay.io/openshift-release-dev/ocp-release:4.11.46-aarch64")

		// signature not found
		newImgs[0].Source = "quay.io/openshift-release-dev/ocp-release@sha256:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34577"
		_, err = ex.GenerateReleaseSignatures(context.Background(), newImgs)
		assert.Equal(t, "[GenerateReleaseSignatures] no signature found for 37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34577 image quay.io/openshift-release-dev/ocp-release@sha256:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34577", err.Error())

		// write file error
		opts.Global.WorkingDir = "none"
		newImgs[0].Source = "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34531"

		_, err = ex.GenerateReleaseSignatures(context.Background(), newImgs)
		if err == nil {
			t.Fatal("should fail")
		}
	})

	t.Run("Testing ReleaseSignature with custom PGP key - should pass", func(t *testing.T) {
		t.Setenv("OCP_SIGNATURE_VERIFICATION_PK", consts.TestFolder+"custom-ocp-sig-key.asc")
		tmpDir := t.TempDir()
		workingDir := filepath.Join(tmpDir, "working-dir")
		err := folder.CreateFolders(filepath.Join(workingDir, SignatureDir))
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
		if err != nil {
			t.Fatal(err)
		}
		assert.Contains(t, res[0].Source, "quay.io/openshift-release-dev/ocp-release:4.11.46-aarch64")
	})

	t.Run("Testing ReleaseSignature with custom but buggy PGP key - should fail", func(t *testing.T) {
		t.Setenv("OCP_SIGNATURE_VERIFICATION_PK", consts.TestFolder+"buggy-ocp-sig-key.asc")

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
		t.Setenv("OCP_SIGNATURE_VERIFICATION_PK", consts.TestFolder+"inexisting-ocp-sig-key.asc")

		tmpDir := t.TempDir()
		workingDir := filepath.Join(tmpDir, "working-dir")
		err := folder.CreateFolders(filepath.Join(workingDir, SignatureDir))
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
		if err != nil {
			t.Fatal(err)
		}
		assert.Contains(t, res[0].Source, "quay.io/openshift-release-dev/ocp-release:4.11.46-aarch64")
	})
}

// TestReleaseSignatureRejectsTamperedSignature verifies that a release whose signature
// blob is well-formed (valid armored key, valid OpenPGP message, well-formed signed
// JSON body) but whose cryptographic signature does not verify (e.g. tampered/corrupted
// in transit, or forged) is rejected by GenerateReleaseSignatures.
//
// This guards against a regression where openpgp.MessageDetails.SignatureError was
// checked immediately after openpgp.ReadMessage() returned, before md.UnverifiedBody
// had been read. Per the openpgp API, the signature can only be verified once the body
// has been fully consumed, so checking SignatureError before that point is always nil
// and never actually rejects a bad signature.
func TestReleaseSignatureRejectsTamperedSignature(t *testing.T) {
	log := clog.New("trace")

	// Generate a throwaway PGP identity to sign/verify the test message with,
	// so this test does not depend on network access or the real OCP signing key.
	hashConfig := &packet.Config{DefaultHash: crypto.SHA256}
	entity, err := openpgp.NewEntity("oc-mirror test signer", "", "test@example.com", hashConfig)
	assert.NoError(t, err)

	var pubKeyBuf bytes.Buffer
	armorWriter, err := armor.Encode(&pubKeyBuf, openpgp.PublicKeyType, nil)
	assert.NoError(t, err)
	assert.NoError(t, entity.Serialize(armorWriter))
	assert.NoError(t, armorWriter.Close())

	digestHex := fmt.Sprintf("%x", sha256.Sum256([]byte("tampered-signature-test")))
	signedRef := "quay.io/openshift-release-dev/ocp-release@sha256:" + digestHex

	// Build the same JSON payload shape that a real "atomic container signature" carries.
	var content v2alpha1.SignatureContentSchema
	content.Critical.Type = "atomic container signature"
	content.Critical.Identity.DockerReference = signedRef
	content.Critical.Image.DockerManifestDigest = "sha256:" + digestHex
	payload, err := json.Marshal(content)
	assert.NoError(t, err)

	// Produce a validly signed OpenPGP message wrapping that payload.
	var signedBuf bytes.Buffer
	signer, err := openpgp.Sign(&signedBuf, entity, nil, hashConfig)
	assert.NoError(t, err)
	_, err = signer.Write(payload)
	assert.NoError(t, err)
	assert.NoError(t, signer.Close())

	// Tamper with the very last byte of the message: the trailing Signature packet is
	// always serialized last (after the literal data content, regardless of any
	// chunking), so this corrupts only the signature's cryptographic material while
	// leaving the signed JSON payload completely intact and parseable.
	tampered := append([]byte{}, signedBuf.Bytes()...)
	tampered[len(tampered)-1] ^= 0xFF

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tampered)
	}))
	defer server.Close()

	t.Setenv("OCP_SIGNATURE_URL", server.URL+"/")

	pubKeyFile := filepath.Join(t.TempDir(), "test-pub.asc")
	assert.NoError(t, os.WriteFile(pubKeyFile, pubKeyBuf.Bytes(), 0o600))
	t.Setenv("OCP_SIGNATURE_VERIFICATION_PK", pubKeyFile)

	workingDir := filepath.Join(t.TempDir(), "working-dir")
	assert.NoError(t, folder.CreateFolders(filepath.Join(workingDir, SignatureDir)))

	global := &mirror.GlobalOptions{SecurePolicy: false, WorkingDir: workingDir}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	opts := mirror.CopyOptions{
		Global:      global,
		SrcImage:    srcOpts,
		DestImage:   destOpts,
		RetryOpts:   retryOpts,
		Destination: consts.DockerProtocol + "localhost:5000/test",
		Mode:        mirror.DiskToMirror,
	}

	ex := NewSignatureClient(log, v2alpha1.ImageSetConfiguration{}, opts)

	imgs := []v2alpha1.CopyImageSchema{
		{
			Source:      signedRef,
			Destination: "localhost:9999/ocp-release:tampered",
		},
	}

	_, err = ex.GenerateReleaseSignatures(context.Background(), imgs)
	assert.Error(t, err, "a release with a tampered/invalid signature must be rejected")
	assert.Contains(t, err.Error(), "signature error")
}
