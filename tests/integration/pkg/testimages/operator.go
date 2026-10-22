package testimages

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/template/basic"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"
)

// operatorRepo is the repository holding every operator image: bundles, operands and catalogs.
const operatorRepo = "oc-mirror/oc-mirror-dev"

const (
	// operandTag is the operand that every test bundle references by digest.
	operandTag = "operand"
	// operandAltTag is a second operand with a different digest, used to reference
	// two digests through the same repository and tag (repo:tag@digest).
	operandAltTag = "operand-alt"
)

// basicTemplateFile is the name of the opm basic catalog templates in catalog
// directories. They are rendered with opm into a "catalog.yaml" FBC file.
const basicTemplateFile = "basic-template.yaml"

// multiArchBundles are also published as "<tag>-multi" image indexes for
// multiArchPlatforms, in addition to their single-arch "<tag>" image.
var multiArchBundles = []string{"foo-bundle-v0.3.1", "bar-bundle-v1.0.0"}

// multiArchCatalogs are published as image indexes for multiArchPlatforms instead of
// single-arch images.
var multiArchCatalogs = []string{"test-catalog-multi"}

var multiArchPlatforms = []v1.Platform{linuxAMD64, linuxARM64}

// buildOperatorImages builds and pushes, from the sources in dir:
//   - the shared operands referenced by the bundles and catalogs (operandTag, operandAltTag)
//   - one bundle image per bundles/<package>/<package>-bundle-v<version> directory,
//     tagged <package>-bundle-v<version>, and one operand per bundle, tagged <package>-v<version>
//   - one catalog image per catalogs/<name> directory, tagged <name>
//
// Images are built in this order because bundle manifests reference operands by
// digest, and catalogs are rendered by opm from the pushed bundle images.
func (b *builder) buildOperatorImages(ctx context.Context, dir string) error {
	for _, tag := range []string{operandTag, operandAltTag} {
		if err := b.buildOperand(tag); err != nil {
			return err
		}
	}

	bundleDirs, err := filepath.Glob(filepath.Join(dir, "bundles", "*", "*-bundle-v*"))
	if err != nil {
		return err
	}
	sort.Strings(bundleDirs)
	for _, bundleDir := range bundleDirs {
		if err := b.buildBundle(bundleDir); err != nil {
			return fmt.Errorf("bundle %s: %w", filepath.Base(bundleDir), err)
		}
	}

	catalogDirs, err := filepath.Glob(filepath.Join(dir, "catalogs", "*"))
	if err != nil {
		return err
	}
	sort.Strings(catalogDirs)
	for _, catalogDir := range catalogDirs {
		if info, err := os.Stat(catalogDir); err != nil || !info.IsDir() {
			continue
		}
		if err := b.buildCatalog(ctx, catalogDir); err != nil {
			return fmt.Errorf("catalog %s: %w", filepath.Base(catalogDir), err)
		}
	}
	return nil
}

// buildOperand pushes a minimal operand image whose only content identifies it,
// so that every operand has a distinct digest.
func (b *builder) buildOperand(tag string) error {
	img, err := buildImage(imageSpec{
		platform: linuxAMD64,
		layers:   [][]layerFile{{{path: "operand", content: []byte(operatorRepo + ":" + tag + "\n")}}},
	})
	if err != nil {
		return fmt.Errorf("operand %s: %w", tag, err)
	}
	return b.pushImage(operatorRepo+":"+tag, img)
}

// buildBundle pushes the registry+v1 bundle image built from bundleDir, its
// operand, and its multi-arch variant if listed in multiArchBundles.
// Bundle manifests are rendered as templates (see Inventory.render).
func (b *builder) buildBundle(bundleDir string) error {
	tag := filepath.Base(bundleDir)

	labels, err := bundleLabels(bundleDir)
	if err != nil {
		return err
	}
	files, err := filesFromDir(bundleDir, "", []string{"manifests", "metadata"}, b.inventory.render)
	if err != nil {
		return err
	}
	spec := imageSpec{
		platform: linuxAMD64,
		labels:   labels,
		layers:   [][]layerFile{files},
	}

	img, err := buildImage(spec)
	if err != nil {
		return err
	}
	if err := b.pushImage(operatorRepo+":"+tag, img); err != nil {
		return err
	}

	if slices.Contains(multiArchBundles, tag) {
		spec.mediaTypes = ociMediaTypes
		idx, err := buildMultiArchImage(spec, multiArchPlatforms)
		if err != nil {
			return err
		}
		if err := b.pushIndex(operatorRepo+":"+tag+"-multi", idx); err != nil {
			return err
		}
	}

	return b.buildOperand(strings.Replace(tag, "-bundle", "", 1))
}

