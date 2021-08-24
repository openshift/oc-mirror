package operator

import (
	"context"
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
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/RedHatGov/bundle/pkg/bundle"
	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/RedHatGov/bundle/pkg/image"
)

// MirrorOptions configures either a Full or Diff mirror operation
// on a particular operator catalog image.
type MirrorOptions struct {
	RootDestDir string
	DryRun      bool
	SkipCleanup bool
	SkipTLS     bool

	Logger *logrus.Entry
}

// complete defaults MirrorOptions.
func (o *MirrorOptions) complete() {
	if o.RootDestDir == "" {
		o.RootDestDir = "create"
	}

	if o.Logger == nil {
		o.Logger = logrus.NewEntry(logrus.New())
	}
}

func (o *MirrorOptions) mktempDir() (string, func(), error) {
	dir := filepath.Join(o.RootDestDir, fmt.Sprintf("operators.%d", time.Now().Unix()))
	return dir, func() {
		if err := os.RemoveAll(dir); err != nil {
			logrus.Error(err)
		}
	}, os.MkdirAll(dir, os.ModePerm)
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

// Full mirrors each catalog image in its entirety to the <RootDestDir>/src directory.
func (o *MirrorOptions) Full(ctx context.Context, cfg v1alpha1.ImageSetConfiguration) (image.Associations, error) {
	o.complete()

	tmp, cleanup, err := o.mktempDir()
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

	allAssocs := image.Associations{}
	for _, ctlg := range cfg.Mirror.Operators {
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

			dc, err = o.diff(ctx, a, ctlg, cfg, tmp)
		} else {
			// Mirror the entire catalog.
			a := action.Render{
				Registry: reg,
				Refs:     []string{ctlg.Catalog},
			}

			dc, err = o.full(ctx, a, ctlg, cfg, tmp)
		}
		if err != nil {
			return nil, err
		}

		if err := o.associateDeclarativeConfigImageLayers(tmp, dc, allAssocs); err != nil {
			return nil, err
		}
	}

	return allAssocs, nil
}

// Diff mirrors only the diff between each old and new catalog image pair
// to the <rootDir>/src directory.
func (o *MirrorOptions) Diff(ctx context.Context, cfg v1alpha1.ImageSetConfiguration, lastRun v1alpha1.PastMirror) (image.Associations, error) {
	o.complete()

	tmp, cleanup, err := o.mktempDir()
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

	allAssocs := image.Associations{}
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

		dc, err := o.diff(ctx, a, ctlg, cfg, tmp)
		if err != nil {
			return nil, err
		}

		if err := o.associateDeclarativeConfigImageLayers(tmp, dc, allAssocs); err != nil {
			return nil, err
		}
	}

	return allAssocs, nil
}

func (o *MirrorOptions) full(ctx context.Context, a action.Render, ctlg v1alpha1.Operator, cfg v1alpha1.ImageSetConfiguration, tmp string) (*declcfg.DeclarativeConfig, error) {

	opts, err := o.newMirrorCatalogOptions(ctlg)
	if err != nil {
		return nil, err
	}

	ctlgRef, err := imgreference.Parse(ctlg.Catalog)
	if err != nil {
		return nil, fmt.Errorf("error parsing catalog: %v", err)
	}
	ctlgRef = ctlgRef.DockerClientDefaults()

	// Create the manifests dir in tmp so it gets cleaned up if desired.
	// opts.Complete() will create this directory.
	// TODO(estroz): if the CLI is meant to be re-entrant, check if this file exists
	// and use it directly if so.
	opts.ManifestDir = filepath.Join(tmp, fmt.Sprintf("manifests-%s-%d", ctlgRef.Name, time.Now().Unix()))

	args := []string{
		// The source is the catalog image itself.
		ctlg.Catalog,
		// The destination is within <RootDestDir>/src/v2/<image-name>.
		refToFileScheme(ctlgRef),
	}
	if err := opts.Complete(&cobra.Command{}, args); err != nil {
		return nil, fmt.Errorf("error constructing catalog options: %v", err)
	}

	// TODO(estroz): the mirrorer needs to be set after Complete() is called
	// because the default ImageMirrorer does not set FileDir from opts.
	// This isn't great because the default ImageMirrorer is a closure
	// and may contain configuration that gets overridden here.
	if opts.ImageMirrorer, err = newMirrorerFunc(cfg, opts); err != nil {
		return nil, fmt.Errorf("error creating mirror function: %v", err)
	}

	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("catalog opts invalid: %v", err)
	}

	if err := opts.Run(); err != nil {
		return nil, fmt.Errorf("error mirroring catalog: %v", err)
	}

	dc, err := a.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("error generating diff: %v", err)
	}

	// Write catalog declarative config file to src so it is included in the archive
	// at a path unique to the image.
	indexDir, err := dcDirForImage(o.RootDestDir, ctlgRef)
	if err != nil {
		return nil, err
	}

	o.Logger.Debugf("generating catalog diff in %s", indexDir)

	catalogIndexPath := filepath.Join(indexDir, fmt.Sprintf("%s-index-full.yaml", ctlgRef.Tag))
	f, err := os.Create(catalogIndexPath)
	if err != nil {
		return nil, fmt.Errorf("error creating full index file: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			o.Logger.Error(err)
		}
	}()
	if err := declcfg.WriteJSON(*dc, f); err != nil {
		return nil, fmt.Errorf("error writing full catalog: %v", err)
	}

	return dc, nil
}

