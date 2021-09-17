package operator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

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
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/RedHatGov/bundle/pkg/bundle"
	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/RedHatGov/bundle/pkg/image"
)

// MirrorOptions configures either a Full or Diff mirror operation
// on a particular operator catalog image.
type MirrorOptions struct {
	cli.RootOptions

	Logger *logrus.Entry

	tmp string
}

func NewMirrorOptions(ro cli.RootOptions) *MirrorOptions {
	return &MirrorOptions{RootOptions: ro}
}

// complete defaults MirrorOptions.
func (o *MirrorOptions) complete() {
	if o.Dir == "" {
		o.Dir = "create"
	}

	if o.Logger == nil {
		o.Logger = logrus.NewEntry(logrus.New())
	}
}

func (o *MirrorOptions) mktempDir() (func(), error) {
	o.tmp = filepath.Join(o.Dir, fmt.Sprintf("operators.%d", time.Now().Unix()))
	return func() {
		if err := os.RemoveAll(o.tmp); err != nil {
			o.Logger.Error(err)
		}
	}, os.MkdirAll(o.tmp, os.ModePerm)
}

func (o *MirrorOptions) createRegistry() (*containerdregistry.Registry, error) {
	cacheDir, err := os.MkdirTemp("", "imageset-catalog-registry-")
	if err != nil {
		return nil, err
	}

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)
	nullLogger := logrus.NewEntry(logger)

	return containerdregistry.NewRegistry(
		containerdregistry.WithCacheDir(cacheDir),
		containerdregistry.SkipTLS(o.SkipTLS),
		// The containerd registry impl is somewhat verbose, even on the happy path,
		// so discard all logger logs. Any important failures will be returned from
		// registry methods and eventually logged as fatal errors.
		containerdregistry.WithLog(nullLogger),
	)
}

// Full mirrors each catalog image in its entirety to the <Dir>/src directory.
func (o *MirrorOptions) Full(ctx context.Context, cfg v1alpha1.ImageSetConfiguration) (image.Associations, error) {
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

	allAssocs := image.NewAssociations()
	for _, ctlg := range cfg.Mirror.Operators {
		ctlgRef, err := imagesource.ParseReference(ctlg.Catalog)
		if err != nil {
			return nil, fmt.Errorf("error parsing catalog: %v", err)
		}
		ctlgRef.Ref = ctlgRef.Ref.DockerClientDefaults()

		opts, err := o.newMirrorCatalogOptions(ctlgRef.Ref, filepath.Join(o.Dir, config.SourceDir), []byte(ctlg.PullSecret))
		if err != nil {
			return nil, err
		}

		catLogger := o.Logger.WithField("catalog", ctlg.Catalog)
		var dc *declcfg.DeclarativeConfig
		if ctlg.HeadsOnly {
			// Generate and mirror a heads-only diff using only the catalog as a new ref.
			a := action.Diff{
				Registry:      reg,
				NewRefs:       []string{ctlg.Catalog},
				Logger:        catLogger,
				IncludeConfig: ctlg.DiffIncludeConfig,
			}

			dc, err = o.diff(ctx, a, opts, ctlgRef, cfg)
		} else {
			// Mirror the entire catalog.
			a := action.Render{
				Registry: reg,
				Refs:     []string{ctlg.Catalog},
			}

			dc, err = o.full(ctx, a, opts, ctlgRef, cfg)
		}
		if err != nil {
			return nil, err
		}

		if err := o.associateDeclarativeConfigImageLayers(ctlgRef, dc, allAssocs); err != nil {
			return nil, err
		}
	}

	return allAssocs, nil
}

// Diff mirrors only the diff between each old and new catalog image pair
// to the <Dir>/src directory.
func (o *MirrorOptions) Diff(ctx context.Context, cfg v1alpha1.ImageSetConfiguration, lastRun v1alpha1.PastMirror) (image.Associations, error) {
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

	allAssocs := image.NewAssociations()
	for _, ctlg := range cfg.Mirror.Operators {
		// Generate and mirror a heads-only diff using the catalog as a new ref,
		// and an old ref found for this catalog in lastRun.
		catLogger := o.Logger.WithField("catalog", ctlg.Catalog)
		a := action.Diff{
			Registry:      reg,
			NewRefs:       []string{ctlg.Catalog},
			Logger:        catLogger,
			IncludeConfig: ctlg.DiffIncludeConfig,
		}

		// An old ref is always required to generate a latest diff.
		for _, operator := range lastRun.Operators {
			if operator.Catalog != ctlg.Catalog {
				continue
			}

			switch {
			case operator.RelIndexPath != "":
				// TODO(estroz)
			case len(operator.Index) != 0:
				// TODO(estroz)
			case operator.ImagePin != "":
				a.OldRefs = []string{operator.ImagePin}
			default:
				return nil, fmt.Errorf("metadata sequence %d catalog %q: at least one of RelIndexPath, Index, or ImagePin must be set", lastRun.Sequence, ctlg.Catalog)
			}
			break
		}

		ctlgRef, err := imagesource.ParseReference(ctlg.Catalog)
		if err != nil {
			return nil, fmt.Errorf("error parsing catalog: %v", err)
		}
		ctlgRef.Ref = ctlgRef.Ref.DockerClientDefaults()

		opts, err := o.newMirrorCatalogOptions(ctlgRef.Ref, filepath.Join(o.Dir, config.SourceDir), []byte(ctlg.PullSecret))
		if err != nil {
			return nil, err
		}

		dc, err := o.diff(ctx, a, opts, ctlgRef, cfg)
		if err != nil {
			return nil, err
		}

		if err := o.associateDeclarativeConfigImageLayers(ctlgRef, dc, allAssocs); err != nil {
			return nil, err
		}
	}

	return allAssocs, nil
}

