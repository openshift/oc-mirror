package testutils

import (
	"bytes"
	"crypto"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	//nolint:staticcheck
	"golang.org/x/crypto/openpgp"
	//nolint:staticcheck
	"golang.org/x/crypto/openpgp/armor"
	//nolint:staticcheck
	"golang.org/x/crypto/openpgp/packet"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

// ReleaseSignatureFixture holds a throwaway PGP key and an HTTP server that
// serves a signature for a locally generated fake release image. Integration
// tests use this instead of a checked-in signature file whose embedded digest
// drifts when GenerateFakeRelease's OCI digest changes.
type ReleaseSignatureFixture struct {
	Server        *httptest.Server
	PublicKeyPath string
}

// SetupReleaseSignatureFixture creates a throwaway PGP identity, writes its
// public key under dir, and starts an HTTP server that responds to the
// oc-mirror signature lookup path (baseURL + "sha256=" + digest + "/signature-1")
// with a payload whose docker-manifest-digest matches releaseDigest.
//
// Callers should set:
//   - OCP_SIGNATURE_VERIFICATION_PK to PublicKeyPath
//   - OCP_SIGNATURE_URL to Server.URL + "/"
//
// and close Server when finished.
func SetupReleaseSignatureFixture(dir, releaseDigest, dockerReference string) (*ReleaseSignatureFixture, error) {
	digestHex := strings.TrimPrefix(releaseDigest, "sha256:")
	manifestDigest := "sha256:" + digestHex

	hashConfig := &packet.Config{DefaultHash: crypto.SHA256}
	entity, pubKeyPath, err := writeTestSigningKey(dir, hashConfig)
	if err != nil {
		return nil, err
	}

	signed, err := signReleaseSignaturePayload(entity, hashConfig, dockerReference, manifestDigest)
	if err != nil {
		return nil, err
	}

	expectedPath := "/sha256=" + digestHex + "/signature-1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(signed)
	}))

	return &ReleaseSignatureFixture{
		Server:        server,
		PublicKeyPath: pubKeyPath,
	}, nil
}

func writeTestSigningKey(dir string, hashConfig *packet.Config) (*openpgp.Entity, string, error) {
	entity, err := openpgp.NewEntity("oc-mirror integration test signer", "", "test@example.com", hashConfig)
	if err != nil {
		return nil, "", fmt.Errorf("create test pgp entity: %w", err)
	}

	var pubKeyBuf bytes.Buffer
	armorWriter, err := armor.Encode(&pubKeyBuf, openpgp.PublicKeyType, nil)
	if err != nil {
		return nil, "", fmt.Errorf("armor public key: %w", err)
	}
	if err := entity.Serialize(armorWriter); err != nil {
		return nil, "", fmt.Errorf("serialize public key: %w", err)
	}
	if err := armorWriter.Close(); err != nil {
		return nil, "", fmt.Errorf("close public key armor: %w", err)
	}

	pubKeyPath := filepath.Join(dir, "test-ocp-sig-key.asc")
	if err := os.WriteFile(pubKeyPath, pubKeyBuf.Bytes(), 0o600); err != nil {
		return nil, "", fmt.Errorf("write public key: %w", err)
	}
	return entity, pubKeyPath, nil
}

func signReleaseSignaturePayload(entity *openpgp.Entity, hashConfig *packet.Config, dockerReference, manifestDigest string) ([]byte, error) {
	var content v2alpha1.SignatureContentSchema
	content.Critical.Type = "atomic container signature"
	content.Critical.Identity.DockerReference = dockerReference
	content.Critical.Image.DockerManifestDigest = manifestDigest
	content.Optional.Creator = "oc-mirror integration test"

	payload, err := json.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("marshal signature content: %w", err)
	}

	var signedBuf bytes.Buffer
	signer, err := openpgp.Sign(&signedBuf, entity, nil, hashConfig)
	if err != nil {
		return nil, fmt.Errorf("create pgp signer: %w", err)
	}
	if _, err := signer.Write(payload); err != nil {
		return nil, fmt.Errorf("write signed payload: %w", err)
	}
	if err := signer.Close(); err != nil {
		return nil, fmt.Errorf("close pgp signer: %w", err)
	}
	return signedBuf.Bytes(), nil
}
