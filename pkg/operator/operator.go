package operator

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/joelanford/ignore"
	imgreference "github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/admin/catalog"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	imagemanifest "github.com/openshift/oc/pkg/cli/image/manifest"
	imgmirror "github.com/openshift/oc/pkg/cli/image/mirror"
	"github.com/operator-framework/operator-registry/pkg/action"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/RedHatGov/bundle/pkg/bundle"
	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

// OperatorOptions configures either a Full or Diff mirror operation
// on a particular operator catalog image.
type OperatorOptions struct {
	RootDestDir string
	DryRun      bool
	Cleanup     bool
	SkipTLS     bool
}

// NewOperatorOptions defaults OperatorOptions.
func NewOperatorOptions() *OperatorOptions {
	return &OperatorOptions{}
}

func mktempDir(srcDir string) (string, func(), error) {
	dir := filepath.Join(srcDir, fmt.Sprintf("operators.%d", time.Now().Unix()))
	return dir, func() {
		if err := os.RemoveAll(dir); err != nil {
			logrus.Error(err)
		}
	}, os.MkdirAll(dir, os.ModePerm)
}

// Full mirrors each catalog image in its entirety to the <RootDestDir>/src directory.
func (o *OperatorOptions) Full(ctx context.Context, cfg v1alpha1.ImageSetConfiguration) (err error) {

	srcDir := filepath.Join(o.RootDestDir, config.SourceDir)
	tmp, cleanup, err := mktempDir(srcDir)
	if err != nil {
		return err
	}
	if o.Cleanup {
		defer cleanup()
	}

	for _, ctlg := range cfg.Mirror.Operators {
		opts := o.newMirrorCatalogOptions(ctlg, srcDir)

		if ctlg.HeadsOnly {
			// Generate and mirror a heads-only diff using only the catalog as a new ref.
			catLogger := logrus.WithField("catalog", ctlg.Catalog)
			a := action.Diff{
				NewRefs:       []string{ctlg.Catalog},
				Logger:        catLogger,
				IncludeConfig: ctlg.IncludeCatalog,
			}

			err = o.diff(ctx, a, ctlg, cfg, opts, tmp)
		} else {
			// Mirror the entire catalog.
			err = o.full(ctx, ctlg, cfg, opts, tmp)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// Diff mirrors only the diff between each old and new catalog image pair
// to the <RootDestDir>/src directory.
func (o *OperatorOptions) Diff(ctx context.Context, cfg v1alpha1.ImageSetConfiguration, lastRun v1alpha1.PastMirror) (err error) {

	srcDir := filepath.Join(o.RootDestDir, config.SourceDir)
	tmp, cleanup, err := mktempDir(srcDir)
	if err != nil {
		return err
	}
	if o.Cleanup {
		defer cleanup()
	}

	for _, ctlg := range cfg.Mirror.Operators {
		opts := o.newMirrorCatalogOptions(ctlg, srcDir)

		// Generate and mirror a heads-only diff using the catalog as a new ref,
		// and an old ref found for this catalog in lastRun.
		// TODO(estroz): registry
		catLogger := logrus.WithField("catalog", ctlg.Catalog)
		a := action.Diff{
			NewRefs:       []string{ctlg.Catalog},
			Logger:        catLogger,
			IncludeConfig: ctlg.IncludeCatalog,
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
				return fmt.Errorf("metadata sequence %d catalog %q: at least one of RelIndexPath, Index, or ImagePin must be set", lastRun.Sequence, ctlg.Catalog)
			}
			break
		}

		if err := o.diff(ctx, a, ctlg, cfg, opts, tmp); err != nil {
			return err
		}
	}

	return nil
}

func (o *OperatorOptions) full(_ context.Context, ctlg v1alpha1.Operator, cfg v1alpha1.ImageSetConfiguration, opts *catalog.MirrorCatalogOptions, tmp string) (err error) {

	ctlgRef, err := imgreference.Parse(ctlg.Catalog)
	if err != nil {
		return fmt.Errorf("error parsing catalog: %v", err)
	}
	ctlgRef = ctlgRef.DockerClientDefaults()

	// Create the manifests dir in tmp so it gets cleaned up if desired.
	// opts.Complete() will create this directory.
	opts.ManifestDir = filepath.Join(tmp, fmt.Sprintf("manifests-%s-%d", ctlgRef.Name, time.Now().Unix()))

	args := []string{
		// The source is the catalog image itself.
		ctlg.Catalog,
		// The destination is within <RootDestDir>/src/v2/<image-name>.
		refToFileScheme(ctlgRef),
	}
	if err := opts.Complete(&cobra.Command{}, args); err != nil {
		return fmt.Errorf("error constructing catalog options: %v", err)
	}

	// TODO(estroz): the mirrorer needs to be set after Complete() is called
	// because the default ImageMirrorer does not set FileDir from opts.
	// This isn't great because the default ImageMirrorer is a closure
	// and may contain configuration that gets overridden here.
	if opts.ImageMirrorer, err = newMirrorerFunc(cfg, opts); err != nil {
		return fmt.Errorf("error : %v", err)
	}

	if err := opts.Validate(); err != nil {
		return fmt.Errorf("catalog opts validation failed: %v", err)
	}

	if err := opts.Run(); err != nil {
		return fmt.Errorf("error running catalog mirror: %v", err)
	}

	return nil
}

func (o *OperatorOptions) diff(_ context.Context, a action.Diff, ctlg v1alpha1.Operator, cfg v1alpha1.ImageSetConfiguration, opts *catalog.MirrorCatalogOptions, tmp string) (err error) {

	ctlgRef, err := imgreference.Parse(ctlg.Catalog)
	if err != nil {
		return fmt.Errorf("error parsing catalog: %v", err)
	}
	ctlgRef = ctlgRef.DockerClientDefaults()

	imgIndexDir := filepath.Join(ctlgRef.Registry, ctlgRef.Namespace, ctlgRef.Name)
	indexDir := filepath.Join(tmp, imgIndexDir)
	a.Logger.Debugf("creating temporary index directory: %s", indexDir)
	if err := os.MkdirAll(indexDir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating diff index dir: %v", err)
	}

	catalogIndexPath := filepath.Join(indexDir, fmt.Sprintf("%s-index-diff.yaml", ctlgRef.Tag))
	f, err := os.Create(catalogIndexPath)
	if err != nil {
		return fmt.Errorf("error creating diff index file: %v", err)
	}
	close := func() {
		if err := f.Close(); err != nil {
			a.Logger.Error(err)
		}
	}

	a.Logger.Debugf("generating diff: %s", catalogIndexPath)
	ctx := context.Background()
	if err := a.RunAndWrite(ctx, f); err != nil {
		close()
		return fmt.Errorf("error generating diff: %v", err)
	}
	close()

	a.Logger.Debugf("wrote index to file: %s", catalogIndexPath)

	opts.IndexExtractor = catalog.IndexExtractorFunc(func(imagesource.TypedImageReference) (string, error) {
		a.Logger.Debugf("returning index dir in extractor: %s", indexDir)
		return indexDir, nil
	})

	opts.RelatedImagesParser = catalog.RelatedImagesParserFunc(parseRelatedImages)

	if opts.ImageMirrorer, err = newMirrorerFunc(cfg, opts); err != nil {
		return fmt.Errorf("error constructing mirror func: %v", err)
	}

	opts.IndexPath = indexDir

	if opts.SourceRef, err = imagesource.ParseReference(ctlgRef.Exact()); err != nil {
		return fmt.Errorf("error parsing source reference: %v", err)
	}
	a.Logger.Debugf("source ref %s", opts.SourceRef.String())

	if opts.DestRef, err = imagesource.ParseReference(refToFileScheme(ctlgRef)); err != nil {
		return fmt.Errorf("error parsing dest reference: %v", err)
	}
	a.Logger.Debugf("dest ref %s", opts.DestRef.String())

	opts.ManifestDir = filepath.Join(tmp, fmt.Sprintf("manifests-%s-%d", opts.SourceRef.Ref.Name, time.Now().Unix()))
	if err = os.MkdirAll(opts.ManifestDir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating manifests dir: %v", err)
	}
	a.Logger.Debugf("running mirrorer with manifests dir: %s", opts.ManifestDir)

	if err := opts.Run(); err != nil {
		return fmt.Errorf("error running catalog mirror: %v", err)
	}

	return nil
}

func (o *OperatorOptions) newMirrorCatalogOptions(ctlg v1alpha1.Operator, srcDir string) *catalog.MirrorCatalogOptions {
	stream := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	opts := catalog.NewMirrorCatalogOptions(stream)
	opts.DryRun = o.DryRun
	opts.FileDir = srcDir
	// TODO(estroz): this expects a file and PullSecret can be either a string or a file reference.
	opts.SecurityOptions.RegistryConfig = ctlg.PullSecret
	opts.SecurityOptions.Insecure = o.SkipTLS

	return opts
}

// Input refs to the mirror library must be prefixed with "file://" if a local file ref.
func refToFileScheme(ref imgreference.DockerImageReference) string {
	return "file://" + filepath.Join(ref.Namespace, ref.Name)
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
