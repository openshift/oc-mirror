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
	"github.com/joelanford/ignore"
	imgreference "github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/admin/catalog"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	imagemanifest "github.com/openshift/oc/pkg/cli/image/manifest"
	imgmirror "github.com/openshift/oc/pkg/cli/image/mirror"
	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/openshift/oc-mirror/pkg/bundle"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc-mirror/pkg/image"
)

var (
	// Pinned to upstream opm v1.19.0 (k8s 1.21).
	OPMImage = "quay.io/operator-framework/opm@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19"
)

// OperatorOptions configures either a Full or Diff mirror operation
// on a particular operator catalog image.
type OperatorOptions struct {
	MirrorOptions

	SkipImagePin bool
	Logger       *logrus.Entry

	tmp string
}

func NewOperatorOptions(mo MirrorOptions) *OperatorOptions {
	return &OperatorOptions{MirrorOptions: mo}
}

// Full mirrors each catalog image in its entirety to the <Dir>/src directory.
func (o *OperatorOptions) Full(ctx context.Context, cfg v1alpha1.ImageSetConfiguration) (image.AssociationSet, error) {
	return o.run(ctx, cfg, o.renderDCFull)
}

// Diff mirrors only the diff between each old and new catalog image pair
// to the <Dir>/src directory.
func (o *OperatorOptions) Diff(ctx context.Context, cfg v1alpha1.ImageSetConfiguration, lastRun v1alpha1.PastMirror) (image.AssociationSet, error) {
	f := func(ctx context.Context, reg *containerdregistry.Registry, ctlg v1alpha1.Operator) (*declcfg.DeclarativeConfig, error) {
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

type renderDCFunc func(context.Context, *containerdregistry.Registry, v1alpha1.Operator) (*declcfg.DeclarativeConfig, error)

// Diff mirrors only the diff between each old and new catalog image pair
// to the <Dir>/src directory.
func (o *OperatorOptions) run(ctx context.Context, cfg v1alpha1.ImageSetConfiguration, renderDC renderDCFunc) (image.AssociationSet, error) {
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

	allAssocs := image.AssociationSet{}
	for _, ctlg := range cfg.Mirror.Operators {

		ctlgRef, err := imagesource.ParseReference(ctlg.Catalog)
		if err != nil {
			return nil, fmt.Errorf("error parsing catalog: %v", err)
		}
		ctlgRef.Ref = ctlgRef.Ref.DockerClientDefaults()

		// Render the catalog to mirror into a declarative config.
		dc, err := renderDC(ctx, reg, ctlg)
		if err != nil {
			return nil, err
		}

		isBlocked := func(ref imgreference.DockerImageReference) bool {
			return bundle.IsBlocked(cfg, ref)
		}
		mappings, err := o.mirror(ctx, dc, ctlgRef, ctlg, isBlocked)
		if err != nil {
			return nil, err
		}

		// Do not build associations on dry runs because there are no manifests
		if !o.DryRun {
			assocs, err := o.associateDeclarativeConfigImageLayers(ctlgRef, dc, mappings)
			if err != nil {
				return nil, err
			}
			allAssocs.Merge(assocs)
		}
	}

	return allAssocs, nil
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
		containerdregistry.SkipTLS(o.SourceSkipTLS),
		// The containerd registry impl is somewhat verbose, even on the happy path,
		// so discard all logger logs. Any important failures will be returned from
		// registry methods and eventually logged as fatal errors.
		containerdregistry.WithLog(nullLogger),
	)
}

// renderDCFull renders data in ctlg into a declarative config for o.Full().
func (o *OperatorOptions) renderDCFull(ctx context.Context, reg *containerdregistry.Registry, ctlg v1alpha1.Operator) (dc *declcfg.DeclarativeConfig, err error) {

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
	} else {
		// Generate and mirror a heads-only diff using only the catalog as a new ref.
		dic, err := ctlg.IncludeConfig.ConvertToDiffIncludeConfig()
		if err != nil {
			return nil, err
		}
		dc, err = action.Diff{
			Registry:          reg,
			NewRefs:           []string{ctlg.Catalog},
			Logger:            catLogger,
			IncludeConfig:     dic,
			IncludeAdditively: includeAdditively,
			SkipDependencies:  ctlg.SkipDependencies,
		}.Run(ctx)
	}

	return dc, err
}

