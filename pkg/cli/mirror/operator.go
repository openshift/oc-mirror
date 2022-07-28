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
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/operator"
	"github.com/openshift/oc-mirror/pkg/operator/diff"
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
	f := func(ctx context.Context, reg *containerdregistry.Registry, ctlg v1alpha2.Operator) (*declcfg.DeclarativeConfig, v1alpha2.IncludeConfig, error) {
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

type renderDCFunc func(context.Context, *containerdregistry.Registry, v1alpha2.Operator) (*declcfg.DeclarativeConfig, v1alpha2.IncludeConfig, error)

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

		targetName, err := ctlg.GetUniqueName()
		if err != nil {
			return nil, err
		}
		targetCtlg, err := imagesource.ParseReference(targetName)
		if err != nil {
			return nil, fmt.Errorf("error parsing catalog: %v", err)
		}

		// Render the catalog to mirror into a declarative config.
		dc, ic, err := renderDC(ctx, reg, ctlg)
		if err != nil {
			return nil, o.checkValidationErr(err)
		}

		mappings, err := o.plan(ctx, dc, ic, ctlgRef, targetCtlg)
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
func (o *OperatorOptions) renderDCFull(ctx context.Context, reg *containerdregistry.Registry, ctlg v1alpha2.Operator) (dc *declcfg.DeclarativeConfig, ic v1alpha2.IncludeConfig, err error) {

	hasInclude := len(ctlg.IncludeConfig.Packages) != 0
	// Render the full catalog if neither HeadsOnly or IncludeConfig are specified.
	full := !ctlg.IsHeadsOnly() && !hasInclude

	catLogger := o.Logger.WithField("catalog", ctlg.Catalog)
	if full {
		// Mirror the entire catalog.
		dc, err = action.Render{
			Registry: reg,
			Refs:     []string{ctlg.Catalog},
		}.Run(ctx)
		if err != nil {
			return dc, ic, err
		}
	} else {
		// Generate and mirror a heads-only diff using only the catalog as a new ref.
		dic, derr := ctlg.IncludeConfig.ConvertToDiffIncludeConfig()
		if derr != nil {
			return dc, ic, derr
		}
		dc, err = diff.Diff{
			Registry:         reg,
			NewRefs:          []string{ctlg.Catalog},
			Logger:           catLogger,
			IncludeConfig:    dic,
			SkipDependencies: ctlg.SkipDependencies,
			HeadsOnly:        ctlg.IsHeadsOnly(),
		}.Run(ctx)
		if err != nil {
			return dc, ic, err
		}

		var icManager operator.IncludeConfigManager
		if hasInclude {
			icManager = operator.NewPackageStrategy(ctlg.IncludeConfig)
		} else {
			icManager = operator.NewCatalogStrategy()
		}

		// Render ic for incorporation into the metadata
		ic, err = icManager.ConvertDCToIncludeConfig(*dc)
		if err != nil {
			return dc, ic, fmt.Errorf("error converting declarative config to include config: %v", err)
		}

		if err := o.verifyDC(dic, dc); err != nil {
			return dc, ic, err
		}
	}

	return dc, ic, nil
}

// renderDCDiff renders data in ctlg into a declarative config for o.PlanDiff().
// This produces the declarative config that will be used to determine
// differential images
func (o *OperatorOptions) renderDCDiff(ctx context.Context, reg *containerdregistry.Registry, ctlg v1alpha2.Operator, lastRun v1alpha2.PastMirror) (dc *declcfg.DeclarativeConfig, ic v1alpha2.IncludeConfig, err error) {
	prevCatalog := make(map[string]v1alpha2.OperatorMetadata, len(lastRun.Operators))
	for _, ctlg := range lastRun.Operators {
		prevCatalog[ctlg.Catalog] = ctlg
	}

	hasInclude := len(ctlg.IncludeConfig.Packages) != 0
	// Process the catalog at heads-only or specified
	// packages at heads-only
	catalogHeadsOnly := ctlg.IsHeadsOnly() && !hasInclude
	includeWithHeadsOnly := ctlg.IsHeadsOnly() && hasInclude
	// Render the full catalog if neither HeadsOnly or IncludeConfig are specified.
	full := !ctlg.IsHeadsOnly() && !hasInclude

	// Generate and mirror a heads-only diff using the catalog as a new ref,
	// and an old ref found for this catalog in lastRun.
	catLogger := o.Logger.WithField("catalog", ctlg.Catalog)
	a := diff.Diff{
		Registry:         reg,
		NewRefs:          []string{ctlg.Catalog},
		Logger:           catLogger,
		SkipDependencies: ctlg.SkipDependencies,
	}

	// Instead of creating a partial FBC with diff
	// generate the current FBC according to the previous
	// include config or just render the full catalog again
	dic, err := ctlg.IncludeConfig.ConvertToDiffIncludeConfig()
	if err != nil {
		return dc, ic, err
	}
	switch {
	case full:
		// Mirror the entire catalog.
		dc, err = action.Render{
			Registry: reg,
			Refs:     []string{ctlg.Catalog},
		}.Run(ctx)
		if err != nil {
			return dc, ic, err
		}
	case catalogHeadsOnly || includeWithHeadsOnly:
		// If a previous catalog is found, reconcile the
		// previously stored IncludeConfig with the current catalog information
		// to make sure the bundles still exist. This causes the declarative config
		// to be rendered once to get the full information and then a second time to
		// get the final copy.
		prev, found := prevCatalog[ctlg.Catalog]
		if found {
			var icManager operator.IncludeConfigManager
			if catalogHeadsOnly {
				icManager = operator.NewCatalogStrategy()
				dc, err = action.Render{
					Registry: reg,
					Refs:     []string{ctlg.Catalog},
				}.Run(ctx)
				if err != nil {
					return dc, ic, err
				}
			} else {
				icManager = operator.NewPackageStrategy(ctlg.IncludeConfig)
				// Must set the current
				// diff include configuration to get the full
				// channels before recalculating.
				a.IncludeConfig = dic
				a.HeadsOnly = false
				dc, err = a.Run(ctx)
				if err != nil {
					return dc, ic, err
				}
			}

			ic, err = icManager.UpdateIncludeConfig(*dc, prev.IncludeConfig)
			if err != nil {
				return dc, ic, fmt.Errorf("error updating include config: %v", err)
			}
			dic, err = ic.ConvertToDiffIncludeConfig()
			if err != nil {
				return dc, ic, fmt.Errorf("error during include config conversion to declarative config: %v", err)
			}

			a.HeadsOnly = true
		}
		fallthrough
	default:
		// Default case is include
		// config will full channels
		a.IncludeConfig = dic
		dc, err = a.Run(ctx)
		if err != nil {
			return dc, ic, err
		}
	}

	if err := o.verifyDC(dic, dc); err != nil {
		return dc, ic, err
	}

	return dc, ic, nil
}

// verifyDC verifies the declarative config and that each of the requested operator packages were
// found and added to the DeclarativeConfig.
func (o *OperatorOptions) verifyDC(dic diff.DiffIncludeConfig, dc *declcfg.DeclarativeConfig) error {
	o.Logger.Debug("DiffIncludeConfig: ", dic)
	o.Logger.Debug("DeclarativeConfig: ", dc)

	// Converting the dc to the model results in running
	// model validations. This checks default channels and
	// replace chain.
	if _, err := declcfg.ConvertToModel(*dc); err != nil {
		return err
	}

	dcMap := make(map[string]bool)
	// Load the declarative config packages into a map
	for _, dcpkg := range dc.Packages {
		dcMap[dcpkg.Name] = true
	}

	for _, pkg := range dic.Packages {
		klog.V(2).Infof("Checking for package: %s", pkg)

		if !dcMap[pkg.Name] {
			// The operator package wasn't found. Log the error and continue on.
			o.Logger.Errorf("Operator %s was not found, please check name, minVersion, maxVersion, and channels in the config file.", pkg.Name)
		}
	}

	return nil
}

func (o *OperatorOptions) plan(ctx context.Context, dc *declcfg.DeclarativeConfig, ic v1alpha2.IncludeConfig, ctlgRef, targetCtlg imagesource.TypedImageReference) (image.TypedImageMapping, error) {

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

	indexDir, err := o.writeConfigs(dc, ic, targetCtlg.Ref)
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
	opts.MaxICSPSize = icspSizeLimit
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
	if err := o.writeLayout(ctx, ctlgRef.Ref, targetCtlg.Ref); err != nil {
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

		if _, ok := mapping[ref]; !ok {
			klog.Warningf("image %s is not included in mapping", img)
		}
		return nil
	}
	for _, b := range dc.Bundles {
		if err := validateFunc(b.Image); err != nil {
			errs = append(errs, fmt.Errorf("bundle %q image %q: %v", b.Name, b.Image, err))
			continue
		}
		for _, relatedImg := range b.RelatedImages {
			if err := validateFunc(relatedImg.Image); err != nil {
				errs = append(errs, fmt.Errorf("bundle %q related image %q: %v", b.Name, relatedImg.Name, err))
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
			klog.Warningf("bundle %s: pinning bundle image %s to digest", b.Name, b.Image)

			if !image.IsImageTagged(b.Image) {
				klog.Warningf("bundle %s: bundle image tag not set", b.Name)
				continue
			}
			if dc.Bundles[i].Image, err = image.ResolveToPin(ctx, resolver, b.Image); err != nil {
				if isSkipErr(err) {
					klog.Warningf("skipping bundle %s image %s resolve error: %v", b.Name, b.Image, err)
				} else {
					errs = append(errs, err)
				}
			}
		}
		for j, ri := range b.RelatedImages {
			if !image.IsImagePinned(ri.Image) {
				klog.Warningf("bundle %s: pinning related image %s to digest", ri.Name, ri.Image)

				if !image.IsImageTagged(ri.Image) {
					klog.Warningf("bundle %s: related image tag not set", b.Name)
					continue
				}

				if b.RelatedImages[j].Image, err = image.ResolveToPin(ctx, resolver, ri.Image); err != nil {
					if isSkipErr(err) {
						klog.Warningf("skipping bundle %s related image %s=%s resolve error: %v", b.Name, ri.Name, ri.Image, err)
					} else {
						errs = append(errs, err)
					}
				}
			}
		}
	}

	return utilerrors.NewAggregate(errs)
}

func (o *OperatorOptions) writeLayout(ctx context.Context, ctlgRef, targetCtlg imgreference.DockerImageReference) error {

	// Write catalog OCI layout file to src so it is included in the archive
	// at a path unique to the image.
	ctlgDir, err := operator.GenerateCatalogDir(targetCtlg)
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

// writeConfigs will write the declarative and include configuration to disk in a directory generated by the catalog name.
func (o *OperatorOptions) writeConfigs(dc *declcfg.DeclarativeConfig, ic v1alpha2.IncludeConfig, targetCtlg imgreference.DockerImageReference) (string, error) {

	// Write catalog declarative config file to src so it is included in the archive
	// at a path unique to the image.
	ctlgDir, err := operator.GenerateCatalogDir(targetCtlg)
	if err != nil {
		return "", err
	}
	catalogBasePath := filepath.Join(o.Dir, config.SourceDir, config.CatalogsDir, ctlgDir)
	indexDir := filepath.Join(catalogBasePath, config.IndexDir)
	if err := os.MkdirAll(indexDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("error creating diff index dir: %v", err)
	}
	catalogIndexPath := filepath.Join(indexDir, "index.json")

	o.Logger.Debugf("writing target catalog %q diff to %s", targetCtlg.Exact(), catalogIndexPath)

	indexFile, err := os.Create(catalogIndexPath)
	if err != nil {
		return "", fmt.Errorf("error creating diff index file: %v", err)
	}

	includeConfigPath := filepath.Join(catalogBasePath, config.IncludeConfigFile)

	o.Logger.Debugf("writing target catalog %q include config to %s", targetCtlg.Exact(), includeConfigPath)

	includeFile, err := os.Create(includeConfigPath)
	if err != nil {
		return "", fmt.Errorf("error creating include config file: %v", err)
	}

	defer func() {
		if err := includeFile.Close(); err != nil {
			o.Logger.Error(err)
		}
		if err := indexFile.Close(); err != nil {
			o.Logger.Error(err)
		}
	}()

	if err := declcfg.WriteJSON(*dc, indexFile); err != nil {
		return "", fmt.Errorf("error writing diff catalog: %v", err)
	}
	if err := ic.Encode(includeFile); err != nil {
		return "", fmt.Errorf("error writing include config file: %v", err)
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

// checkError will check validation errors
// from operator registry and return a modified
// error messages for mirror usage
// FIXME(jpower432): Checking against the errors string could be an issue since
// this is depending on the strings returned from `opm validate`. It would be better to propose that
// the validation error are exposed here https://github.com/operator-framework/operator-registry/blob/master/alpha/model/error.go
// and adds structure errors that return package and channels information.
func (o *OperatorOptions) checkValidationErr(err error) error {
	if err == nil {
		return nil
	}

	fmt.Fprintln(o.ErrOut, "\nThe rendered catalog is invalid.")
	// handle known error causes
	var validationMsg string
	switch {
	case strings.Contains(err.Error(), "channel must contain at least one bundle"):
		validationMsg = "\nPlease check the minVersion, maxVersion, and default channel for each invalid package."
	case strings.Contains(err.Error(), "multiple channel heads found in graph"):
		validationMsg = "\nPlease check the minVersion and maxVersion for each invalid channel."
	}
	fmt.Fprintln(o.ErrOut, "\nRun \"oc-mirror list operators --catalog CATALOG-NAME --package PACKAGE-NAME\" for more information.")
	fmt.Fprintln(o.ErrOut, validationMsg)
	return err
}