func (o *MirrorOptions) diff(ctx context.Context, a action.Diff, ctlg v1alpha1.Operator, cfg v1alpha1.ImageSetConfiguration, tmp string) (*declcfg.DeclarativeConfig, error) {

	ctlgRef, err := imgreference.Parse(ctlg.Catalog)
	if err != nil {
		return nil, fmt.Errorf("error parsing catalog: %v", err)
	}
	ctlgRef = ctlgRef.DockerClientDefaults()

	// Write catalog declarative config file to src so it is included in the archive
	// at a path unique to the image.
	indexDir, err := dcDirForImage(o.RootDestDir, ctlgRef)
	if err != nil {
		return nil, err
	}

	o.Logger.Debugf("generating catalog %q diff in %s", ctlg.Catalog, indexDir)

	dc, err := a.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("error generating diff: %v", err)
	}

	catalogIndexPath := filepath.Join(indexDir, fmt.Sprintf("%s-index-diff.yaml", ctlgRef.Tag))
	f, err := os.Create(catalogIndexPath)
	if err != nil {
		return nil, fmt.Errorf("error creating diff index file: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			o.Logger.Error(err)
		}
	}()
	if err := declcfg.WriteJSON(*dc, f); err != nil {
		return nil, fmt.Errorf("error writing diff catalog: %v", err)
	}

	o.Logger.Debugf("wrote index to file: %s", catalogIndexPath)

	opts, err := o.newMirrorCatalogOptions(ctlg)
	if err != nil {
		return nil, err
	}

	opts.IndexExtractor = catalog.IndexExtractorFunc(func(imagesource.TypedImageReference) (string, error) {
		o.Logger.Debugf("returning index dir in extractor: %s", indexDir)
		return indexDir, nil
	})

	opts.RelatedImagesParser = catalog.RelatedImagesParserFunc(parseRelatedImages)

	if opts.ImageMirrorer, err = newMirrorerFunc(cfg, opts); err != nil {
		return nil, fmt.Errorf("error constructing mirror func: %v", err)
	}

	opts.IndexPath = indexDir

	if opts.SourceRef, err = imagesource.ParseReference(ctlgRef.Exact()); err != nil {
		return nil, fmt.Errorf("error parsing source reference: %v", err)
	}
	o.Logger.Debugf("source ref %s", opts.SourceRef.String())

	if opts.DestRef, err = imagesource.ParseReference(refToFileScheme(ctlgRef)); err != nil {
		return nil, fmt.Errorf("error parsing dest reference: %v", err)
	}
	o.Logger.Debugf("dest ref %s", opts.DestRef.String())

	// TODO(estroz): if the CLI is meant to be re-entrant, check if this file exists
	// and use it directly if so.
	opts.ManifestDir = filepath.Join(tmp, fmt.Sprintf("manifests-%s-%d", opts.SourceRef.Ref.Name, time.Now().Unix()))
	if err = os.MkdirAll(opts.ManifestDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("error creating manifests dir: %v", err)
	}
	o.Logger.Debugf("running mirrorer with manifests dir %s", opts.ManifestDir)

	if err := opts.Run(); err != nil {
		return nil, fmt.Errorf("error running catalog mirror: %v", err)
	}

	return dc, nil
}

