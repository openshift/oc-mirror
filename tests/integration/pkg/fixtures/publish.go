package fixtures

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/manifest"
	ocilayout "go.podman.io/image/v5/oci/layout"
	"go.podman.io/image/v5/signature"
	"go.podman.io/image/v5/signature/signer"
	"go.podman.io/image/v5/signature/sigstore"
	"go.podman.io/image/v5/types"
)

// ManifestFormat is the manifest format of an image built for a single platform. An image built
// for several platforms is always an OCI image index of OCI images, like `podman manifest push`.
type ManifestFormat int

const (
	// OCIManifest is the format `podman build` uses by default.
	OCIManifest ManifestFormat = iota
	// DockerManifest is the Docker schema 2 format of `podman build --format=docker`.
	DockerManifest
)

// PublisherConfig configures a Publisher.
type PublisherConfig struct {
	// Registry is the host[:port] of the registry the images are pushed to.
	Registry string
	// Certificate is the PEM encoded certificate authority of the registry.
	Certificate []byte
	// CosignKey is the cosign private key, without password, that signs every pushed manifest,
	// including the images of an index.
	CosignKey string
	// OPMImage is the base image of the catalogs.
	OPMImage string
	// WorkDir is where the images are written before they are pushed.
	WorkDir string
}

// Publisher builds images from their sources and pushes them, signed, to a registry. It pushes
// them the way `skopeo copy --all --sign-by-sigstore-private-key` does, from an OCI layout where
// they are written first: the signatures are stored as cosign stores them.
type Publisher struct {
	registry  string
	opmImage  string
	layoutDir string
	layout    layout.Path
	sysCtx    *types.SystemContext
	policy    *signature.PolicyContext
	signer    *signer.Signer
	bases     map[string]v1.Image
}

// NewPublisher returns a Publisher, which must be closed after use.
func NewPublisher(cfg PublisherConfig) (*Publisher, error) {
	certDir := filepath.Join(cfg.WorkDir, "certs")
	registriesDir := filepath.Join(cfg.WorkDir, "registries.d")
	for dir, files := range map[string]map[string][]byte{
		certDir:       {"ca.crt": cfg.Certificate},
		registriesDir: {"default.yaml": []byte("default-docker:\n  use-sigstore-attachments: true\n")},
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		for file, data := range files {
			if err := os.WriteFile(filepath.Join(dir, file), data, 0o644); err != nil {
				return nil, err
			}
		}
	}
	layoutDir := filepath.Join(cfg.WorkDir, "oci")
	path, err := layout.Write(layoutDir, empty.Index)
	if err != nil {
		return nil, err
	}

	// A nil passphrase means that it is not provided, rather than empty.
	cosignSigner, err := sigstore.NewSigner(sigstore.WithPrivateKeyFile(cfg.CosignKey, []byte{}))
	if err != nil {
		return nil, fmt.Errorf("failed to load the cosign key %s: %w", cfg.CosignKey, err)
	}
	policy, err := signature.NewPolicyContext(&signature.Policy{
		Default: signature.PolicyRequirements{signature.NewPRInsecureAcceptAnything()},
	})
	if err != nil {
		return nil, errors.Join(err, cosignSigner.Close())
	}
	return &Publisher{
		registry:  cfg.Registry,
		opmImage:  cfg.OPMImage,
		layoutDir: layoutDir,
		layout:    path,
		sysCtx:    &types.SystemContext{DockerCertPath: certDir, RegistriesDirPath: registriesDir},
		policy:    policy,
		signer:    cosignSigner,
	}, nil
}

// Close releases the resources of the Publisher.
func (p *Publisher) Close() error {
	return errors.Join(p.policy.Destroy(), p.signer.Close())
}

// PublishCatalog builds the catalog of fbcDir for each platform and pushes it as repo:tag.
// Like the catalog images published before, a single platform produces a Docker schema 2 image.
func (p *Publisher) PublishCatalog(ctx context.Context, fbcDir, repo, tag string, platforms []v1.Platform) (name.Digest, error) {
	return p.publish(ctx, repo, tag, platforms, DockerManifest, func(platform v1.Platform) (v1.Image, error) {
		base, err := p.base(ctx, p.opmImage, platform)
		if err != nil {
			return nil, err
		}
		return Catalog(ctx, base, fbcDir)
	})
}

// PublishBundle builds the operator bundle of bundleDir for each platform and pushes it as
// repo:tag. Like the bundle images published before, a single platform produces a Docker
// schema 2 image.
func (p *Publisher) PublishBundle(ctx context.Context, bundleDir, repo, tag string, platforms []v1.Platform) (name.Digest, error) {
	return p.publish(ctx, repo, tag, platforms, DockerManifest, func(platform v1.Platform) (v1.Image, error) {
		return Bundle(bundleDir, platform)
	})
}

