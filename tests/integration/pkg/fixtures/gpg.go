package fixtures

import (
	"bytes"
	"crypto"
	"encoding/json"
	"io"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	// golang.org/x/crypto/openpgp is deprecated, but it is what oc-mirror verifies release signatures with.
	"golang.org/x/crypto/openpgp"        //nolint:staticcheck
	"golang.org/x/crypto/openpgp/armor"  //nolint:staticcheck
	"golang.org/x/crypto/openpgp/packet" //nolint:staticcheck
)

// ReleaseSigner signs release images with an ephemeral GPG key, in the format of the release
// signatures oc-mirror downloads and verifies.
type ReleaseSigner struct {
	entity *openpgp.Entity
	config *packet.Config
}

// NewReleaseSigner generates a new GPG key to sign releases with.
func NewReleaseSigner() (*ReleaseSigner, error) {
	config := &packet.Config{DefaultHash: crypto.SHA512, RSABits: 2048}
	entity, err := openpgp.NewEntity("oc-mirror Release Test", "", "robot@test.com", config)
	if err != nil {
		return nil, err
	}
	return &ReleaseSigner{entity: entity, config: config}, nil
}

// PublicKey returns the armored public key that verifies the signatures.
func (s *ReleaseSigner) PublicKey() ([]byte, error) {
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	if err != nil {
		return nil, err
	}
	if err := s.entity.Serialize(w); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Sign returns the signature of the release image with the given digest, published as ref.
// Like `gpg --sign`, the signature is a compressed message with the signed payload inline.
func (s *ReleaseSigner) Sign(ref name.Tag, digest v1.Hash) ([]byte, error) {
	var payload struct {
		Critical struct {
			Identity struct {
				DockerReference string `json:"docker-reference"`
			} `json:"identity"`
			Image struct {
				DockerManifestDigest string `json:"docker-manifest-digest"`
			} `json:"image"`
			Type string `json:"type"`
		} `json:"critical"`
		Optional struct{} `json:"optional"`
	}
	payload.Critical.Identity.DockerReference = ref.Name()
	payload.Critical.Image.DockerManifestDigest = digest.String()
	payload.Critical.Type = "atomic container signature"
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var signature bytes.Buffer
	compressed, err := packet.SerializeCompressed(nopWriteCloser{&signature}, packet.CompressionZIP, nil)
	if err != nil {
		return nil, err
	}
	signed, err := openpgp.Sign(compressed, s.entity, nil, s.config)
	if err != nil {
		return nil, err
	}
	if _, err := signed.Write(data); err != nil {
		return nil, err
	}
	if err := signed.Close(); err != nil {
		return nil, err
	}
	if err := compressed.Close(); err != nil {
		return nil, err
	}
	return signature.Bytes(), nil
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error {
	return nil
}