// bundleLabels returns the registry+v1 bundle labels, including the annotations
// from metadata/annotations.yaml, as set in a bundle Dockerfile.
func bundleLabels(bundleDir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(bundleDir, "metadata", "annotations.yaml"))
	if err != nil {
		return nil, err
	}
	var annotations struct {
		Annotations map[string]string `json:"annotations"`
	}
	if err := yaml.Unmarshal(data, &annotations); err != nil {
		return nil, fmt.Errorf("parsing annotations.yaml: %w", err)
	}

	labels := map[string]string{
		"operators.operatorframework.io.bundle.mediatype.v1": "registry+v1",
		"operators.operatorframework.io.bundle.manifests.v1": "manifests/",
		"operators.operatorframework.io.bundle.metadata.v1":  "metadata/",
	}
	for k, v := range annotations.Annotations {
		labels[k] = v
	}
	return labels, nil
}

// buildCatalog pushes the file-based catalog defined in catalogDir as an image
// serving it from /configs:
//   - basic-template.yaml files are opm basic catalog templates, rendered with opm
//     (like `opm alpha render-template basic`) into catalog.yaml files
//   - any other file is FBC added as is, after rendering it as a template (see Inventory.render)
//
// The image has no opm binary nor serve cache: oc-mirror only reads the FBC.
func (b *builder) buildCatalog(ctx context.Context, catalogDir string) error {
	tag := filepath.Base(catalogDir)

	render := func(p string, content []byte) ([]byte, error) {
		if filepath.Base(p) == basicTemplateFile {
			return b.renderBasicTemplate(ctx, content)
		}
		return b.inventory.render(p, content)
	}
	files, err := filesFromDir(catalogDir, "configs", []string{"."}, render)
	if err != nil {
		return err
	}
	for i := range files {
		if path.Base(files[i].path) == basicTemplateFile {
			files[i].path = path.Join(path.Dir(files[i].path), "catalog.yaml")
		}
	}

	spec := imageSpec{
		platform: linuxAMD64,
		labels:   map[string]string{"operators.operatorframework.io.index.configs.v1": "/configs"},
		layers:   [][]layerFile{files},
	}

	if slices.Contains(multiArchCatalogs, tag) {
		spec.mediaTypes = ociMediaTypes
		idx, err := buildMultiArchImage(spec, multiArchPlatforms)
		if err != nil {
			return err
		}
		return b.pushIndex(operatorRepo+":"+tag, idx)
	}

	img, err := buildImage(spec)
	if err != nil {
		return err
	}
	return b.pushImage(operatorRepo+":"+tag, img)
}

// renderBasicTemplate renders an opm basic catalog template into FBC YAML, pulling
// the bundle images it references from the registry, as `opm alpha render-template basic` does.
func (b *builder) renderBasicTemplate(ctx context.Context, template []byte) ([]byte, error) {
	cacheDir, err := os.MkdirTemp("", "oc-mirror-opm-registry-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(cacheDir)

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	opts := []containerdregistry.RegistryOption{
		containerdregistry.WithCacheDir(cacheDir),
		containerdregistry.WithLog(logrus.NewEntry(logger)),
	}
	if rootCAs := b.reg.RootCAs(); rootCAs != nil {
		opts = append(opts, containerdregistry.WithRootCAs(rootCAs))
	} else {
		opts = append(opts, containerdregistry.WithPlainHTTP(true))
	}
	reg, err := containerdregistry.NewRegistry(opts...)
	if err != nil {
		return nil, fmt.Errorf("creating opm registry client: %w", err)
	}
	defer reg.Destroy()

	tmpl := basic.Template{
		RenderBundle: func(ctx context.Context, image string) (*declcfg.DeclarativeConfig, error) {
			return action.Render{
				Refs:           []string{image},
				Registry:       reg,
				AllowedRefMask: action.RefBundleImage,
			}.Run(ctx)
		},
	}
	cfg, err := tmpl.Render(ctx, bytes.NewReader(template))
	if err != nil {
		return nil, fmt.Errorf("rendering basic catalog template: %w", err)
	}

	var out bytes.Buffer
	if err := declcfg.WriteYAML(*cfg, &out); err != nil {
		return nil, fmt.Errorf("writing rendered catalog: %w", err)
	}
	return out.Bytes(), nil
}