// renderDCDiff renders data in ctlg into a declarative config for o.Diff().
func (o *OperatorOptions) renderDCDiff(ctx context.Context, reg *containerdregistry.Registry, ctlg v1alpha1.Operator, lastRun v1alpha1.PastMirror) (dc *declcfg.DeclarativeConfig, err error) {
	hasInclude := len(ctlg.IncludeConfig.Packages) != 0
	// Generate and mirror a heads-only diff using the catalog as a new ref,
	// and an old ref found for this catalog in lastRun.
	catLogger := o.Logger.WithField("catalog", ctlg.Catalog)
	dic, err := ctlg.IncludeConfig.ConvertToDiffIncludeConfig()
	if err != nil {
		return nil, err
	}
	a := action.Diff{
		Registry:      reg,
		NewRefs:       []string{ctlg.Catalog},
		Logger:        catLogger,
		IncludeConfig: dic,
		// This is hard-coded to false because a diff post-metadata creation must always include
		// newly published catalog data to join graphs. Any included objects previously included
		// will be added as a diff as part of the latest diff mode.
		IncludeAdditively: false,
		SkipDependencies:  ctlg.SkipDependencies,
	}

	// An old ref is always required to generate a latest diff.
	// To make sure we always download the full include config
	// don't set the old ref when that is specified
	if !hasInclude {
		for _, operator := range lastRun.Operators {
			if operator.Catalog != ctlg.Catalog {
				continue
			}

			if operator.ImagePin == "" {
				return nil, fmt.Errorf("metadata sequence %d catalog %q: ImagePin must be set", lastRun.Sequence, ctlg.Catalog)
			}
			a.OldRefs = []string{operator.ImagePin}
			break
		}
	}

	return a.Run(ctx)
}

