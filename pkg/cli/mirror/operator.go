package mirror

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/remotes"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/joelanford/ignore"
	imgreference "github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/admin/catalog"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/operator"
)

// OperatorOptions configures either a Full or Diff mirror operation
// on a particular operator catalog image.
type OperatorOptions struct {
	*MirrorOptions

	SkipImagePin bool
	Logger       *logrus.Entry

	tmp      string
	insecure bool
}

func NewOperatorOptions(mo *MirrorOptions) *OperatorOptions {
	opts := &OperatorOptions{MirrorOptions: mo}
	if mo.SourcePlainHTTP || mo.SourceSkipTLS {
		opts.insecure = true
	}
	return opts
}

// PlanFull plans a mirror for each catalog image in its entirety
func (o *OperatorOptions) PlanFull(ctx context.Context, cfg v1alpha2.ImageSetConfiguration) (image.TypedImageMapping, error) {
	return o.run(ctx, cfg, o.renderDCFull)
}

// PlanDiff plans only the diff between each old and new catalog image pair
func (o *OperatorOptions) PlanDiff(ctx context.Context, cfg v1alpha2.ImageSetConfiguration, lastRun v1alpha2.PastMirror) (image.TypedImageMapping, error) {
	f := func(ctx context.Context, reg *containerdregistry.Registry, ctlg v1alpha2.Operator) (*declcfg.DeclarativeConfig, error) {
		return o.renderDCDiff(ctx, reg, ctlg, lastRun)
	}
	return o.run(ctx, cfg, f)
}

// complete defaults OperatorOptions.
func (o *OperatorOptions) complete() {
	if o.Dir == "" {
		o.Dir = "create"
	}

	if o.Logger == nil {
		o.Logger = logrus.NewEntry(logrus.New())
	}
}

type renderDCFunc func(context.Context, *containerdregistry.Registry, v1alpha2.Operator) (*declcfg.DeclarativeConfig, error)

func (o *OperatorOptions) run(ctx context.Context, cfg v1alpha2.ImageSetConfiguration, renderDC renderDCFunc) (image.TypedImageMapping, error) {
	o.complete()

	cleanup, err := o.mktempDir()
	if err != nil {
		return nil, err
	}
	if !o.SkipCleanup {
		defer cleanup()
	}

	reg, err := o.createRegistry()
	if err != nil {
		return nil, fmt.Errorf("error creating container registry: %v", err)
	}
	defer reg.Destroy()

	mmapping := image.TypedImageMapping{}
	for _, ctlg := range cfg.Mirror.Operators {

		ctlgRef, err := imagesource.ParseReference(ctlg.Catalog)
		if err != nil {
			return nil, fmt.Errorf("error parsing catalog: %v", err)
		}

		// Render the catalog to mirror into a declarative config.
		dc, err := renderDC(ctx, reg, ctlg)
		if err != nil {
			return nil, err
		}

		mappings, err := o.plan(ctx, dc, ctlgRef)
		if err != nil {
			return nil, err
		}
		mmapping.Merge(mappings)
	}

	return mmapping, nil
}

func (o *OperatorOptions) mktempDir() (func(), error) {
	o.tmp = filepath.Join(o.Dir, fmt.Sprintf("operators.%d", time.Now().Unix()))
	return func() {
		if err := os.RemoveAll(o.tmp); err != nil {
			o.Logger.Error(err)
		}
	}, os.MkdirAll(o.tmp, os.ModePerm)
}

func (o *OperatorOptions) createRegistry() (*containerdregistry.Registry, error) {
	cacheDir, err := os.MkdirTemp("", "imageset-catalog-registry-")
	if err != nil {
		return nil, err
	}

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)
	nullLogger := logrus.NewEntry(logger)

	return containerdregistry.NewRegistry(
		containerdregistry.WithCacheDir(cacheDir),
		containerdregistry.SkipTLSVerify(o.SourceSkipTLS),
		containerdregistry.WithPlainHTTP(o.SourcePlainHTTP),
		// The containerd registry impl is somewhat verbose, even on the happy path,
		// so discard all logger logs. Any important failures will be returned from
		// registry methods and eventually logged as fatal errors.
		containerdregistry.WithLog(nullLogger),
	)
}

