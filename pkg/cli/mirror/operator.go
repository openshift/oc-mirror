package mirror

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	"github.com/otiai10/copy"
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
	// Wrapper renderDCDiff so it satisfies the renderDCFunc function signature.
	f := func(
		ctx context.Context,
		reg *containerdregistry.Registry,
		ctlg v1alpha2.Operator,
		digestsToProcess map[OperatorCatalogPlatform]CatalogMetadata,
	) error {
		return o.renderDCDiff(ctx, reg, ctlg, lastRun, digestsToProcess)
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

// ErrorMessagePrefix creates an error message prefix that handles both single and multi architecture images.
// In the case of a single architecture catalog image, we don't need a prefix since that would make things more confusing.
func (p *OperatorCatalogPlatform) ErrorMessagePrefix() string {
	// default to empty string for single architecture images
	platformErrorMessage := ""
	if p.isIndex {
		platformErrorMessage = fmt.Sprintf("platform %s: ", p.String())
	}
	return platformErrorMessage
}

/*
CatalogMetadata represents the combination of a DeclarativeConfig, and its associated
IncludeConfig, which is either provided by the user directly or generated from the contents
of DeclarativeConfig
*/
type CatalogMetadata struct {
	dc         *declcfg.DeclarativeConfig // a DeclarativeConfig instance
	ic         v1alpha2.IncludeConfig     // a IncludeConfig instance
	catalogRef name.Reference             // the reference used to obtain DeclarativeConfig and IncludeConfig
}

/*
renderDCFunc is a function signature for rendering declarative configurations for single and multi architecture catalogs.
Currently renderDCFull and renderDCDiff implement this function signature.

# Arguments

• context.Context: the cancellation context

• *containerdregistry.Registry: a containerd registry

• v1alpha2.Operator: operator metadata that should be processed

• map[OperatorCatalogPlatform]CatalogMetadata: This is an in/out parameter, where the CatalogMetadata value
initially contains a pre-fetched digest reference that corresponds to a specific platform. These digest values
originate with a v1alpha2.Operator.Catalog reference. The CatalogMetadata is augmented with DeclarativeConfig
and IncludeConfig data by the time this function returns.

# Returns

• error: non-nil if an error occurs, nil otherwise
*/
type renderDCFunc func(
	context.Context,
	*containerdregistry.Registry,
	v1alpha2.Operator,
	map[OperatorCatalogPlatform]CatalogMetadata,
) error

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

		ctlgRef, err := image.ParseReference(ctlg.Catalog)
		if err != nil {
			return nil, err
		}
		targetName, err := ctlg.GetUniqueName()
		if err != nil {
			return nil, err
		}
		if ctlg.IsFBCOCI() {
			targetName = v1alpha2.OCITransportPrefix + "//" + targetName
		}
		targetCtlg, err := image.ParseReference(targetName)
		if err != nil {
			return nil, fmt.Errorf("error parsing catalog: %v", err)
		}

		// get the digests to process... could be more than one if a manifest list image is provided
		// we do this here so we don't have to do it multiple times within the renderDC function
		digestsToProcess, err := getImageDigests(ctx, ctlg.Catalog, nil, o.insecure)
		if err != nil {
			return nil, fmt.Errorf("error fetching digests for catalog %s: %v", ctlg.Catalog, err)
		}
		// Render the catalog to mirror into a declarative config.
		err = renderDC(ctx, reg, ctlg, digestsToProcess)
		if err != nil {
			return nil, o.checkValidationErr(err)
		}

		mappings, err := o.plan(ctx, digestsToProcess, ctlgRef, targetCtlg)
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
	logger.SetOutput(io.Discard)
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
// Satisfies the renderDCFunc function signature.
func (o *OperatorOptions) renderDCFull(
	ctx context.Context,
	reg *containerdregistry.Registry,
	ctlg v1alpha2.Operator,
	digestsToProcess map[OperatorCatalogPlatform]CatalogMetadata,
) (err error) {
	hasInclude := len(ctlg.IncludeConfig.Packages) != 0
	// Render the full catalog if neither HeadsOnly or IncludeConfig are specified.
	full := !ctlg.IsHeadsOnly() && !hasInclude

	// TODO: this merged code needs to be addressed in some fashion
	// if ctlg.IsFBCOCI() {
	// 	// initialize path where we assume the catalog config dir is <current working directory>/olm_artifacts/<repo>/<config folder>
	// 	ctlgRef, err = o.getOperatorCatalogRef(ctx, ctlg.Catalog)
	// 	if err != nil {
	// 		return dc, ic, err
	// 	}
	// }

	// Now we need to process each architecture specific image that was discovered.
	// Failures encountered during processing of any architecture abandons processing
	// from that point forward.
	for platformKey, catalogMetadata := range digestsToProcess {
		digest := catalogMetadata.catalogRef
		catalog := digest.Name()
		catLogger := o.Logger.WithField("catalog", catalog)
		var dc *declcfg.DeclarativeConfig
		var ic v1alpha2.IncludeConfig
		if full {
			// Mirror the entire catalog.
			dc, err = action.Render{
				Registry: reg,
				Refs:     []string{catalog},
			}.Run(ctx)
			if err != nil {
				return err
			}
			// use the include config from v1alpha2.Operator
			ic = ctlg.IncludeConfig
		} else {
			// Generate and mirror a heads-only diff using only the catalog as a new ref.
			dic, derr := ctlg.IncludeConfig.ConvertToDiffIncludeConfig()
			if derr != nil {
				return derr
			}
			dc, err = diff.Diff{
				Registry:         reg,
				NewRefs:          []string{catalog},
				Logger:           catLogger,
				IncludeConfig:    dic,
				SkipDependencies: ctlg.SkipDependencies,
				HeadsOnly:        ctlg.IsHeadsOnly(),
			}.Run(ctx)
			if err != nil {
				return err
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
				return fmt.Errorf("error converting declarative config to include config: %v", err)
			}

			if err := o.verifyDC(dic, dc); err != nil {
				return err
			}
		}

		// update the local catalogMetadata value, and update the map with this copy
		catalogMetadata.dc = dc
		catalogMetadata.ic = ic
		digestsToProcess[platformKey] = catalogMetadata
	}

	return nil
}

// renderDCDiff renders data in ctlg into a declarative config for o.PlanDiff().
// This produces the declarative config that will be used to determine
// differential images.
func (o *OperatorOptions) renderDCDiff(
	ctx context.Context,
	reg *containerdregistry.Registry,
	ctlg v1alpha2.Operator,
	lastRun v1alpha2.PastMirror,
	digestsToProcess map[OperatorCatalogPlatform]CatalogMetadata,
) (err error) {
	// initialize map with previous catalogs by name for easy lookup
	prevCatalog := make(map[string]v1alpha2.OperatorMetadata, len(lastRun.Operators))
	for _, pastCtlg := range lastRun.Operators {
		prevCatalog[pastCtlg.Catalog] = pastCtlg
	}

	// now we need to process each architecture specific image that was discovered
	// failures encountered during processing of any architecture abandons processing
	// from that point forward
	for platformKey, catalogMetadata := range digestsToProcess {
		digest := catalogMetadata.catalogRef

		// TODO: this merged code needs to be addressed in some fashion
		// ctlgRef := ctlg.Catalog //applies for all docker-v2 remote catalogs
		// if ctlg.IsFBCOCI() {
		// 	ctlgRef, err = o.getOperatorCatalogRef(ctx, ctlg.Catalog)
		// 	if err != nil {
		// 		return dc, ic, err
		// 	}
		// }

		// TODO: this function currently uses OCI image paths for the "unique name"... is this OK??? It's only being used for metadata lookup, so it might be fine.
		// TODO: this needs to take into account architecture to make it unique
		uniqueName, err := ctlg.GetUniqueName()
		uniqueName = uniqueName + platformKey.String()

		if err != nil {
			// stop all processing since we hit an error
			return err
		}
		prev, found := prevCatalog[uniqueName]

		// The architecture specific catalog is new or we just need to mirror the full catalog or channels.
		if !found || !ctlg.IsHeadsOnly() {
			// render full handles all architectures so we can stop processing here and just return
			return o.renderDCFull(ctx, reg, ctlg, digestsToProcess)
		}

		hasInclude := len(ctlg.IncludeConfig.Packages) != 0
		// Process the catalog at heads-only or specified
		// packages at heads-only
		catalogHeadsOnly := ctlg.IsHeadsOnly() && !hasInclude
		includeWithHeadsOnly := ctlg.IsHeadsOnly() && hasInclude

		catalog := digest.Name()

		// Generate a heads-only diff using the catalog as a new ref and previous bundle information.
		catLogger := o.Logger.WithField("catalog", catalog)
		diffAction := diff.Diff{
			Registry:         reg,
			NewRefs:          []string{catalog},
			Logger:           catLogger,
			SkipDependencies: ctlg.SkipDependencies,
		}

		// If a previous catalog is found, reconcile the
		// previously stored IncludeConfig with the current catalog information
		// to make sure the bundles still exist. This causes the declarative config
		// to be rendered once to get the full information and then a second time to
		// get the final copy. This will help determine what bundles have been pruned.
		var icManager operator.IncludeConfigManager
		var dc *declcfg.DeclarativeConfig
		switch {
		case catalogHeadsOnly:
			icManager = operator.NewCatalogStrategy()
			dc, err = action.Render{
				Registry: reg,
				Refs:     []string{catalog},
			}.Run(ctx)
			if err != nil {
				// stop all processing since we hit an error
				return err
			}

		case includeWithHeadsOnly:
			icManager = operator.NewPackageStrategy(ctlg.IncludeConfig)
			// Must set the current
			// diff include configuration to get the full
			// channels before recalculating.
			dic, err := ctlg.IncludeConfig.ConvertToDiffIncludeConfig()
			if err != nil {
				// stop all processing since we hit an error
				return err
			}
			diffAction.IncludeConfig = dic
			diffAction.HeadsOnly = false
			dc, err = diffAction.Run(ctx)
			if err != nil {
				// stop all processing since we hit an error
				return err
			}
		}

		// Update the IncludeConfig and diff include configuration based on previous mirrored bundles
		// and the current catalog.
		ic, err := icManager.UpdateIncludeConfig(*dc, prev.IncludeConfig)
		if err != nil {
			return fmt.Errorf("error updating include config: %v", err)
		}
		dic, err := ic.ConvertToDiffIncludeConfig()
		if err != nil {
			// stop all processing since we hit an error
			return fmt.Errorf("error during include config conversion to declarative config: %v", err)
		}

		// Set up action diff for final declarative config rendering
		diffAction.HeadsOnly = true
		diffAction.IncludeConfig = dic
		dc, err = diffAction.Run(ctx)
		if err != nil {
			// stop all processing since we hit an error
			return err
		}

		if err := o.verifyDC(dic, dc); err != nil {
			// stop all processing since we hit an error
			return err
		}

		// update the local catalogMetadata value, and update the map with this copy
		catalogMetadata.dc = dc
		catalogMetadata.ic = ic
		digestsToProcess[platformKey] = catalogMetadata
	}
	return nil
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

/*
plan determines the source -> destination mapping for images associated with the provided catalog

# Arguments

• ctx: A cancellation context

• renderResultsPerPlatform: platform -> catalog metadata mapping for the ctlgRef argument

• ctlgRef: this is the source catalog reference

• targetCtlg: this is the target catalog reference

# Return

• image.TypedImageMapping: the source -> destination image mapping for images found during planning

• error: non-nil if an error occurs, nil otherwise
*/
func (o *OperatorOptions) plan(ctx context.Context, renderResultsPerPlatform map[OperatorCatalogPlatform]CatalogMetadata, ctlgRef, targetCtlg image.TypedImageReference) (image.TypedImageMapping, error) {

	o.Logger.Debugf("Mirroring catalog %q bundle and related images", ctlgRef.Ref.Exact())

	if !o.SkipImagePin {
		resolver, err := containerdregistry.NewResolver("", o.SourceSkipTLS, o.SourcePlainHTTP, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating image resolver: %v", err)
		}
		if err := o.pinImages(ctx, renderResultsPerPlatform, resolver); err != nil {
			return nil, fmt.Errorf("error pinning images in catalog %s: %v", ctlgRef, err)
		}
	}

	indexDirectories, err := o.writeConfigs(renderResultsPerPlatform, targetCtlg.Ref)
	if err != nil {
		return nil, err
	}

	// we should have one or more index directories to process
	// i.e. a single directory if we're dealing with a single architecture catalog
	// and multiple directories for multi architecture catalogs
	allMappings := image.TypedImageMapping{}
	for _, indexDir := range indexDirectories {
		opts, err := o.newMirrorCatalogOptions(ctlgRef.Ref, filepath.Join(o.Dir, config.SourceDir))
		if err != nil {
			return nil, err
		}
		if ctlgRef.Type != image.DestinationOCI {
			// Create the mapping file, but don't mirror quite yet.
			// Since the file-based catalog (declarative config) needs to be rebuilt
			// after rendering with the existing image in the publishing step,
			// we can just build the new image once then.
			opts.ManifestOnly = true
			opts.ImageMirrorer = catalog.ImageMirrorerFunc(func(mapping map[imagesource.TypedImageReference]imagesource.TypedImageReference) error {
				return nil
			})
			opts.IndexPath = indexDir

			opts.IndexExtractor = catalog.IndexExtractorFunc(func(imagesource.TypedImageReference) (string, error) {
				o.Logger.Debugf("returning index dir in extractor: %s", indexDir)
				return indexDir, nil
			})

			opts.RelatedImagesParser = catalog.RelatedImagesParserFunc(parseRelatedImages)

			opts.MaxICSPSize = icspSizeLimit
			opts.SourceRef = imagesource.TypedImageReference{
				Ref:  ctlgRef.Ref,
				Type: ctlgRef.Type,
			}
			opts.DestRef = imagesource.TypedImageReference{
				Type: imagesource.DestinationFile,
			}

			if err := opts.Validate(); err != nil {
				return nil, fmt.Errorf("invalid catalog mirror options: %v", err)
			}

			if err := opts.Run(); err != nil {
				return nil, fmt.Errorf("error running catalog mirror: %v", err)
			}
		} else {
			repo := ctlgRef.Ref.Name

			artifactsPath := artifactsFolderName

			operatorCatalog := v1alpha2.TrimProtocol(ctlgRef.OCIFBCPath)

			// check for the valid config label to use
			configsLabel, err := o.GetCatalogConfigPath(ctx, operatorCatalog)
			if err != nil {

				return nil, fmt.Errorf("unable to retrieve configs layer for image %s:\n%v\nMake sure this catalog is in OCI format", operatorCatalog, err)
			}
			// initialize path starting with <current working directory>/olm_artifacts/<repo>
			catalogContentsDir := filepath.Join(artifactsPath, repo)
			// initialize path where we assume the catalog config dir is <current working directory>/olm_artifacts/<repo>/<config folder>
			ctlgConfigDir := filepath.Join(catalogContentsDir, configsLabel)

			// TODO: this merged code needs to be addressed in some fashion
			// the renderResultsPerPlatform has the original location where the catalog came from, ctlgConfigDir is hardcoded to single location
			// and needs to be made multi arch aware or integrated differently

			// get all related images for every catalog dir
			var relatedImages []declcfg.RelatedImage
			for _, catalogMetaData := range renderResultsPerPlatform {
				currentRelatedImages, err := getRelatedImages(ctlgConfigDir, catalogMetaData.ic.Packages)
				if err != nil {
					return nil, err
				}
				relatedImages = append(relatedImages, currentRelatedImages...)
			}

			// place related images into the workspace - aka mirrorToDisk
			// TODO this should probably be done only if artifacts have not been copied
			result := image.TypedImageMapping{}
			// create mappings for the related images that will moved from the workspace to the final destination
			for _, i := range relatedImages {
				// intentionally removed the usernamespace from the call, because mirror.go is going to add it back!!
				err := o.addRelatedImageToMapping(ctx, result, i, o.ToMirror, "")
				if err != nil {
					return nil, err
				}

			}
			if err := writeMappingFile(mappingFile, result); err != nil {
				return nil, err
			}
		}

		// FIXME: There's no reason why the catalog.MirrorCatalogOptions could not store a reference
		// to its own mapping results, that way this re-reading of the mapping data from a file
		// would not have to happen. I am sure this was done as a path of least resistance to avoid
		// making changes in oc project, but this could be optimized if desired
		mappingFile := filepath.Join(opts.ManifestDir, mappingFile)
		mappings, err := image.ReadImageMapping(mappingFile, "=", v1alpha2.TypeOperatorBundle)
		if err != nil {
			return nil, err
		}

		// Remove the catalog image from mappings we are going to transfer this
		// using an OCI layout.
		var ctlgImg image.TypedImage
		if ctlgRef.Type == "oci" {
			ctlgImg, err = image.ParseTypedImage(ctlgRef.OCIFBCPath, v1alpha2.TypeOperatorBundle)
			if err != nil {
				return nil, err
			}
		} else {
			ctlgImg, err = image.ParseTypedImage(ctlgRef.Ref.Exact(), v1alpha2.TypeOperatorBundle)
			if err != nil {
				return nil, err
			}
		}

		mappings.Remove(ctlgImg)
		// Write catalog OCI layout file to src so it is included in the archive
		// at a path unique to the image.
		if ctlgRef.Type != image.DestinationOCI {
			if err := o.writeLayout(ctx, ctlgRef.Ref, targetCtlg.Ref); err != nil {
				return nil, err
			}
		} else {
			ctlgDir, err := operator.GenerateCatalogDir(targetCtlg.Ref)
			if err != nil {
				return nil, err
			}
			layoutDir := filepath.Join(o.Dir, config.SourceDir, config.CatalogsDir, ctlgDir, config.LayoutsDir)
			if err := os.MkdirAll(layoutDir, os.ModePerm); err != nil {
				return nil, fmt.Errorf("error catalog layout dir: %v", err)
			}
			if err := copy.Copy(v1alpha2.TrimProtocol(ctlgRef.OCIFBCPath), layoutDir); err != nil {
				return nil, fmt.Errorf("error copying oci fbc catalog to layout directory: %v", err)
			}
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
		// save mappings for later
		allMappings.Merge(mappings)
	}

	return allMappings, validateMapping(renderResultsPerPlatform, allMappings)
}

// validateMapping will search for bundle and related images in mapping
// and log a warning if an image does not exist and will not be mirrored
func validateMapping(renderResultsPerPlatform map[OperatorCatalogPlatform]CatalogMetadata, mapping image.TypedImageMapping) error {
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

	for platform, catalogMetadata := range renderResultsPerPlatform {
		platformErrorMessage := platform.ErrorMessagePrefix()
		for _, b := range catalogMetadata.dc.Bundles {
			if err := validateFunc(b.Image); err != nil {
				errs = append(errs, fmt.Errorf("%sbundle %q image %q: %v", platformErrorMessage, b.Name, b.Image, err))
				continue
			}
			for _, relatedImg := range b.RelatedImages {
				if err := validateFunc(relatedImg.Image); err != nil {
					errs = append(errs, fmt.Errorf("%sbundle %q related image %q: %v", platformErrorMessage, b.Name, relatedImg.Name, err))
					continue
				}
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}

// pinImages resolves every image in dc to it's canonical name (includes digest).
func (o *OperatorOptions) pinImages(ctx context.Context, renderResultsPerPlatform map[OperatorCatalogPlatform]CatalogMetadata, resolver remotes.Resolver) (err error) {

	var errs []error
	for platform, catalogMetadata := range renderResultsPerPlatform {
		platformErrorMessage := platform.ErrorMessagePrefix()
		// Check that declarative config is not nil
		// to avoid panics
		if err := validate(catalogMetadata.dc); err != nil {
			return err
		}
		// Instead of returning an error, just log it.
		isSkipErr := func(err error) bool {
			return o.ContinueOnError || (o.SkipMissing && errors.Is(err, errdefs.ErrNotFound))
		}

		for i, b := range catalogMetadata.dc.Bundles {

			if !image.IsImagePinned(b.Image) {
				klog.Warningf("%sbundle %s: pinning bundle image %s to digest", platformErrorMessage, b.Name, b.Image)

				if !image.IsImageTagged(b.Image) {
					klog.Warningf("%sbundle %s: bundle image tag not set", platformErrorMessage, b.Name)
					continue
				}
				if catalogMetadata.dc.Bundles[i].Image, err = image.ResolveToPin(ctx, resolver, b.Image); err != nil {
					if isSkipErr(err) {
						klog.Warningf("%sskipping bundle %s image %s resolve error: %v", platformErrorMessage, b.Name, b.Image, err)
					} else {
						errs = append(errs, err)
					}
				}
			}
			for j, ri := range b.RelatedImages {
				if !image.IsImagePinned(ri.Image) {
					klog.Warningf("%sbundle %s: pinning related image %s to digest", platformErrorMessage, ri.Name, ri.Image)

					if !image.IsImageTagged(ri.Image) {
						klog.Warningf("%sbundle %s: related image tag not set", platformErrorMessage, b.Name)
						continue
					}

					if b.RelatedImages[j].Image, err = image.ResolveToPin(ctx, resolver, ri.Image); err != nil {
						if isSkipErr(err) {
							klog.Warningf("%sskipping bundle %s related image %s=%s resolve error: %v", platformErrorMessage, b.Name, ri.Name, ri.Image, err)
						} else {
							errs = append(errs, err)
						}
					}
				}
			}
		}
	}

	return utilerrors.NewAggregate(errs)
}

/*
writeLayout creates OCI layout on the file system by pulling the image from the ctlgRef argument

# Arguments

• ctx: A cancellation context

• ctlgRef: this is the source catalog reference

• targetCtlg: this is the target catalog reference

# Return

• error: non-nil if an error occurs, nil otherwise
*/
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

		// try to get the config file... does it have os/arch values?
		configFile, err := img.ConfigFile()
		if err != nil || configFile == nil || (configFile.Architecture == "" && configFile.OS == "") {
			o.Logger.Debugf("could not determine platform for catalog image %s, using linux/amd64 instead", ref.Name())
			// Default to amd64 architecture with no multi-arch image since we can't know for sure what this image is
			if err := layoutPath.AppendImage(img, layout.WithPlatform(v1.Platform{OS: "linux", Architecture: "amd64"})); err != nil {
				return err
			}
			return nil
		}

		// set the correct platform while appending the image
		if err := layoutPath.AppendImage(img, layout.WithPlatform(v1.Platform{OS: configFile.OS, Architecture: configFile.Architecture, Variant: configFile.Variant})); err != nil {
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

/*
writeConfigs will write the declarative and include configuration to disk in a directory generated by the catalog name.

plan determines the source -> destination mapping for images associated with the provided catalog

# Arguments

• renderResultsPerPlatform: platform -> catalog metadata mapping for the targetCtlg argument

• targetCtlg: this is the target catalog reference

# Return

• []string: one or more index directories i.e. a single directory if we're dealing with a single architecture catalog, and multiple directories for multi architecture catalogs

• error: non-nil if an error occurs, nil otherwise
*/
func (o *OperatorOptions) writeConfigs(renderResultsPerPlatform map[OperatorCatalogPlatform]CatalogMetadata, targetCtlg imgreference.DockerImageReference) ([]string, error) {

	indexDirectories := []string{}

	// Discover if the platforms provided are single/multi architecture.
	// As long as one platform is flagged as multi architecture, stop
	// processing and assume all entries should be treated as multi platform.
	isIndex := false
	for platform := range renderResultsPerPlatform {
		if platform.isIndex {
			isIndex = true
			break
		}
	}

	// common function to handle single/multi architecture catalogs
	createIndexAndIncludeFile := func(
		indexDir string, // location where the index directory should go
		includeConfigPath string, // location where the config file go
		platform OperatorCatalogPlatform, // the platform for this catalog (only needed for writing debug info)
		catalogMetadata CatalogMetadata, // the content to write to disk
	) error {

		if err := os.MkdirAll(indexDir, os.ModePerm); err != nil {
			return fmt.Errorf("error creating diff index dir: %v", err)
		}
		catalogIndexPath := filepath.Join(indexDir, "index.json")
		if isIndex {
			o.Logger.Debugf("writing target catalog %q for platform %s diff to %s", targetCtlg.Exact(), platform.String(), catalogIndexPath)
		} else {
			o.Logger.Debugf("writing target catalog %q diff to %s", targetCtlg.Exact(), catalogIndexPath)
		}

		indexFile, err := os.Create(catalogIndexPath)
		if err != nil {
			return fmt.Errorf("error creating diff index file: %v", err)
		}

		o.Logger.Debugf("writing target catalog %q include config to %s", targetCtlg.Exact(), includeConfigPath)

		includeFile, err := os.Create(includeConfigPath)
		if err != nil {
			return fmt.Errorf("error creating include config file: %v", err)
		}

		defer func() {
			if err := includeFile.Close(); err != nil {
				o.Logger.Error(err)
			}
			if err := indexFile.Close(); err != nil {
				o.Logger.Error(err)
			}
		}()

		if err := declcfg.WriteJSON(*catalogMetadata.dc, indexFile); err != nil {
			return fmt.Errorf("error writing diff catalog: %v", err)
		}

		if err := catalogMetadata.ic.Encode(includeFile); err != nil {
			return fmt.Errorf("error writing include config file: %v", err)
		}
		// success... remember this directory
		indexDirectories = append(indexDirectories, indexDir)
		return nil
	}

	// generate the folder structure unique to this catalog
	// e.g. foo.io/bar/baz/image/sha256:XXXX
	ctlgDir, err := operator.GenerateCatalogDir(targetCtlg)
	if err != nil {
		return indexDirectories, err
	}
	catalogBasePath := filepath.Join(o.Dir, config.SourceDir, config.CatalogsDir, ctlgDir)

	if isIndex {
		// create content... e.g.:
		// foo.io/bar/baz/image/sha256:XXXX/multi/<platform>/index/index.json
		// foo.io/bar/baz/image/sha256:XXXX/multi/<platform>/include-config.gob
		for platform, catalogMetdata := range renderResultsPerPlatform {
			err := createIndexAndIncludeFile(
				filepath.Join(catalogBasePath, config.MultiDir, platform.String(), config.IndexDir),
				filepath.Join(catalogBasePath, config.MultiDir, platform.String(), config.IncludeConfigFile),
				platform,
				catalogMetdata,
			)
			if err != nil {
				return indexDirectories, err
			}
		}
	} else {
		// There should only be one platform entry for a single architecture catalog.
		// If this is not true, then bail out since something is wrong
		if len(renderResultsPerPlatform) != 1 {
			return indexDirectories, fmt.Errorf("unexpected number of platforms for target catalog %q", targetCtlg.Exact())
		}

		// create content... e.g.:
		// foo.io/bar/baz/image/sha256:XXXX/index/index.json
		// foo.io/bar/baz/image/sha256:XXXX/include-config.gob
		// easiest way to get at the values is iterate... previous sanity check should ensure we only iterate once
		for platform, catalogMetdata := range renderResultsPerPlatform {
			err := createIndexAndIncludeFile(
				filepath.Join(catalogBasePath, config.IndexDir),
				filepath.Join(catalogBasePath, config.IncludeConfigFile),
				platform,
				catalogMetdata,
			)
			if err != nil {
				return indexDirectories, err
			}
			// "paranoid break" just in case
			break
		}
	}

	return indexDirectories, nil
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

// validate declarative config
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

func (o OperatorOptions) getOperatorCatalogRef(ctx context.Context, ref string) (string, error) {
	_, _, repo, _, _ := v1alpha2.ParseImageReference(ref)
	artifactsPath := artifactsFolderName
	operatorCatalog := v1alpha2.TrimProtocol(ref)
	// check for the valid config label to use
	configsLabel, err := o.GetCatalogConfigPath(ctx, operatorCatalog)
	if err != nil {
		return "", fmt.Errorf("unable to retrieve configs layer for image %s:\n%v\nMake sure this catalog is in OCI format", ref, err)
	}
	// initialize path starting with <current working directory>/olm_artifacts/<repo>
	catalogContentsDir := filepath.Join(artifactsPath, repo)
	// initialize path where we assume the catalog config dir is <current working directory>/olm_artifacts/<repo>/<config folder>
	ctlgRef := filepath.Join(catalogContentsDir, configsLabel)
	return ctlgRef, nil
}
