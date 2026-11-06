package integration_test

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/google/go-containerregistry/pkg/v1/validate"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/pkg/lib/config"

	"github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

const (
	// catalogConfigsLabel is the label oc-mirror reads to find the declarative
	// config inside a catalog image.
	catalogConfigsLabel = "operators.operatorframework.io.index.configs.v1"
	// catalogConfigsPath is where the fixture places the declarative config.
	catalogConfigsPath = "/configs"
)

// catalogFixture is a catalog image built during the test from checked-in
// declarative config and served by a disposable source registry.
//
// The image only carries the declarative config and the configs label: it has
// no opm binary or serve cache. It exercises oc-mirror's catalog extraction,
// filtering, rebuild and mirroring, but it cannot be served as a
// CatalogSource.
type catalogFixture struct {
	// Ref is the tagged reference to use in an ImageSetConfiguration.
	Ref string
	// DigestRef pins Ref to the digest of the pushed image.
	DigestRef string
	// Repo is the repository path of Ref, without the registry host.
	Repo string
	// Tag is the tag of Ref.
	Tag string
}

// seedCatalogFixture validates the declarative config in dir the same way
// `opm validate` does, builds a data-only catalog image from it and pushes it
// as repo:tag to a new source registry. The source registry is separate from
// testRegistry so the mirroring destination stays free of fixture content,
// and it is stopped when the current spec ends.
func seedCatalogFixture(dir, repo, tag string) catalogFixture {
	GinkgoHelper()

	Expect(config.Validate(ctx, os.DirFS(dir))).To(Succeed(), "catalog fixture %s is not a valid declarative config", dir)
	img := buildCatalogImage(dir)

	source, err := registry.Start(ctx, registryConfig, 0)
	Expect(err).NotTo(HaveOccurred(), "failed to start the catalog source registry")
	DeferCleanup(func() {
		if err := source.Stop(); err != nil {
			GinkgoWriter.Printf("Failed to stop catalog source registry: %v\n", err)
		}
	})
	Expect(source.WaitReady(ctx, 30*time.Second)).To(Succeed(), "catalog source registry not ready")

	ref, err := name.NewTag(fmt.Sprintf("%s/%s:%s", source.Endpoint(), repo, tag), name.Insecure)
	Expect(err).NotTo(HaveOccurred())
	Expect(remote.Write(ref, img, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))).To(Succeed(),
		"failed to push catalog fixture to %s", ref)

	digest, err := img.Digest()
	Expect(err).NotTo(HaveOccurred())
	fixture := catalogFixture{
		Ref:       ref.String(),
		DigestRef: fmt.Sprintf("%s/%s@%s", source.Endpoint(), repo, digest),
		Repo:      repo,
		Tag:       tag,
	}
	expectCatalogFixtureServed(fixture, dir)
	return fixture
}

// buildCatalogImage returns a single-layer OCI image holding the declarative
// config in dir under catalogConfigsPath, labelled for oc-mirror.
func buildCatalogImage(dir string) v1.Image { //nolint:ireturn // go-containerregistry images are interfaces
	GinkgoHelper()

	layerData := catalogConfigsTar(dir)
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(layerData)), nil
	}, tarball.WithMediaType(types.OCILayer))
	Expect(err).NotTo(HaveOccurred())

	img, err := mutate.AppendLayers(mutate.MediaType(empty.Image, types.OCIManifestSchema1), layer)
	Expect(err).NotTo(HaveOccurred())
	img = mutate.ConfigMediaType(img, types.OCIConfigJSON)

	cfg, err := img.ConfigFile()
	Expect(err).NotTo(HaveOccurred())
	cfg = cfg.DeepCopy()
	cfg.OS = "linux"
	cfg.Architecture = "amd64"
	cfg.Config.Labels = map[string]string{catalogConfigsLabel: catalogConfigsPath}
	img, err = mutate.ConfigFile(img, cfg)
	Expect(err).NotTo(HaveOccurred())
	return img
}

// catalogConfigsTar archives the directories and regular files in dir below
// catalogConfigsPath.
func catalogConfigsTar(dir string) []byte {
	GinkgoHelper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	root := strings.TrimPrefix(catalogConfigsPath, "/")
	err := filepath.WalkDir(dir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return fmt.Errorf("resolve %s relative to %s: %w", p, dir, err)
		}
		hdr := &tar.Header{Name: path.Join(root, filepath.ToSlash(rel)), Format: tar.FormatPAX}
		var data []byte
		switch {
		case entry.IsDir():
			hdr.Typeflag = tar.TypeDir
			hdr.Name += "/"
			hdr.Mode = 0o755
		case entry.Type().IsRegular():
			if data, err = os.ReadFile(p); err != nil {
				return fmt.Errorf("read %s: %w", p, err)
			}
			hdr.Typeflag = tar.TypeReg
			hdr.Mode = 0o644
			hdr.Size = int64(len(data))
		default:
			return fmt.Errorf("unsupported file type %s for %s", entry.Type(), p)
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write tar header for %s: %w", p, err)
		}
		if _, err := tw.Write(data); err != nil {
			return fmt.Errorf("write %s to tar: %w", p, err)
		}
		return nil
	})
	Expect(err).NotTo(HaveOccurred(), "failed to archive catalog fixture %s", dir)
	Expect(tw.Close()).To(Succeed())
	return buf.Bytes()
}

// expectCatalogFixtureServed reads the pushed fixture back from its source
// registry, validates its manifest, config and layer, and verifies it carries
// the same bundles as the checked-in declarative config.
func expectCatalogFixtureServed(fixture catalogFixture, dir string) {
	GinkgoHelper()

	ref, err := name.NewDigest(fixture.DigestRef, name.Insecure)
	Expect(err).NotTo(HaveOccurred())
	img, err := remote.Image(ref, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))
	Expect(err).NotTo(HaveOccurred(), "failed to read catalog fixture %s", ref)
	Expect(validate.Image(img)).To(Succeed(), "catalog fixture %s is incomplete or corrupt", ref)

	served := extractCatalogImageConfigs(img)
	DeferCleanup(os.RemoveAll, served)

	Expect(declarativeConfigBundles(served)).To(Equal(declarativeConfigBundles(dir)),
		"catalog fixture %s does not carry the declarative config from %s", ref, dir)
}

// declarativeConfigBundles returns each bundle's image and related images,
// keyed by bundle name.
func declarativeConfigBundles(dir string) map[string][]string {
	GinkgoHelper()

	cfg, err := declcfg.LoadFS(ctx, os.DirFS(dir))
	Expect(err).NotTo(HaveOccurred(), "failed to load declarative config from %s", dir)
	bundles := map[string][]string{}
	for _, b := range cfg.Bundles {
		images := make([]string, 0, 1+len(b.RelatedImages))
		images = append(images, b.Image)
		for _, ri := range b.RelatedImages {
			images = append(images, ri.Image)
		}
		bundles[b.Name] = images
	}
	return bundles
}

// writeCatalogFixtureISC writes an ImageSetConfiguration that mirrors the given
// packages from catalogRef and returns its path.
func writeCatalogFixtureISC(catalogRef string, packages ...string) string {
	GinkgoHelper()

	isc := fmt.Sprintf(`kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v2alpha1
mirror:
  operators:
  - catalog: %s
    packages:
`, catalogRef)
	for _, pkg := range packages {
		isc += fmt.Sprintf("    - name: %s\n", pkg)
	}

	p := filepath.Join(GinkgoT().TempDir(), "isc.yaml")
	Expect(os.WriteFile(p, []byte(isc), 0o600)).To(Succeed())
	return p
}
