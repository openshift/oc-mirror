// Package testimages builds every container image used as mirroring source by the
// integration tests and pushes them to a local registry.
//
// The images are built from the sources under testdata/images at the start of each
// test session, so a change to a test catalog or bundle is picked up by the same
// pull request that introduces it, without publishing anything to a remote registry.
// Operator catalogs are rendered with opm from their basic catalog templates.
// Images are cosign-signed (except those meant to be unsigned), and release
// payloads also get the GPG signatures oc-mirror verifies.
package testimages

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/sigstore/pkg/signature"
	"golang.org/x/crypto/openpgp" //nolint:staticcheck // see signing.go

	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

const (
	// SourceRegistryPort is the port the source registry must listen on.
	SourceRegistryPort = 5001
	// SourceRegistry is the host:port that test images, catalogs and
	// ImageSetConfigurations use to reference source images
	// (e.g. localhost:5001/oc-mirror/oc-mirror-dev:test-catalog-latest).
	SourceRegistry = "localhost:5001"
)

// Options configures BuildAndPush.
type Options struct {
	// ImagesDir holds the sources of the images (testdata/images).
	ImagesDir string
	// CosignKeyPath is a cosign private key without passphrase. Every image is
	// signed with it.
	CosignKeyPath string
	// ReleaseSignaturesDir receives one GPG signature per release payload, named
	// as oc-mirror expects them in working-dir/signatures.
	ReleaseSignaturesDir string
	// ReleasePublicKeyPath receives the armored GPG public key that verifies the
	// release signatures, for OCP_SIGNATURE_VERIFICATION_PK.
	ReleasePublicKeyPath string
}

// BuildAndPush builds all test images from opts.ImagesDir and pushes them to reg,
// which must listen on SourceRegistry. The returned Inventory resolves the digests
// of the pushed images.
func BuildAndPush(ctx context.Context, reg *registry.Registry, opts Options) (*Inventory, error) {
	if reg.Endpoint() != SourceRegistry {
		return nil, fmt.Errorf("test images reference each other through %s, but the registry listens on %s",
			SourceRegistry, reg.Endpoint())
	}
	cosignSigner, err := loadCosignSigner(opts.CosignKeyPath)
	if err != nil {
		return nil, err
	}
	releaseSigner, err := newReleaseSigner()
	if err != nil {
		return nil, err
	}
	b := &builder{
		reg:                  reg,
		remoteOpts:           reg.RemoteOptions(ctx),
		cosignSigner:         cosignSigner,
		releaseSigner:        releaseSigner,
		releaseSignaturesDir: opts.ReleaseSignaturesDir,
		inventory:            newInventory(),
	}

	if err := b.buildOperatorImages(ctx, filepath.Join(opts.ImagesDir, "operator")); err != nil {
		return nil, fmt.Errorf("building operator images: %w", err)
	}
	if err := b.buildReleaseImages(filepath.Join(opts.ImagesDir, "release")); err != nil {
		return nil, fmt.Errorf("building release images: %w", err)
	}
	if err := b.buildAdditionalImages(); err != nil {
		return nil, fmt.Errorf("building additional images: %w", err)
	}
	if err := b.writeReleasePublicKey(opts.ReleasePublicKeyPath); err != nil {
		return nil, fmt.Errorf("writing release public key: %w", err)
	}

	return b.inventory, nil
}

// builder pushes images to the source registry, signs them, and records their digests.
type builder struct {
	reg                  *registry.Registry
	remoteOpts           []remote.Option
	cosignSigner         signature.Signer
	releaseSigner        *openpgp.Entity
	releaseSignaturesDir string
	inventory            *Inventory
}

// pushImage pushes and signs img as <repoTag> in the source registry, and records its digest.
func (b *builder) pushImage(repoTag string, img v1.Image) error {
	if err := b.pushUnsignedImage(repoTag, img); err != nil {
		return err
	}
	digest, err := img.Digest()
	if err != nil {
		return err
	}
	return b.sign(repository(repoTag), digest)
}

// pushIndex pushes and signs idx (and its manifests) as <repoTag> in the source
// registry, and records its digest.
func (b *builder) pushIndex(repoTag string, idx v1.ImageIndex) error {
	if err := b.pushUnsignedIndex(repoTag, idx); err != nil {
		return err
	}
	return b.signIndex(repository(repoTag), idx)
}

// pushUnsignedImage pushes img as <repoTag> in the source registry, and records its digest.
func (b *builder) pushUnsignedImage(repoTag string, img v1.Image) error {
	ref, err := b.reg.Reference(repoTag)
	if err != nil {
		return fmt.Errorf("invalid image reference %q: %w", repoTag, err)
	}
	if err := remote.Write(ref, img, b.remoteOpts...); err != nil {
		return fmt.Errorf("pushing %s: %w", ref, err)
	}
	digest, err := img.Digest()
	if err != nil {
		return fmt.Errorf("computing digest of %s: %w", ref, err)
	}
	b.inventory.add(repoTag, digest)
	return nil
}

// pushUnsignedIndex pushes idx as <repoTag> in the source registry, and records its digest.
func (b *builder) pushUnsignedIndex(repoTag string, idx v1.ImageIndex) error {
	ref, err := b.reg.Reference(repoTag)
	if err != nil {
		return fmt.Errorf("invalid image reference %q: %w", repoTag, err)
	}
	if err := remote.WriteIndex(ref, idx, b.remoteOpts...); err != nil {
		return fmt.Errorf("pushing %s: %w", ref, err)
	}
	digest, err := idx.Digest()
	if err != nil {
		return fmt.Errorf("computing digest of %s: %w", ref, err)
	}
	b.inventory.add(repoTag, digest)
	return nil
}

// repository returns the repository part of "<repository>:<tag>".
func repository(repoTag string) string {
	return repoTag[:strings.LastIndex(repoTag, ":")]
}