func (o *MirrorOptions) mirror(ctx context.Context, opts *catalog.MirrorCatalogOptions, ctlgRef, destRef imagesource.TypedImageReference, isBlockedFuncs ...blockedFunc) (err error) {

	// Complete() will do a bunch of setup, so just let it re-parse src and dst.
	args := []string{
		ctlgRef.String(),
		destRef.String(),
	}
	if err := opts.Complete(&cobra.Command{}, args); err != nil {
		return fmt.Errorf("error constructing catalog options: %v", err)
	}
	opts.DestRef = imagesource.TypedImageReference{
		Type: imagesource.DestinationFile,
	}

	// TODO(estroz): the mirrorer needs to be set after Complete() is called
	// because the default ImageMirrorer does not set FileDir from opts nor
	// does in understand blocked mappings.
	// This isn't great because the default ImageMirrorer is a closure
	// and may contain configuration that gets overridden here.
	opts.ImageMirrorer = newMirrorerFunc(opts, isBlockedFuncs...)

	if err := opts.Validate(); err != nil {
		return fmt.Errorf("invalid catalog mirror options: %v", err)
	}

	if err := opts.Run(); err != nil {
		return fmt.Errorf("error mirroring catalog: %v", err)
	}

	return nil
}

func (o *MirrorOptions) full(ctx context.Context, a action.Render, opts *catalog.MirrorCatalogOptions, ctlgRef imagesource.TypedImageReference, cfg v1alpha1.ImageSetConfiguration) (*declcfg.DeclarativeConfig, error) {

	isBlocked := func(ref imgreference.DockerImageReference) bool {
		return bundle.IsBlocked(cfg, ref)
	}
	if err := o.mirror(ctx, opts, ctlgRef, refToFileRef(ctlgRef.Ref), isBlocked); err != nil {
		return nil, err
	}

	o.Logger.Debugf("rendering catalog %q", ctlgRef.Ref.Exact())

	dc, err := a.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("error rendering catalog: %v", err)
	}

	if _, err := o.writeDC(dc, ctlgRef.Ref); err != nil {
		return nil, err
	}

	return dc, nil
}

func (o *MirrorOptions) diff(ctx context.Context, a action.Diff, opts *catalog.MirrorCatalogOptions, ctlgRef imagesource.TypedImageReference, cfg v1alpha1.ImageSetConfiguration) (*declcfg.DeclarativeConfig, error) {

	o.Logger.Debugf("generating catalog %q diff", ctlgRef.Ref.Exact())

	dc, err := a.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("error generating catalog diff: %v", err)
	}

	indexDir, err := o.writeDC(dc, ctlgRef.Ref)
	if err != nil {
		return nil, err
	}

	opts.IndexExtractor = catalog.IndexExtractorFunc(func(imagesource.TypedImageReference) (string, error) {
		o.Logger.Debugf("returning index dir in extractor: %s", indexDir)
		return indexDir, nil
	})
	opts.IndexPath = indexDir

	opts.RelatedImagesParser = catalog.RelatedImagesParserFunc(parseRelatedImages)

	isBlocked := func(ref imgreference.DockerImageReference) bool {
		return bundle.IsBlocked(cfg, ref)
	}
	opts.ImageMirrorer = newMirrorerFunc(opts, isBlocked)

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

	return dc, nil
}

