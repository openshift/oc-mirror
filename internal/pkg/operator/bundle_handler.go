package operator

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/folder"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// bundleManifestsLabel is the image label declaring the directory that holds
// the bundle's CSV.
const bundleManifestsLabel = "operators.operatorframework.io.bundle.manifests.v1"

const defaultBundleManifestsDir = "manifests"

// BundleHandler tells a bundle's own controller images apart from the operands
// it deploys. Only the bundle can answer: olm.csv.metadata omits the install
// strategy, so the catalog alone cannot.
type BundleHandler struct {
	Log      clog.PluggableLoggerInterface
	Manifest manifest.ManifestInterface
	Mirror   mirror.MirrorInterface
}

// ControllerImages returns the images the CSV runs itself, from the containers
// and init containers of every deployment. Everything else in relatedImages is
// an operand. The extraction under bundleDir is reused if already present.
func (o BundleHandler) ControllerImages(ctx context.Context, bundleImage, bundleDir string, opts mirror.CopyOptions) (map[string]struct{}, error) {
	manifestsDir := filepath.Join(bundleDir, operatorBundleManifestsDir)

	csv, err := o.clusterServiceVersion(ctx, bundleImage, bundleDir, manifestsDir, opts)
	if err != nil {
		return nil, err
	}

	images := make(map[string]struct{})
	for _, dep := range csv.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		for _, c := range dep.Spec.Template.Spec.Containers {
			images[c.Image] = struct{}{}
		}
		for _, c := range dep.Spec.Template.Spec.InitContainers {
			images[c.Image] = struct{}{}
		}
	}

	return images, nil
}

func (o BundleHandler) clusterServiceVersion(ctx context.Context, bundleImage, bundleDir, manifestsDir string, opts mirror.CopyOptions) (*v1alpha1.ClusterServiceVersion, error) {
	imageDir := filepath.Join(bundleDir, operatorBundleImageDir)

	if err := o.ensureBundleInOCIFormat(ctx, bundleImage, imageDir, opts); err != nil {
		return nil, fmt.Errorf("copy bundle %s: %w", bundleImage, err)
	}

	img, err := o.Manifest.GetOCIImageFromIndex(imageDir)
	if err != nil {
		return nil, fmt.Errorf("read bundle image %s: %w", bundleImage, err)
	}

	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("read config of bundle %s: %w", bundleImage, err)
	}

	// A bundle without the label keeps the default; only a failed read above
	// is an error, since guessing the directory hides the real cause.
	label := defaultBundleManifestsDir
	if cfg != nil {
		if l := cfg.Config.Labels[bundleManifestsLabel]; l != "" {
			label = l
		}
	}

	subdir, err := bundleManifestsSubdir(label)
	if err != nil {
		return nil, fmt.Errorf("bundle %s: %w", bundleImage, err)
	}

	if err := o.Manifest.ExtractOCILayers(img, manifestsDir, subdir); err != nil {
		return nil, fmt.Errorf("extract manifests from bundle %s: %w", bundleImage, err)
	}

	csv, err := findClusterServiceVersion(filepath.Join(manifestsDir, subdir))
	if err != nil {
		return nil, fmt.Errorf("bundle %s: %w", bundleImage, err)
	}

	return csv, nil
}

// ensureBundleInOCIFormat copies the bundle into an OCI layout unless it is
// already there. Bundle references always come from a catalog, so they are
// always registry references.
func (o BundleHandler) ensureBundleInOCIFormat(ctx context.Context, bundleImage, imageDir string, opts mirror.CopyOptions) error {
	if _, err := os.Stat(filepath.Join(imageDir, "index.json")); err == nil {
		o.Log.Debug("bundle image already in OCI format at %q", imageDir)
		return nil
	}

	var gOpts mirror.GlobalOptions
	if opts.Global != nil {
		gOpts = *opts.Global
	}
	localOpts := opts
	localOpts.Global = &gOpts
	localOpts.Stdout = io.Discard
	localOpts.RemoveSignatures = true
	localOpts.Global.SecurePolicy = false
	// The CSV is identical across architectures, so never filter instances
	// here: doing so would fail for exactly the bundles we need to inspect.
	localOpts.InstancePlatforms = nil

	if err := folder.CreateFolders(imageDir); err != nil {
		return err
	}

	src := bundleImage
	if !strings.Contains(src, "://") {
		src = consts.DockerProtocol + src
	}

	return o.Mirror.Run(ctx, src, consts.OciProtocolTrimmed+imageDir, mirror.CopyMode, &localOpts)
}

// bundleManifestsSubdir validates the manifests directory a bundle declares.
// The label is untrusted: it must not escape the extraction directory.
func bundleManifestsSubdir(label string) (string, error) {
	trimmed := strings.Trim(label, "/")

	// Reject ".." before cleaning: Clean is lexical, so it would collapse
	// "a/../manifests" even when "a" is a symlink from the bundle.
	for _, part := range strings.Split(trimmed, "/") {
		if part == ".." {
			return "", fmt.Errorf("invalid manifests directory %q", label)
		}
	}

	clean := filepath.Clean(trimmed)
	if clean == "." || !filepath.IsLocal(clean) {
		return "", fmt.Errorf("invalid manifests directory %q", label)
	}
	return clean, nil
}

// isCSVFileName matches a CSV by filename. name must be lower-cased.
func isCSVFileName(name string) bool {
	if !strings.Contains(name, "clusterserviceversion") && !strings.Contains(name, "csv") {
		return false
	}
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".json")
}

func findClusterServiceVersion(dir string) (*v1alpha1.ClusterServiceVersion, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read manifests directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !isCSVFileName(strings.ToLower(entry.Name())) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}

		var csv v1alpha1.ClusterServiceVersion
		if err := yaml.Unmarshal(data, &csv); err != nil {
			return nil, fmt.Errorf("parse %s: %w", entry.Name(), err)
		}
		// Require the kind: the filename filter is loose, and an empty CSV
		// would make every related image look like an operand.
		if csv.Kind != "ClusterServiceVersion" {
			continue
		}
		return &csv, nil
	}

	return nil, fmt.Errorf("no ClusterServiceVersion found in %s", dir)
}