func (o *OperatorOptions) mirror(ctx context.Context, dc *declcfg.DeclarativeConfig, ctlgRef imagesource.TypedImageReference, ctlg v1alpha1.Operator, isBlocked ...blockedFunc) (map[string]string, error) {

	o.Logger.Debugf("Mirroring catalog %q bundle and related images", ctlgRef.Ref.Exact())

	opts, err := o.newMirrorCatalogOptions(ctlgRef.Ref, filepath.Join(o.Dir, config.SourceDir), []byte(ctlg.PullSecret))
	if err != nil {
		return nil, err
	}

	if !o.SkipImagePin {
		resolver, err := containerdregistry.NewResolver("", o.SourceSkipTLS, nil)
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
	// after rendering with the existing image in the publish step,
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

	mappings, err := image.ReadImageMapping(filepath.Join(opts.ManifestDir, mappingFile))
	if err != nil {
		return nil, err
	}

	// Remove the catalog image from mappings.
	delete(mappings, ctlgRef.Ref.Exact())

	// Remove catalog namespace prefix from each mapping's destination, which is added by opts.Run().
	for src, dst := range mappings {
		dstRef, err := imagesource.ParseReference(dst)
		if err != nil {
			return nil, err
		}
		newRepoName := strings.TrimPrefix(dstRef.Ref.RepositoryName(), ctlgRef.Ref.RepositoryName())
		newRepoName = strings.TrimPrefix(newRepoName, "/")
		tmpRef, err := imgreference.Parse(newRepoName)
		if err != nil {
			return nil, err
		}
		dstRef.Ref.Namespace = tmpRef.Namespace
		dstRef.Ref.Name = tmpRef.Name
		mappings[src] = dstRef.String()
	}

	return mappings, o.mirrorMappings(opts, mappings, isBlocked...)
}

// pinImages resolves every image in dc to it's canonical name (includes digest).
func (o *OperatorOptions) pinImages(ctx context.Context, dc *declcfg.DeclarativeConfig, resolver remotes.Resolver) (err error) {

	// Instead of returning an error, just log it.
	isSkipErr := func(err error) bool {
		return o.ContinueOnError || (o.SkipMissing && errors.Is(err, errdefs.ErrNotFound))
	}

	var errs []error
	for i, b := range dc.Bundles {

		if !image.IsImagePinned(b.Image) {

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

func (o *OperatorOptions) writeDC(dc *declcfg.DeclarativeConfig, ctlgRef imgreference.DockerImageReference) (string, error) {

	// Write catalog declarative config file to src so it is included in the archive
	// at a path unique to the image.
	leafDir := ctlgRef.Tag
	if leafDir == "" {
		leafDir = ctlgRef.ID
	}
	if leafDir == "" {
		return "", fmt.Errorf("catalog %q must have either a tag or digest", ctlgRef.Exact())
	}
	indexDir := filepath.Join(o.Dir, config.SourceDir, "catalogs", ctlgRef.Registry, ctlgRef.Namespace, ctlgRef.Name, leafDir)
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

func (o *OperatorOptions) newMirrorCatalogOptions(ctlgRef imgreference.DockerImageReference, fileDir string, pullSecret []byte) (*catalog.MirrorCatalogOptions, error) {
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

	opts.SecurityOptions.Insecure = o.SourceSkipTLS
	opts.SecurityOptions.SkipVerification = o.SkipVerification

	// If the pullSecret is not empty create a cached context
	// else let `oc mirror` use the default docker config location
	if len(pullSecret) != 0 {
		ctx, err := config.CreateContext(pullSecret, o.SkipVerification, o.SourceSkipTLS)
		if err != nil {
			return nil, err
		}
		opts.SecurityOptions.CachedContext = ctx
	}

	return opts, nil
}

const mappingFile = "mapping.txt"

// isMappingFile returns true if p has a mapping file name.
func isMappingFile(p string) bool {
	return filepath.Base(p) == mappingFile
}

func (o *OperatorOptions) associateDeclarativeConfigImageLayers(ctlgRef imagesource.TypedImageReference, dc *declcfg.DeclarativeConfig, mappings map[string]string) (image.AssociationSet, error) {

	var images []string
	for _, b := range dc.Bundles {
		images = append(images, b.Image)
		for _, relatedImg := range b.RelatedImages {
			images = append(images, relatedImg.Image)
		}
	}

	srcDir := filepath.Join(o.Dir, config.SourceDir)
	assocs, err := image.AssociateImageLayers(srcDir, mappings, images, image.TypeGeneric)
	if err != nil {
		merr := &image.ErrNoMapping{}
		cerr := &image.ErrInvalidComponent{}
		for _, err := range err.Errors() {
			if !errors.As(err, &merr) && !errors.As(err, &cerr) {
				return nil, err
			}
		}
		o.Logger.Warn(err)
	}

	return assocs, nil
}

type blockedFunc func(imgreference.DockerImageReference) bool

func (o *OperatorOptions) mirrorMappings(opts *catalog.MirrorCatalogOptions, mappings map[string]string, isBlockedFuncs ...blockedFunc) (err error) {
	mmappings := []imgmirror.Mapping{}
	for fromStr, toStr := range mappings {

		m := imgmirror.Mapping{Name: fromStr}
		if m.Source, err = imagesource.ParseReference(fromStr); err != nil {
			return err
		}
		if m.Destination, err = imagesource.ParseReference(toStr); err != nil {
			return err
		}
		blocked := false
		for _, isBlocked := range isBlockedFuncs {
			if isBlocked(m.Source.Ref) {
				logrus.Debugf("image %s was specified as blocked, skipping...", m.Source.Ref.Name)
				blocked = true
				break
			}
		}
		if !blocked {
			mmappings = append(mmappings, m)
		}
	}

	a := imgmirror.NewMirrorImageOptions(opts.IOStreams)
	a.SkipMissing = o.SkipMissing
	a.ContinueOnError = o.ContinueOnError
	a.DryRun = opts.DryRun
	a.SecurityOptions = opts.SecurityOptions
	// FileDir is set so images are mirrored under the correct directory.
	a.FileDir = opts.FileDir
	// because images in the catalog are statically referenced by digest,
	// we do not allow filtering for mirroring. this may change if sparse manifestlists are allowed
	// by registries, or if multi-arch management moves into images that can be rewritten on mirror (i.e. the bundle
	// images themselves, not the images referenced inside of the bundle images).
	a.FilterOptions = imagemanifest.FilterOptions{FilterByOS: ".*"}
	a.ParallelOptions = opts.ParallelOptions
	a.KeepManifestList = true
	a.Mappings = mmappings
	a.SkipMultipleScopes = true

	if err := a.Validate(); err != nil {
		fmt.Fprintf(opts.IOStreams.ErrOut, "error configuring image mirroring: %v\n", err)
		return nil
	}

	if err := a.Run(); err != nil {
		fmt.Fprintf(opts.IOStreams.ErrOut, "error mirroring image: %v\n", err)
		return nil
	}

	return nil
}

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