// renderDCFull renders data in ctlg into a declarative config for o.Full().
func (o *OperatorOptions) renderDCFull(ctx context.Context, reg *containerdregistry.Registry, ctlg v1alpha2.Operator) (dc *declcfg.DeclarativeConfig, err error) {

	hasInclude := len(ctlg.IncludeConfig.Packages) != 0
	// Only add on top of channel heads if both HeadsOnly and IncludeConfig are specified.
	includeAdditively := ctlg.IsHeadsOnly() && hasInclude
	// Render the full catalog if neither HeadsOnly or IncludeConfig are specified (the default).
	full := !ctlg.IsHeadsOnly() && !hasInclude

	catLogger := o.Logger.WithField("catalog", ctlg.Catalog)
	if full {
		// Mirror the entire catalog.
		dc, err = action.Render{
			Registry: reg,
			Refs:     []string{ctlg.Catalog},
		}.Run(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		// Generate and mirror a heads-only diff using only the catalog as a new ref.
		dic, derr := ctlg.IncludeConfig.ConvertToDiffIncludeConfig()
		if derr != nil {
			return nil, derr
		}
		dc, err = action.Diff{
			Registry:          reg,
			NewRefs:           []string{ctlg.Catalog},
			Logger:            catLogger,
			IncludeConfig:     dic,
			IncludeAdditively: includeAdditively,
			SkipDependencies:  ctlg.SkipDependencies,
		}.Run(ctx)
		if err != nil {
			return nil, err
		}

		verifyOperatorPkgFound(dic, dc)
	}

	return dc, nil
}

// renderDCDiff renders data in ctlg into a declarative config for o.PlanDiff().
// This produces the declarative config that will be used to determine
// differential images
func (o *OperatorOptions) renderDCDiff(ctx context.Context, reg *containerdregistry.Registry, ctlg v1alpha2.Operator, lastRun v1alpha2.PastMirror) (dc *declcfg.DeclarativeConfig, err error) {
	prevCatalog := make(map[string]v1alpha2.OperatorMetadata, len(lastRun.Operators))
	for _, ctlg := range lastRun.Operators {
		prevCatalog[ctlg.Catalog] = ctlg
	}

	hasInclude := len(ctlg.IncludeConfig.Packages) != 0
	// Render the full catalog if neither HeadsOnly or IncludeConfig are specified.
	full := !ctlg.IsHeadsOnly() && !hasInclude

	// Generate and mirror a heads-only diff using the catalog as a new ref,
	// and an old ref found for this catalog in lastRun.
	catLogger := o.Logger.WithField("catalog", ctlg.Catalog)
	a := action.Diff{
		Registry: reg,
		NewRefs:  []string{ctlg.Catalog},
		Logger:   catLogger,
		// This is hard-coded to false because a diff post-metadata creation must always include
		// newly published catalog data to join graphs. Any included objects previously included
		// will be added as a diff as part of the latest diff mode.
		IncludeAdditively: false,
		SkipDependencies:  ctlg.SkipDependencies,
	}

	// Instead of creating a partial FBC with diff
	// generate the current FBC according to the previous
	// include config or just render the full catalog again
	dic, err := ctlg.IncludeConfig.ConvertToDiffIncludeConfig()
	if err != nil {
		return nil, err
	}
	switch {
	case full:
		// Mirror the entire catalog.
		dc, err = action.Render{
			Registry: reg,
			Refs:     []string{ctlg.Catalog},
		}.Run(ctx)
		if err != nil {
			return nil, err
		}
	case !hasInclude:
		// If a previous specified starting version can be found on
		// the mirror run update IncludeConfig , else, keep the
		// IncludeConfig to get a new catalog at heads only.
		prev, found := prevCatalog[ctlg.Catalog]
		if found {
			dc, err = action.Render{
				Registry: reg,
				Refs:     []string{ctlg.Catalog},
			}.Run(ctx)
			if err != nil {
				return nil, err
			}
			ic, err := operator.UpdateIncludeConfig(*dc, prev.IncludeConfig)
			if err != nil {
				return nil, err
			}
			dic, err = ic.ConvertToDiffIncludeConfig()
			if err != nil {
				return nil, err
			}
		}
		fallthrough
	default:
		a.IncludeConfig = dic
		dc, err = a.Run(ctx)
		if err != nil {
			return nil, err
		}
	}

	verifyOperatorPkgFound(dic, dc)

	return dc, nil
}

// verifyOperatorPkgFound will verify that each of the requested operator packages were
// found and added to the DeclarativeConfig.
func verifyOperatorPkgFound(dic action.DiffIncludeConfig, dc *declcfg.DeclarativeConfig) {
	logrus.Debug("DiffIncludeConfig: ", dic)
	logrus.Debug("DeclarativeConfig: ", dc)

	dcMap := make(map[string]bool)

	// Load the declarative config packages into a map
	for _, dcpkg := range dc.Packages {
		dcMap[dcpkg.Name] = true
	}

	for _, pkg := range dic.Packages {
		logrus.Debug("Checking for package: ", pkg)

		if !dcMap[pkg.Name] {
			// The operator package wasn't found. Log the error and continue on.
			logrus.Errorf("Operator %s was not found, please check name, startingVersion, and channels in the config file.", pkg.Name)
		}
	}
}

func (o *OperatorOptions) plan(ctx context.Context, dc *declcfg.DeclarativeConfig, ctlgRef imagesource.TypedImageReference) (image.TypedImageMapping, error) {

	o.Logger.Debugf("Mirroring catalog %q bundle and related images", ctlgRef.Ref.Exact())

	opts, err := o.newMirrorCatalogOptions(ctlgRef.Ref, filepath.Join(o.Dir, config.SourceDir))
	if err != nil {
		return nil, err
	}

	if !o.SkipImagePin {
		resolver, err := containerdregistry.NewResolver("", o.SourceSkipTLS, o.SourcePlainHTTP, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating image resolver: %v", err)
		}
		if err := o.pinImages(ctx, dc, resolver); err != nil {
			return nil, fmt.Errorf("error pinning images in catalog %s: %v", ctlgRef, err)
		}
	}

	indexDir, err := o.writeDC(dc, ctlgRef.Ref)
	if err != nil {
		return nil, err
	}

	// Create the mapping file, but don't mirror quite yet.
	// Since the file-based catalog (declarative config) needs to be rebuilt
	// after rendering with the existing image in the publishing step,
	// we can just build the new image once then.
	opts.ManifestOnly = true
	opts.ImageMirrorer = catalog.ImageMirrorerFunc(func(mapping map[imagesource.TypedImageReference]imagesource.TypedImageReference) error {
		return nil
	})

	opts.IndexExtractor = catalog.IndexExtractorFunc(func(imagesource.TypedImageReference) (string, error) {
		o.Logger.Debugf("returning index dir in extractor: %s", indexDir)
		return indexDir, nil
	})
	opts.IndexPath = indexDir

	opts.RelatedImagesParser = catalog.RelatedImagesParserFunc(parseRelatedImages)

	opts.SourceRef = ctlgRef
	opts.DestRef = imagesource.TypedImageReference{
		Type: imagesource.DestinationFile,
	}

	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid catalog mirror options: %v", err)
	}

	if err := opts.Run(); err != nil {
		return nil, fmt.Errorf("error running catalog mirror: %v", err)
	}

	mappingFile := filepath.Join(opts.ManifestDir, mappingFile)
	mappings, err := image.ReadImageMapping(mappingFile, "=", v1alpha2.TypeOperatorBundle)
	if err != nil {
		return nil, err
	}

	// Remove the catalog image from mappings we are going to transfer this
	// using an OCI layout.
	ctlgImg, err := image.ParseTypedImage(ctlgRef.Ref.Exact(), v1alpha2.TypeOperatorBundle)
	if err != nil {
		return nil, err
	}
	mappings.Remove(ctlgImg)
	if err := o.writeLayout(ctx, ctlgRef.Ref); err != nil {
		return nil, err
	}

	// Remove catalog namespace prefix from each mapping's destination, which is added by opts.Run().
	for srcRef, dstRef := range mappings {
		newRepoName := strings.TrimPrefix(dstRef.Ref.RepositoryName(), ctlgRef.Ref.RepositoryName())
		newRepoName = strings.TrimPrefix(newRepoName, "/")
		tmpRef, err := imgreference.Parse(newRepoName)
		if err != nil {
			return nil, err
		}
		dstRef.Ref.Namespace = tmpRef.Namespace
		dstRef.Ref.Name = tmpRef.Name
		mappings[srcRef] = dstRef
	}

	return mappings, validateMapping(*dc, mappings)
}

