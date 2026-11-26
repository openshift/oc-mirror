// Package origin runs the registry that the integration tests mirror operator catalogs and
// releases from. All of its images are built from their sources in testdata when it starts:
// catalogs, with their bundles and operands, and releases, with their components. This way,
// changing any of them does not require publishing anything to a public registry.
package origin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/openshift/oc-mirror/tests/integration/pkg/fixtures"
	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

// defaultPlatform is the platform of the images that are not multi-arch.
var defaultPlatform = v1.Platform{OS: "linux", Architecture: "amd64"}

// Config describes the images an origin registry serves. Every subdirectory of the directories
// below is the source of one image, named after it, and other files, like READMEs, are ignored.
// A directory left empty is skipped.
type Config struct {
	// RegistryConfig is the configuration file of the distribution registry.
	RegistryConfig string

	// OperatorRepo holds the catalogs, bundles and operands, tagged with their directory name.
	OperatorRepo string
	// CatalogsDir has file-based catalogs. Their bundles can be placeholders, rendered from the
	// bundles of BundlesDir: see fixtures.RenderBundles.
	CatalogsDir string
	// BundlesDir has operator bundles.
	BundlesDir string
	// OperandsDir has the image fixtures the bundles reference: see fixtures.ImageSpec.
	OperandsDir string

	// ReleaseRepo is the parent of the repositories of the releases and of their components,
	// published as ReleaseRepo/<directory>:ReleaseTag.
	ReleaseRepo string
	ReleaseTag  string
	// ReleasesDir has release payloads.
	ReleasesDir string
	// ComponentsDir has the image fixtures the releases reference: see fixtures.ImageSpec.
	ComponentsDir string

	// MultiArch maps the images built as an image index, by directory name, to their platforms.
	// The other ones are built for linux/amd64.
	MultiArch map[string][]v1.Platform
	// OPMImage is the base image of the catalogs, see fixtures.OPMImage.
	OPMImage string
	// CosignKey is the cosign private key, without password, that signs every image.
	CosignKey string
	// WorkDir is where the sources are rendered, and where the release signatures are written.
	WorkDir string
}

// Registry serves the images built from testdata over HTTPS, like a public registry.
type Registry struct {
	*registry.Registry

	// digests maps the reference by tag of every image to its digest.
	digests              map[string]string
	releaseSignaturesDir string
	releasePublicKeyFile string
}

// Start starts an origin registry, then builds the images of cfg and pushes them, signed, to it.
func Start(ctx context.Context, cfg Config) (*Registry, error) {
	reg, err := registry.StartTLS(ctx, cfg.RegistryConfig, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to start the origin registry: %w", err)
	}
	r := &Registry{Registry: reg, digests: map[string]string{}}
	if err := r.publish(ctx, cfg); err != nil {
		return nil, errors.Join(err, reg.Stop())
	}
	return r, nil
}

// Image returns the reference of the image published as repoTag, a repository and tag like
// oc-mirror/oc-mirror-dev:test-catalog.
func (r *Registry) Image(repoTag string) (string, error) {
	ref, err := name.NewTag(r.Endpoint() + "/" + repoTag)
	if err != nil {
		return "", err
	}
	if _, ok := r.digests[ref.Name()]; !ok {
		return "", fmt.Errorf("%s was not built from testdata", repoTag)
	}
	return ref.Name(), nil
}

// ByDigest returns the reference by digest of the image with the given reference by tag.
func (r *Registry) ByDigest(ref string) (name.Digest, error) {
	tag, err := name.NewTag(ref)
	if err != nil {
		return name.Digest{}, err
	}
	digest, ok := r.digests[tag.Name()]
	if !ok {
		return name.Digest{}, fmt.Errorf("%s was not built from testdata", ref)
	}
	return tag.Context().Digest(digest), nil
}

// RenderDir copies srcDir to dstDir, replacing the placeholders of its files that refer to the
// images of the registry:
//   - ${ORIGIN_REGISTRY} is the host:port of the registry,
//   - ${DIGEST:<repository>:<tag>} is the digest of the image published as repository:tag.
func (r *Registry) RenderDir(srcDir, dstDir string) error {
	return RenderDir(srcDir, dstDir, r.resolve)
}

// ReleaseSignaturesDir returns the directory with the GPG signatures of the releases, named
// <tag>-sha256-<digest> like oc-mirror caches them in working-dir/signatures.
func (r *Registry) ReleaseSignaturesDir() string {
	return r.releaseSignaturesDir
}