func (o *MirrorOptions) writeDC(dc *declcfg.DeclarativeConfig, ctlgRef imgreference.DockerImageReference) (string, error) {

	// Write catalog declarative config file to src so it is included in the archive
	// at a path unique to the image.
	indexDir := filepath.Join(o.Dir, config.SourceDir, "catalogs", ctlgRef.Registry, ctlgRef.Namespace, ctlgRef.Name)
	if err := os.MkdirAll(indexDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("error creating diff index dir: %v", err)
	}

	prefix := ctlgRef.Tag
	if prefix == "" {
		prefix = ctlgRef.ID
	}
	catalogIndexPath := filepath.Join(indexDir, fmt.Sprintf("%s-index.json", prefix))

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

func (o *MirrorOptions) newMirrorCatalogOptions(ctlgRef imgreference.DockerImageReference, fileDir string, pullSecret []byte) (*catalog.MirrorCatalogOptions, error) {
	opts := catalog.NewMirrorCatalogOptions(o.IOStreams)
	opts.DryRun = o.DryRun
	opts.FileDir = fileDir
	opts.MaxPathComponents = 2

	// Create the manifests dir in tmp so it gets cleaned up if desired.
	// TODO(estroz): if the CLI is meant to be re-entrant, check if this file exists
	// and use it directly if so.
	opts.ManifestDir = filepath.Join(o.tmp, fmt.Sprintf("manifests-%s-%d", ctlgRef.Name, time.Now().Unix()))
	if err := os.MkdirAll(opts.ManifestDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("error creating manifests dir: %v", err)
	}
	o.Logger.Debugf("running mirrorer with manifests dir %s", opts.ManifestDir)

	opts.SecurityOptions.Insecure = o.SkipTLS
	opts.SecurityOptions.SkipVerification = o.SkipVerification

	// If the pullSecret is not empty create a cached context
	// else let `oc mirror` use the default docker config location
	if len(pullSecret) != 0 {
		ctx, err := config.CreateContext(pullSecret, o.SkipVerification, o.SkipTLS)
		if err != nil {
			return nil, err
		}
		opts.SecurityOptions.CachedContext = ctx
	}

	return opts, nil
}

// Input refs to the mirror library must be prefixed with "file://" if a local file ref.
func refToFileRef(ref imgreference.DockerImageReference) imagesource.TypedImageReference {
	ref.Registry = ""
	return imagesource.TypedImageReference{
		Type: imagesource.DestinationFile,
		Ref:  ref,
	}
}

// isMappingFile returns true if p has a mapping file name.
func isMappingFile(p string) bool {
	const mappingFile = "mapping.txt"
	return filepath.Base(p) == mappingFile
}

func (o *MirrorOptions) associateDeclarativeConfigImageLayers(ctlgRef imagesource.TypedImageReference, dc *declcfg.DeclarativeConfig, allAssocs image.Associations) error {

	var bundleImages, relatedImages []string
	for _, b := range dc.Bundles {
		bundleImages = append(bundleImages, b.Image)
		for _, relatedImg := range b.RelatedImages {
			relatedImages = append(relatedImages, relatedImg.Image)
		}
	}

	srcDir := filepath.Join(o.Dir, config.SourceDir)

	associateWithType := func(mappings map[string]string, images []string, typ image.ImageType) error {
		assocs, err := image.AssociateImageLayers(srcDir, mappings, images, typ)
		if err != nil {
			merr := &image.ErrNoMapping{}
			cerr := &image.ErrInvalidComponent{}
			for _, err := range err.Errors() {
				if !errors.As(err, &merr) && !errors.As(err, &cerr) {
					return err
				}
			}
			o.Logger.Warn(err)
		}

		allAssocs.Merge(assocs)

		return nil
	}

	foundAtLeastOneMapping := false
	if err := filepath.Walk(o.tmp, func(path string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !isMappingFile(path) {
			return nil
		}
		foundAtLeastOneMapping = true

		o.Logger.Debugf("reading mapping file %s", path)
		imgMappings, err := image.ReadImageMapping(path)
		if err != nil {
			return err
		}

		for _, err := range []error{
			// NB(estroz): assuming that any related image will not point to
			// an operator catalog or bundle may not be a valid assumption.
			// Even if not valid, mirroring bundle and catalog images as normal images
			// should work as though they were mirrored via the `catalog mirror` API.
			associateWithType(imgMappings, relatedImages, image.TypeOperatorRelatedImage),
			associateWithType(imgMappings, bundleImages, image.TypeOperatorBundle),
			associateWithType(imgMappings, []string{ctlgRef.Ref.Exact()}, image.TypeOperatorCatalog),
		} {
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if !foundAtLeastOneMapping {
		return fmt.Errorf("no operator catalog mappings found")
	}

	return nil
}

// Mostly copied from https://github.com/openshift/oc/blob/4df50be4d929ce036c4f07893c07a1782eadbbba/pkg/cli/admin/catalog/mirror.go#L284-L313
// Hoping this can be temporary, and `oc adm mirror catalog` libs support index.yaml direct mirroring.

type blockedFunc func(imgreference.DockerImageReference) bool

func newMirrorerFunc(opts *catalog.MirrorCatalogOptions, isBlockedFuncs ...blockedFunc) catalog.ImageMirrorerFunc {
	return func(mapping map[imagesource.TypedImageReference]imagesource.TypedImageReference) error {

		mappings := []imgmirror.Mapping{}
		for from, to := range mapping {

			blocked := false
			for _, isBlocked := range isBlockedFuncs {
				if isBlocked(from.Ref) {
					logrus.Debugf("image %s was specified as blocked, skipping...", from.Ref.Name)
					blocked = true
					break
				}
			}
			if !blocked {
				mappings = append(mappings, imgmirror.Mapping{
					Source:      from,
					Destination: to,
				})
			}
		}

		a := imgmirror.NewMirrorImageOptions(opts.IOStreams)
		a.SkipMissing = true
		a.ContinueOnError = true
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
		a.Mappings = mappings
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
				// TODO: blocked images.
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
