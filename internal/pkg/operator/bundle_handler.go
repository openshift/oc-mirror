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

// bundleManifestsLabel is the image label a registry+v1 bundle uses to declare
// the directory holding its manifests, the same way a catalog declares its
// configs directory. It is the directory that contains the CSV.
const bundleManifestsLabel = "operators.operatorframework.io.bundle.manifests.v1"

// defaultBundleManifestsDir is used when a bundle image does not carry the
// manifests label.
const defaultBundleManifestsDir = "manifests"

// BundleHandler reads a registry+v1 operator bundle image in order to tell the
// bundle's own controller images apart from the operands it deploys.
//
// This has to come from the bundle: a catalog's olm.csv.metadata property
// carries the CSV's labels and descriptions but not its install strategy, so
// the file-based catalog alone cannot say which related images are controllers.
type BundleHandler struct {
	Log      clog.PluggableLoggerInterface
	Manifest manifest.ManifestInterface
	Mirror   mirror.MirrorInterface
}

// ControllerImages returns the set of images the bundle's ClusterServiceVersion
// runs itself, taken from the containers and init containers of every
// deployment in its install strategy. Every other entry in the bundle's
// relatedImages is an operand.
//
// bundleDir is where the bundle image is laid out on disk; the extraction is
// reused on later runs if it is already present.
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

// clusterServiceVersion pulls the bundle image into an OCI layout if needed,
// extracts its manifests directory and parses the CSV it contains.
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

	if err := o.Manifest.ExtractOCILayers(img, manifestsDir, label); err != nil {
		return nil, fmt.Errorf("extract manifests from bundle %s: %w", bundleImage, err)
	}

	csv, err := findClusterServiceVersion(filepath.Join(manifestsDir, strings.Trim(label, "/")))
	if err != nil {
		return nil, fmt.Errorf("bundle %s: %w", bundleImage, err)
	}

	return csv, nil
}

// ensureBundleInOCIFormat copies the bundle image into an OCI layout on disk,
// unless it is already there. It mirrors EnsureCatalogInOCIFormat, minus the
// oci:// source handling: bundle references always come from a catalog, so they
// are always registry references.
func (o BundleHandler) ensureBundleInOCIFormat(ctx context.Context, bundleImage, imageDir string, opts mirror.CopyOptions) error {
	if _, err := os.Stat(filepath.Join(imageDir, "index.json")); err == nil {
		o.Log.Debug("bundle image already in OCI format at %q", imageDir)
		return nil
	}

	// modify a copy, not the pointed value
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

// isCSVFileName reports whether a file in a bundle's manifests directory looks
// like its ClusterServiceVersion. name is expected to be lower-cased.
func isCSVFileName(name string) bool {
	if !strings.Contains(name, "clusterserviceversion") && !strings.Contains(name, "csv") {
		return false
	}
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".json")
}

// findClusterServiceVersion locates and parses the CSV inside an extracted
// bundle manifests directory.
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
		if csv.Kind != "" && csv.Kind != "ClusterServiceVersion" {
			continue
		}
		return &csv, nil
	}

	return nil, fmt.Errorf("no ClusterServiceVersion found in %s", dir)
}