// ReleasePublicKeyFile returns the file with the armored GPG public key that verifies the release
// signatures, which OCP_SIGNATURE_VERIFICATION_PK makes oc-mirror use.
func (r *Registry) ReleasePublicKeyFile() string {
	return r.releasePublicKeyFile
}

func (r *Registry) resolve(name, argument string) (string, error) {
	switch {
	case name == "ORIGIN_REGISTRY" && argument == "":
		return r.Endpoint(), nil
	case name == "DIGEST" && argument != "":
		ref, err := r.ByDigest(r.Endpoint() + "/" + argument)
		if err != nil {
			return "", fmt.Errorf("%s is not built from testdata, or after the files that refer to it", argument)
		}
		return ref.DigestStr(), nil
	}
	return "", errors.New("unknown placeholder")
}

// publish builds and pushes the images in this order, so that the sources of the bundles, the
// catalogs and the releases can refer to the images they depend on with placeholders.
func (r *Registry) publish(ctx context.Context, cfg Config) (err error) {
	if err := r.WaitReady(ctx, 30*time.Second); err != nil {
		return fmt.Errorf("origin registry not ready: %w", err)
	}
	p, err := fixtures.NewPublisher(fixtures.PublisherConfig{
		Registry:    r.Endpoint(),
		Certificate: r.Certificate(),
		CosignKey:   cfg.CosignKey,
		OPMImage:    cfg.OPMImage,
		WorkDir:     filepath.Join(cfg.WorkDir, "images"),
	})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, p.Close()) }()

	// Like before, operator images are Docker schema 2 images and release images OCI images.
	if err := r.publishImages(ctx, p, cfg, cfg.OperandsDir, cfg.operatorImage, fixtures.DockerManifest); err != nil {
		return err
	}
	if err := r.publishImages(ctx, p, cfg, cfg.ComponentsDir, cfg.releaseImage, fixtures.OCIManifest); err != nil {
		return err
	}
	bundlesDir, err := r.publishBundles(ctx, p, cfg)
	if err != nil {
		return err
	}
	if err := r.publishCatalogs(ctx, p, cfg, bundlesDir); err != nil {
		return err
	}
	return r.publishReleases(ctx, p, cfg)
}

// publishImages publishes the image fixtures of dir, named by imageName.
func (r *Registry) publishImages(ctx context.Context, p *fixtures.Publisher, cfg Config, dir string, imageName func(fixture string) (string, string), format fixtures.ManifestFormat) error {
	return forEachFixture(dir, func(fixture, path string) error {
		repo, tag := imageName(fixture)
		ref, err := p.PublishImage(ctx, path, repo, tag, cfg.platforms(fixture), format)
		if err != nil {
			return fmt.Errorf("failed to publish image %s: %w", fixture, err)
		}
		return r.record(ref, tag)
	})
}

// publishBundles renders the placeholders of the bundles and publishes them, and returns the
// directory of the rendered bundles.
func (r *Registry) publishBundles(ctx context.Context, p *fixtures.Publisher, cfg Config) (string, error) {
	if cfg.BundlesDir == "" {
		return "", nil
	}
	bundlesDir := filepath.Join(cfg.WorkDir, "bundles")
	return bundlesDir, forEachFixture(cfg.BundlesDir, func(bundle, path string) error {
		rendered := filepath.Join(bundlesDir, bundle)
		if err := r.RenderDir(path, rendered); err != nil {
			return err
		}
		repo, tag := cfg.operatorImage(bundle)
		ref, err := p.PublishBundle(ctx, rendered, repo, tag, cfg.platforms(bundle))
		if err != nil {
			return fmt.Errorf("failed to publish bundle %s: %w", bundle, err)
		}
		return r.record(ref, tag)
	})
}