// validateMapping will search for bundle and related images in mapping
// and log a warning if an image does not exist and will not be mirrored
func validateMapping(dc declcfg.DeclarativeConfig, mapping image.TypedImageMapping) error {
	var errs []error
	validateFunc := func(img string) error {
		ref, err := image.ParseTypedImage(img, v1alpha2.TypeOperatorBundle)
		if err != nil {
			return err
		}
		_, ok := mapping[ref]
		if !ok {
			logrus.Warnf("image %s is not included in mapping", img)
		}
		return nil
	}
	for _, b := range dc.Bundles {
		if err := validateFunc(b.Image); err != nil {
			errs = append(errs, err)
			continue
		}
		for _, relatedImg := range b.RelatedImages {
			if err := validateFunc(relatedImg.Image); err != nil {
				errs = append(errs, err)
				continue
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}

// pinImages resolves every image in dc to it's canonical name (includes digest).
func (o *OperatorOptions) pinImages(ctx context.Context, dc *declcfg.DeclarativeConfig, resolver remotes.Resolver) (err error) {

	// Check that declarative config is not nil
	// to avoid panics
	if err := validate(dc); err != nil {
		return err
	}
	// Instead of returning an error, just log it.
	isSkipErr := func(err error) bool {
		return o.ContinueOnError || (o.SkipMissing && errors.Is(err, errdefs.ErrNotFound))
	}

	var errs []error
	for i, b := range dc.Bundles {

		if !image.IsImagePinned(b.Image) {
			logrus.Warnf("bundle %s: pinning bundle image %s to digest", b.Name, b.Image)

			if !image.IsImageTagged(b.Image) {
				logrus.Warnf("bundle %s: bundle image tag not set", b.Name)
				continue
			}
			if dc.Bundles[i].Image, err = image.ResolveToPin(ctx, resolver, b.Image); err != nil {
				if isSkipErr(err) {
					logrus.Warnf("skipping bundle %s image %s resolve error: %v", b.Name, b.Image, err)
				} else {
					errs = append(errs, err)
				}
			}
		}
		for j, ri := range b.RelatedImages {
			if !image.IsImagePinned(ri.Image) {
				logrus.Warnf("bundle %s: pinning related image %s to digest", ri.Name, ri.Image)

				if !image.IsImageTagged(ri.Image) {
					logrus.Warnf("bundle %s: related image tag not set", b.Name)
					continue
				}

				if b.RelatedImages[j].Image, err = image.ResolveToPin(ctx, resolver, ri.Image); err != nil {
					if isSkipErr(err) {
						logrus.Warnf("skipping bundle %s related image %s=%s resolve error: %v", b.Name, ri.Name, ri.Image, err)
					} else {
						errs = append(errs, err)
					}
				}
			}
		}
	}

	return utilerrors.NewAggregate(errs)
}

func (o *OperatorOptions) writeLayout(ctx context.Context, ctlgRef imgreference.DockerImageReference) error {

	// Write catalog OCI layout file to src so it is included in the archive
	// at a path unique to the image.
	ctlgDir, err := operator.GenerateCatalogDir(ctlgRef)
	if err != nil {
		return err
	}
	layoutDir := filepath.Join(o.Dir, config.SourceDir, config.CatalogsDir, ctlgDir, config.LayoutsDir)
	if err := os.MkdirAll(layoutDir, os.ModePerm); err != nil {
		return fmt.Errorf("error catalog layout dir: %v", err)
	}

	o.Logger.Debugf("writing catalog %q layout to %s", ctlgRef.Exact(), layoutDir)

	ref, err := name.ParseReference(ctlgRef.Exact(), getNameOpts(o.insecure)...)
	if err != nil {
		return err
	}
	desc, err := remote.Get(ref, getRemoteOpts(ctx, o.insecure)...)
	if err != nil {
		return err
	}

	if desc.MediaType.IsImage() {
		layoutPath, err := layout.Write(layoutDir, empty.Index)
		if err != nil {
			return fmt.Errorf("error creating OCI layout: %v", err)
		}
		img, err := desc.Image()
		if err != nil {
			return err
		}
		// Default to amd64 architecture with no multi-arch image
		if err := layoutPath.AppendImage(img, layout.WithPlatform(v1.Platform{OS: "linux", Architecture: "amd64"})); err != nil {
			return err
		}

	} else {
		idx, err := desc.ImageIndex()
		if err != nil {
			return err
		}
		if _, err = layout.Write(layoutDir, idx); err != nil {
			return fmt.Errorf("error creating OCI layout: %v", err)
		}
	}

	return nil
}

func (o *OperatorOptions) writeDC(dc *declcfg.DeclarativeConfig, ctlgRef imgreference.DockerImageReference) (string, error) {

	// Write catalog declarative config file to src so it is included in the archive
	// at a path unique to the image.
	ctlgDir, err := operator.GenerateCatalogDir(ctlgRef)
	if err != nil {
		return "", err
	}
	indexDir := filepath.Join(o.Dir, config.SourceDir, config.CatalogsDir, ctlgDir, config.IndexDir)
	if err := os.MkdirAll(indexDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("error creating diff index dir: %v", err)
	}
	catalogIndexPath := filepath.Join(indexDir, "index.json")

	o.Logger.Debugf("writing catalog %q diff to %s", ctlgRef.Exact(), catalogIndexPath)

	f, err := os.Create(catalogIndexPath)
	if err != nil {
		return "", fmt.Errorf("error creating diff index file: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			o.Logger.Error(err)
		}
	}()
	if err := declcfg.WriteJSON(*dc, f); err != nil {
		return "", fmt.Errorf("error writing diff catalog: %v", err)
	}

	return indexDir, nil
}

func (o *OperatorOptions) newMirrorCatalogOptions(ctlgRef imgreference.DockerImageReference, fileDir string) (*catalog.MirrorCatalogOptions, error) {

	opts := catalog.NewMirrorCatalogOptions(o.IOStreams)
	opts.DryRun = o.DryRun
	opts.FileDir = fileDir
	opts.MaxPathComponents = 2

	// Create the manifests dir in tmp so it gets cleaned up if desired.
	// TODO(estroz): if the CLI is meant to be re-entrant, check if this file exists
	// and use it directly if so.
	opts.ManifestDir = filepath.Join(o.tmp, fmt.Sprintf("manifests-%s", ctlgRef.Name))
	if err := os.MkdirAll(opts.ManifestDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("error creating manifests dir: %v", err)
	}
	o.Logger.Debugf("running mirrorer with manifests dir %s", opts.ManifestDir)

	opts.SecurityOptions.Insecure = o.insecure

	regctx, err := image.NewContext(o.SkipVerification)
	if err != nil {
		return nil, fmt.Errorf("error creating registry context: %v", err)
	}
	opts.SecurityOptions.CachedContext = regctx

	return opts, nil
}

const mappingFile = "mapping.txt"

// Copied from https://github.com/openshift/oc/blob/4df50be4d929ce036c4f07893c07a1782eadbbba/pkg/cli/admin/catalog/mirror.go#L449-L503
// Hoping this can be temporary, and `oc adm mirror catalog` libs support index.yaml direct mirroring.

type declcfgMeta struct {
	Schema        string                `json:"schema"`
	Image         string                `json:"image"`
	RelatedImages []declcfgRelatedImage `json:"relatedImages,omitempty"`
}

type declcfgRelatedImage struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

func parseRelatedImages(root string) (map[string]struct{}, error) {
	rootFS := os.DirFS(root)

	matcher, err := ignore.NewMatcher(rootFS, ".indexignore")
	if err != nil {
		return nil, err
	}

	relatedImages := map[string]struct{}{}
	if err := fs.WalkDir(rootFS, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || matcher.Match(path, false) {
			return nil
		}
		f, err := rootFS.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		dec := yaml.NewYAMLOrJSONDecoder(f, 4096)
		for {
			var blob declcfgMeta
			if err := dec.Decode(&blob); err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
			relatedImages[blob.Image] = struct{}{}
			for _, ri := range blob.RelatedImages {
				relatedImages[ri.Image] = struct{}{}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	delete(relatedImages, "")
	return relatedImages, nil
}

func validate(dc *declcfg.DeclarativeConfig) error {
	if dc == nil {
		return errors.New("bug: nil declarative config")
	}
	return nil
}