func dcDirForImage(rootDir string, ref imgreference.DockerImageReference) (string, error) {
	indexDir := filepath.Join(rootDir, config.SourceDir, "catalogs",
		ref.Registry, ref.Namespace, ref.Name)
	if err := os.MkdirAll(indexDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("error creating diff index dir: %v", err)
	}
	return indexDir, nil
}

func (o *MirrorOptions) newMirrorCatalogOptions(ctlg v1alpha1.Operator) (*catalog.MirrorCatalogOptions, error) {
	stream := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	opts := catalog.NewMirrorCatalogOptions(stream)
	opts.DryRun = o.DryRun
	opts.FileDir = filepath.Join(o.RootDestDir, config.SourceDir)
	// FIXME(jpower): need to have the user set skipVerification value
	// If the pullSecret is not empty create a cached context
	// else let `oc mirror` use the default docker config location
	if len(ctlg.PullSecret) != 0 {
		ctx, err := config.CreateContext([]byte(ctlg.PullSecret), false, o.SkipTLS)
		if err != nil {
			return nil, err
		}
		opts.SecurityOptions.CachedContext = ctx
	}

	opts.SecurityOptions.Insecure = o.SkipTLS

	return opts, nil
}

// Input refs to the mirror library must be prefixed with "file://" if a local file ref.
func refToFileScheme(ref imgreference.DockerImageReference) string {
	return "file://" + filepath.Join(ref.Namespace, ref.Name)
}

func (o *MirrorOptions) associateDeclarativeConfigImageLayers(mappingDir string, dc *declcfg.DeclarativeConfig, allAssocs image.Associations) error {

	images := []string{}
	for _, b := range dc.Bundles {
		images = append(images, b.Image)
		for _, relatedImg := range b.RelatedImages {
			images = append(images, relatedImg.Image)
		}
	}

	foundAtLeastOneMapping := false
	if err := filepath.Walk(mappingDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if filepath.Base(path) != "mapping.txt" {
			return nil
		}
		foundAtLeastOneMapping = true

		o.Logger.Debugf("reading mapping file %s", path)
		imgMappings, err := image.ReadImageMapping(path)
		if err != nil {
			return err
		}

		srcDir := filepath.Join(o.RootDestDir, config.SourceDir)
		assocs, err := image.AssociateImageLayers(srcDir, imgMappings, images)
		if merr, ok := err.(*image.MirrorError); ok {
			logrus.Warn(merr)
		} else {
			return err
		}

		allAssocs.Merge(assocs)
		return nil
	}); err != nil {
		return err
	}

	if !foundAtLeastOneMapping {
		return fmt.Errorf("no operator catalog mappings found")
	}

	return nil
}

// Copied from https://github.com/openshift/oc/blob/4df50be4d929ce036c4f07893c07a1782eadbbba/pkg/cli/admin/catalog/mirror.go#L284-L313
// Hoping this can be temporary, and `oc adm mirror catalog` libs support index.yaml direct mirroring.

func newMirrorerFunc(cfg v1alpha1.ImageSetConfiguration, opts *catalog.MirrorCatalogOptions) (catalog.ImageMirrorerFunc, error) {
	allmanifestsFilter := imagemanifest.FilterOptions{FilterByOS: ".*"}
	if err := allmanifestsFilter.Validate(); err != nil {
		return nil, err
	}

	return func(mapping map[imagesource.TypedImageReference]imagesource.TypedImageReference) error {

		mappings := []imgmirror.Mapping{}
		for from, to := range mapping {

			if bundle.IsBlocked(cfg, from.Ref) {
				logrus.Debugf("image %s was specified as blocked per config, skipping...", from.Ref.Name)
				continue
			}
			mappings = append(mappings, imgmirror.Mapping{
				Source:      from,
				Destination: to,
			})
		}

		a := imgmirror.NewMirrorImageOptions(opts.IOStreams)
		a.SkipMissing = true
		a.ContinueOnError = true
		a.DryRun = opts.DryRun
		a.SecurityOptions = opts.SecurityOptions
		a.FileDir = opts.FileDir
		// because images in the catalog are statically referenced by digest,
		// we do not allow filtering for mirroring. this may change if sparse manifestlists are allowed
		// by registries, or if multi-arch management moves into images that can be rewritten on mirror (i.e. the bundle
		// images themselves, not the images referenced inside of the bundle images).
		a.FilterOptions = allmanifestsFilter
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
	}, nil
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