// publishCatalogs renders the placeholders and the bundles of the catalogs, and publishes them.
func (r *Registry) publishCatalogs(ctx context.Context, p *fixtures.Publisher, cfg Config, bundlesDir string) error {
	if cfg.CatalogsDir == "" {
		return nil
	}
	bundleRepo := r.Endpoint() + "/" + cfg.OperatorRepo
	bundleDir := func(image string) (string, error) {
		ref, err := name.NewTag(image)
		if err != nil {
			return "", err
		}
		if bundlesDir == "" || ref.Context().Name() != bundleRepo {
			return "", fmt.Errorf("only the bundles of %s can be rendered", bundleRepo)
		}
		dir := filepath.Join(bundlesDir, ref.TagStr())
		if _, err := os.Stat(dir); err != nil {
			return "", fmt.Errorf("there is no bundle %s in %s", ref.TagStr(), cfg.BundlesDir)
		}
		return dir, nil
	}
	return forEachFixture(cfg.CatalogsDir, func(catalog, path string) error {
		rendered := filepath.Join(cfg.WorkDir, "catalogs", catalog)
		if err := r.RenderDir(path, rendered); err != nil {
			return err
		}
		if err := fixtures.RenderBundles(ctx, rendered, bundleDir); err != nil {
			return err
		}
		repo, tag := cfg.operatorImage(catalog)
		ref, err := p.PublishCatalog(ctx, rendered, repo, tag, cfg.platforms(catalog))
		if err != nil {
			return fmt.Errorf("failed to publish catalog %s: %w", catalog, err)
		}
		return r.record(ref, tag)
	})
}

// publishReleases renders the placeholders of the releases and publishes them. It also signs each
// release with a new GPG key, because oc-mirror only mirrors releases with a valid GPG signature,
// which it otherwise downloads from Red Hat.
func (r *Registry) publishReleases(ctx context.Context, p *fixtures.Publisher, cfg Config) error {
	if cfg.ReleasesDir == "" {
		return nil
	}
	signer, err := fixtures.NewReleaseSigner()
	if err != nil {
		return err
	}
	r.releaseSignaturesDir = filepath.Join(cfg.WorkDir, "release-signatures")
	if err := os.MkdirAll(r.releaseSignaturesDir, 0o755); err != nil {
		return err
	}
	err = forEachFixture(cfg.ReleasesDir, func(release, path string) error {
		rendered := filepath.Join(cfg.WorkDir, "releases", release)
		if err := r.RenderDir(path, rendered); err != nil {
			return err
		}
		repo, tag := cfg.releaseImage(release)
		ref, err := p.PublishRelease(ctx, rendered, repo, tag, cfg.platforms(release))
		if err != nil {
			return fmt.Errorf("failed to publish release %s: %w", release, err)
		}
		if err := r.record(ref, tag); err != nil {
			return err
		}

		digest, err := v1.NewHash(ref.DigestStr())
		if err != nil {
			return err
		}
		signature, err := signer.Sign(ref.Context().Tag(tag), digest)
		if err != nil {
			return fmt.Errorf("failed to sign release %s: %w", release, err)
		}
		signatureFile := fmt.Sprintf("%s-%s-%s", tag, digest.Algorithm, digest.Hex)
		return os.WriteFile(filepath.Join(r.releaseSignaturesDir, signatureFile), signature, 0o644)
	})
	if err != nil {
		return err
	}

	publicKey, err := signer.PublicKey()
	if err != nil {
		return err
	}
	r.releasePublicKeyFile = filepath.Join(cfg.WorkDir, "release-pk.asc")
	return os.WriteFile(r.releasePublicKeyFile, publicKey, 0o644)
}

// record saves the digest of the image published as ref with the given tag.
func (r *Registry) record(ref name.Digest, tag string) error {
	key := ref.Context().Tag(tag).Name()
	if _, ok := r.digests[key]; ok {
		return fmt.Errorf("%s is built from two sources in testdata", key)
	}
	r.digests[key] = ref.DigestStr()
	return nil
}

// operatorImage returns the repository and tag of the catalog, bundle or operand fixture.
func (cfg Config) operatorImage(fixture string) (string, string) {
	return cfg.OperatorRepo, fixture
}

// releaseImage returns the repository and tag of the release or release component fixture.
func (cfg Config) releaseImage(fixture string) (string, string) {
	return cfg.ReleaseRepo + "/" + fixture, cfg.ReleaseTag
}

func (cfg Config) platforms(fixture string) []v1.Platform {
	if platforms, ok := cfg.MultiArch[fixture]; ok {
		return platforms
	}
	return []v1.Platform{defaultPlatform}
}

// forEachFixture calls fn with the name and path of every subdirectory of dir, which must have
// at least one, unless dir is empty.
func forEachFixture(dir string, fn func(fixture, path string) error) error {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	found := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		found = true
		if err := fn(entry.Name(), filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("no fixtures found in %s", dir)
	}
	return nil
}