// PublishImage builds the image fixture of dir, described by its image.yaml file, for each
// platform and pushes it as repo:tag, in the given format for a single platform.
func (p *Publisher) PublishImage(ctx context.Context, dir, repo, tag string, platforms []v1.Platform, format ManifestFormat) (name.Digest, error) {
	spec, err := LoadImageSpec(dir)
	if err != nil {
		return name.Digest{}, err
	}
	return p.publish(ctx, repo, tag, platforms, format, func(platform v1.Platform) (v1.Image, error) {
		var base v1.Image
		if spec.Base != "" {
			if base, err = p.base(ctx, spec.Base, platform); err != nil {
				return nil, err
			}
		}
		return Image(base, dir, spec, platform)
	})
}

// PublishRelease builds the release payload of releaseDir for each platform and pushes it as
// repo:tag: an OCI image for a single platform, or an OCI image index for several platforms.
func (p *Publisher) PublishRelease(ctx context.Context, releaseDir, repo, tag string, platforms []v1.Platform) (name.Digest, error) {
	return p.publish(ctx, repo, tag, platforms, OCIManifest, func(platform v1.Platform) (v1.Image, error) {
		return Release(releaseDir, platform)
	})
}

func (p *Publisher) publish(ctx context.Context, repo, tag string, platforms []v1.Platform, format ManifestFormat, build func(v1.Platform) (v1.Image, error)) (name.Digest, error) {
	ref, err := name.NewTag(p.registry + "/" + repo + ":" + tag)
	if err != nil {
		return name.Digest{}, err
	}
	if len(platforms) == 0 {
		return name.Digest{}, fmt.Errorf("no platforms to build %s for", ref)
	}
	images := make([]v1.Image, 0, len(platforms))
	for _, platform := range platforms {
		img, err := build(platform)
		if err != nil {
			return name.Digest{}, fmt.Errorf("failed to build %s for %s: %w", ref, platform, err)
		}
		images = append(images, img)
	}

	// The image is named repo:tag in the layout, which only has the images built by p.
	refName := repo + ":" + tag
	annotations := layout.WithAnnotations(map[string]string{imgspecv1.AnnotationRefName: refName})
	if len(images) == 1 {
		err = p.layout.AppendImage(images[0], annotations)
	} else {
		format = OCIManifest
		var index v1.ImageIndex
		if index, err = Index(platforms, images); err == nil {
			err = p.layout.AppendIndex(index, annotations)
		}
	}
	if err != nil {
		return name.Digest{}, fmt.Errorf("failed to write %s: %w", ref, err)
	}

	src, err := ocilayout.NewReference(p.layoutDir, refName)
	if err != nil {
		return name.Digest{}, err
	}
	dest, err := docker.ParseReference("//" + ref.Name())
	if err != nil {
		return name.Digest{}, err
	}
	manifestType := imgspecv1.MediaTypeImageManifest
	if format == DockerManifest {
		manifestType = manifest.DockerV2Schema2MediaType
	}
	copied, err := copy.Image(ctx, p.policy, dest, src, &copy.Options{
		DestinationCtx:        p.sysCtx,
		ImageListSelection:    copy.CopyAllImages,
		ForceManifestMIMEType: manifestType,
		Signers:               []*signer.Signer{p.signer},
	})
	if err != nil {
		return name.Digest{}, fmt.Errorf("failed to push %s: %w", ref, err)
	}
	digest, err := manifest.Digest(copied)
	if err != nil {
		return name.Digest{}, err
	}
	return ref.Context().Digest(digest.String()), nil
}

// base returns the image of ref for platform, which is pulled lazily and only once.
func (p *Publisher) base(ctx context.Context, ref string, platform v1.Platform) (v1.Image, error) {
	key := ref + " " + platform.String()
	if base, ok := p.bases[key]; ok {
		return base, nil
	}
	parsed, err := name.ParseReference(ref)
	if err != nil {
		return nil, err
	}
	base, err := remote.Image(parsed, remote.WithContext(ctx), remote.WithPlatform(platform))
	if err != nil {
		return nil, fmt.Errorf("failed to get %s for %s: %w", ref, platform, err)
	}
	cf, err := base.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to read config of %s for %s: %w", ref, platform, err)
	}
	actual := cf.Platform()
	if actual == nil {
		return nil, fmt.Errorf("%s has no platform in its config, expected %s", ref, platform)
	}
	if !actual.Satisfies(platform) {
		return nil, fmt.Errorf("%s has platform %s, expected %s", ref, actual, platform)
	}
	if p.bases == nil {
		p.bases = map[string]v1.Image{}
	}
	p.bases[key] = base
	return base, nil
}
