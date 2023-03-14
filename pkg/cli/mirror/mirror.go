package mirror

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/opencontainers/go-digest"
	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	imagemanifest "github.com/openshift/oc/pkg/cli/image/manifest"
	"github.com/openshift/oc/pkg/cli/image/mirror"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/bundle"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/cli/mirror/describe"
	"github.com/openshift/oc-mirror/pkg/cli/mirror/initcmd"
	"github.com/openshift/oc-mirror/pkg/cli/mirror/list"
	"github.com/openshift/oc-mirror/pkg/cli/mirror/version"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

var (
	mirrorlongDesc = templates.LongDesc(
		` 
		Create and publish user-configured mirrors with a declarative configuration input.
		Accepts an argument defining the destination for the mirrored images using the prefix file:// for a local mirror packed into a 
		tar archive or docker:// for images to be streamed registry to registry without being stored locally. The default docker credentials are 
		used for authenticating to the registries. The podman location for credentials is also supported as a secondary location.
		When using file mirroring, the --from and --config flags control the location of the images to mirror. The --config flag accepts
		an imageset configuration file and the --from flag accepts the location of the imageset on disk. The --from input can be passed as a 
		file or directory, but must contain only one image sequence. The naming convention for an imageset is mirror\_seq<sequence number>\_<tar count>.tar.
		The location of the directory used by oc-mirror as a workspace defaults to the name oc-mirror-workspace. The location of this directory
		is outlined in the following: 
		1. Destination prefix is docker:// - The current working directory will be used.
		2. Destination prefix is file:// - The destination directory specified will be used.
		`,
	)
	mirrorExamples = templates.Examples(
		`
		# Mirror to a directory
		oc-mirror --config mirror-config.yaml file://mirror
		# Mirror to a directory without layer and image differential operations
		oc-mirror --config mirror-config.yaml file://mirror --ignore-history
		# Mirror to mirror publish
		oc-mirror --config mirror-config.yaml docker://localhost:5000
		# Publish a previously created mirror archive
		oc-mirror --from mirror_seq1_000000.tar docker://localhost:5000
		# Publish to a registry and add a top-level namespace
		oc-mirror --from mirror_seq1_000000.tar docker://localhost:5000/namespace
		# Generate manifests for previously created mirror archive
		oc-mirror --from mirror_seq1_000000.tar docker://localhost:5000/namespace --manifests-only
		# Skip metadata check during imageset publishing. This example shows a two-step process.
		# A differential imageset would have to be created with --ignore-history to be
		# successfully published with --skip-metadata-check.
		oc-mirror --config mirror-config.yaml file://mirror --ignore-history
		oc-mirror --from mirror_seq2_000000.tar docker://localhost:5000/namespace --skip-metadata-check
		`,
	)
)

func NewMirrorCmd() *cobra.Command {
	o := MirrorOptions{}
	o.RootOptions = &cli.RootOptions{
		IOStreams: genericclioptions.IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
	}

	// Configures a REST client getter factory from configs for mirroring releases.
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDiscoveryBurst(250)
	matchVersionKubeConfigFlags := kcmdutil.NewMatchVersionFlags(kubeConfigFlags)
	f := kcmdutil.NewFactory(matchVersionKubeConfigFlags)

	cmd := &cobra.Command{
		Use: fmt.Sprintf(
			"%s <destination type>:<destination location>",
			filepath.Base(os.Args[0]),
		),
		Short:             "Manage mirrors per user configuration",
		Long:              mirrorlongDesc,
		Example:           mirrorExamples,
		PersistentPreRun:  o.LogfilePreRun,
		PersistentPostRun: o.LogfilePostRun,
		Args:              cobra.MinimumNArgs(1),
		SilenceErrors:     false,
		SilenceUsage:      false,
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete(cmd, args))
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run(cmd, f))
		},
	}

	o.BindFlags(cmd.Flags())
	o.RootOptions.BindFlags(cmd.PersistentFlags())

	cmd.AddCommand(version.NewVersionCommand(f, o.RootOptions))
	cmd.AddCommand(list.NewListCommand(f, o.RootOptions))
	cmd.AddCommand(describe.NewDescribeCommand(f, o.RootOptions))
	cmd.AddCommand(initcmd.NewInitCommand(f, o.RootOptions))

	return cmd
}

func (o *MirrorOptions) Complete(cmd *cobra.Command, args []string) error {

	destination := args[0]
	splitIdx := strings.Index(destination, "://")
	if splitIdx == -1 {
		return fmt.Errorf("no scheme delimiter in destination argument")
	}
	typStr, ref := destination[:splitIdx], destination[splitIdx+3:]

	switch typStr {
	case "file":
		if cmd.Flags().Changed("dir") {
			return fmt.Errorf("--dir cannot be specified with file destination scheme")
		}
		ref = filepath.Clean(ref)
		if ref == "" {
			ref = "."
		}
		o.OutputDir = ref
		// If the destination is on disk, made the output dir the
		// parent dir for the workspace
		o.Dir = filepath.Join(o.OutputDir, o.Dir)
	case "oci":
		if cmd.Flags().Changed("dir") {
			return fmt.Errorf("--dir cannot be specified with oci destination scheme")
		}
		ref = filepath.Clean(ref)
		if ref == "" {
			ref = "."
		}
		o.OutputDir = ref
		// If the destination is on disk, made the output dir the
		// parent dir for the workspace
		o.Dir = filepath.Join(o.OutputDir, o.Dir)
	case "docker":
		mirror, err := imagesource.ParseReference(ref)
		if err != nil {
			return err
		}
		o.ToMirror = mirror.Ref.Registry
		o.UserNamespace = mirror.Ref.AsRepository().RepositoryName()
		err = checkDockerReference(mirror, o.MaxNestedPaths)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown destination scheme %q", typStr)
	}

	return nil
}

// checkDockerReference prints warnings or returns an error if applicable.
func checkDockerReference(mirror imagesource.TypedImageReference, nested int) error {
	switch {
	case mirror.Ref.Registry == "" && mirror.Ref.Namespace != "" && strings.Count(mirror.Ref.Name, "/") >= 1:
		klog.V(0).Info("The docker reference was parsed as a namespace and a repository name, not including a registry.")
		klog.V(0).Info("To specify a registry, use a qualified hostname.")
		klog.V(0).Info("For example, instead of docker://registry/namespace/repository, use docker://registry.localdomain/namespace/repository")
	case mirror.Ref.Registry == "" && mirror.Ref.Namespace != "" && mirror.Ref.Name != "":
		klog.V(0).Info("The docker reference was parsed as a namespace and name, not including a registry.")
		klog.V(0).Info("To specify a registry, use a qualified hostname.")
		klog.V(0).Info("For example, instead of docker://registry/repository, use docker://registry.localdomain/repository")
	case mirror.Ref.Registry == "" && mirror.Ref.Namespace == "" && mirror.Ref.Tag == "" && mirror.Ref.ID == "":
		klog.V(0).Info("The docker reference was parsed as a repository (or image) name, not a registry.")
		klog.V(0).Info("To specify a repository, use a qualified hostname.")
		klog.V(0).Info("For example, instead of docker://registry, use docker://registry.localdomain")
	case mirror.Ref.Registry == "" && (mirror.Ref.Tag != "" || mirror.Ref.ID != ""):
		klog.V(0).Info("The docker reference was parsed as image:tag, not as hostname:port.")
		klog.V(0).Info("To specify a registry, use a qualified hostname.")
		klog.V(0).Info("For example, instead of docker://registry:5000, use docker://registry.localdomain:5000")
	}
	if mirror.Ref.Registry == "" || mirror.Ref.Tag != "" || mirror.Ref.ID != "" {
		return fmt.Errorf("destination registry must consist of registry host and namespace(s) only, and must not include an image tag or ID")
	}

	depth := strings.Split(strings.Join([]string{mirror.Ref.Namespace, mirror.Ref.Name}, "/"), "/")
	if len(depth) > nested {
		destination := strings.Join([]string{mirror.Ref.Registry, mirror.Ref.Namespace, mirror.Ref.Name}, "/")
		return fmt.Errorf("the max-nested-paths value (%d) for %s exceeds the registry mirror paths setting (some registries limit the nested paths)", len(depth), destination)
	}
	return nil
}

func (o *MirrorOptions) Validate() error {
	switch {
	case len(o.From) > 0 && len(o.ToMirror) == 0:
		return fmt.Errorf("must specify a registry destination")
	case len(o.OutputDir) > 0 && len(o.ConfigPath) == 0:
		return fmt.Errorf("must specify a configuration file with --config")
	case len(o.ToMirror) > 0 && len(o.ConfigPath) == 0 && len(o.From) == 0:
		return fmt.Errorf("must specify --config or --from with registry destination")
	case o.ManifestsOnly && len(o.From) == 0:
		return fmt.Errorf("must specify a path to an archive with --from with --manifest-only")
	}

	var destInsecure bool
	if o.DestPlainHTTP || o.DestSkipTLS {
		destInsecure = true
	}

	// Attempt to login to registry
	// FIXME(jpower432): CheckPushPermissions is slated for deprecation
	// must replace with its replacement
	if len(o.ToMirror) > 0 && !o.ManifestsOnly {
		klog.Infof("Checking push permissions for %s", o.ToMirror)
		ref := path.Join(o.ToMirror, o.UserNamespace, "oc-mirror")
		klog.V(2).Infof("Using image %s to check permissions", ref)
		imgRef, err := name.ParseReference(ref, getNameOpts(destInsecure)...)
		if err != nil {
			return err
		}
		if err := remote.CheckPushPermission(imgRef, authn.DefaultKeychain, createRT(destInsecure)); err != nil {
			return fmt.Errorf("error checking push permissions for %s: %v", o.ToMirror, err)
		}
	}

	if len(o.From) > 0 {
		if _, err := os.Stat(o.From); err != nil {
			return err
		}
	}

	// Three mode options
	mirrorToDisk := len(o.OutputDir) > 0 && o.From == ""
	diskToMirror := len(o.ToMirror) > 0 && len(o.From) > 0
	mirrorToMirror := len(o.ToMirror) > 0 && len(o.ConfigPath) > 0

	// mirrorToDisk workflow is not supported with the oci feature
	if o.UseOCIFeature && mirrorToDisk {
		return fmt.Errorf("oci feature cannot be used when mirroring to local archive")
	}
	// diskToMirror workflow is not supported with the oci feature
	if o.UseOCIFeature && diskToMirror {
		return fmt.Errorf("oci feature cannot be used when publishing from a local archive to a registry")
	}
	// when oci flag is not set, ImageSetConfig should not contain any operators with oci:// prefix
	if !o.UseOCIFeature && mirrorToMirror {
		cfg, err := config.ReadConfig(o.ConfigPath)
		if err != nil {
			return fmt.Errorf("unable to read the configuration file provided with --config: %v", err)
		}
		for _, op := range cfg.Mirror.Operators {
			if op.IsFBCOCI() {
				return fmt.Errorf("use of OCI FBC catalogs (prefix oci://) in configuration file is authorized only with flag --use-oci-feature")
			}
		}
	}
	if !o.UseOCIFeature && len(o.OCIRegistriesConfig) > 0 {
		return fmt.Errorf("oci-registries-config flag can only be used with the --use-oci-feature flag")
	}

	if o.SkipPruning {
		klog.Infof("Skip pruning - N.B. this feature is unsupported")
	}

	return nil
}

type cleanupFunc func() error

func (o *MirrorOptions) Run(cmd *cobra.Command, f kcmdutil.Factory) (err error) {
	if o.OutputDir != "" {
		if err := os.MkdirAll(o.OutputDir, 0750); err != nil {
			return err
		}
	}

	cleanup := func() error {
		if !o.SkipCleanup {
			os.RemoveAll("olm_artifacts")
			return os.RemoveAll(filepath.Join(o.Dir, config.SourceDir))
		}
		return nil
	}

	return o.mirrorImages(cmd.Context(), cleanup)
}

func (o *MirrorOptions) mirrorImages(ctx context.Context, cleanup cleanupFunc) error {

	o.remoteRegFuncs = RemoteRegFuncs{
		newImageSource: func(ctx context.Context, sys *types.SystemContext, imgRef types.ImageReference) (types.ImageSource, error) {
			return imgRef.NewImageSource(ctx, sys)
		},
		getManifest: func(ctx context.Context, instanceDigest *digest.Digest, imgSrc types.ImageSource) ([]byte, string, error) {
			return imgSrc.GetManifest(ctx, instanceDigest)
		},
		handleMetadata: o.handleMetadata,
	}

	// Three mode options
	mirrorToDisk := len(o.OutputDir) > 0 && o.From == ""
	diskToMirror := len(o.ToMirror) > 0 && len(o.From) > 0
	mirrorToMirror := len(o.ToMirror) > 0 && len(o.ConfigPath) > 0

	switch {
	case o.ManifestsOnly:
		meta, err := bundle.ReadMetadataFromFile(ctx, o.From)
		if err != nil {
			return fmt.Errorf("error retrieving metadata from %q: %v", o.From, err)
		}

		mapping, err := image.ConvertToTypedMapping(meta.PastAssociations)
		if err != nil {
			return err
		}
		mapping.ToRegistry(o.ToMirror, o.UserNamespace)
		results, err := o.createResultsDir()
		if err != nil {
			return err
		}
		return o.generateResults(mapping, results)
	case mirrorToDisk:
		cfg, err := config.ReadConfig(o.ConfigPath)
		if err != nil {
			return err
		}

		return o.mirrorToDiskWrapper(ctx, cfg, cleanup)

	case diskToMirror:
		dir, err := o.createResultsDir()
		if err != nil {
			return err
		}
		o.OutputDir = dir
		return o.diskToMirrorWrapper(ctx, cleanup)

	case mirrorToMirror:

		cfg, err := config.ReadConfig(o.ConfigPath)
		if err != nil {
			return err
		}
		return o.mirrorToMirrorWrapper(ctx, cfg, cleanup)

	}
	if o.continuedOnError {
		return fmt.Errorf("one or more errors occurred")
	}

	return cleanup()
}

// removePreviouslyMirrored will check if an image has been previously mirrored
// and remove it from the mapping if found. These images are added to the current AssociationSet
// to maintain a history of images. Any images in the AssociationSet that was not requested in the mapping
// will be pruned from the history.
func (o *MirrorOptions) removePreviouslyMirrored(images image.TypedImageMapping, meta v1alpha2.Metadata) (image.AssociationSet, error) {
	prevDownloads, err := image.ConvertToAssociationSet(meta.PastAssociations)
	if err != nil {
		return image.AssociationSet{}, err
	}

	if o.IgnoreHistory {
		return prevDownloads, nil
	}

	var keep []string
	for srcRef := range images {
		// All keys need to specify image with digest.
		// Tagged images will need to be re-downloaded to
		// ensure their digests have not been updated.
		if srcRef.Ref.ID == "" {
			continue
		}
		if found := prevDownloads.SetContainsKey(srcRef.Ref.String()); found {
			klog.V(2).Infof("Skipping previously mirrored image %s", srcRef.Ref.String())
			images.Remove(srcRef)
			keep = append(keep, srcRef.Ref.String())
		}
	}

	prunedDownloads, err := image.Prune(prevDownloads, keep)
	if err != nil {
		return prunedDownloads, err
	}

	if len(images) == 0 {
		return image.AssociationSet{}, ErrNoUpdatesExist
	}

	return prunedDownloads, prunedDownloads.Validate()
}

// mirrorMappings downloads individual images from an image mapping.
func (o *MirrorOptions) mirrorMappings(cfg v1alpha2.ImageSetConfiguration, images image.TypedImageMapping, insecure bool) error {

	opts, err := o.newMirrorImageOptions(insecure)
	if err != nil {
		return err
	}

	var mappings []mirror.Mapping
	for srcRef, dstRef := range images {
		blocked, err := isBlocked(cfg.Mirror.BlockedImages, srcRef.Ref.Exact())
		if err != nil {
			return err
		}
		if blocked {
			klog.Warningf("skipping blocked image %s", srcRef.String())
			// Remove to make sure this does not end up in the metadata
			images.Remove(srcRef)
			continue
		}

		srcTIR := imagesource.TypedImageReference{
			Ref:  srcRef.Ref,
			Type: srcRef.Type,
		}

		dstTIR := imagesource.TypedImageReference{
			Ref:  dstRef.Ref,
			Type: dstRef.Type,
		}
		mappings = append(mappings, mirror.Mapping{
			Source:      srcTIR,
			Destination: dstTIR,
			Name:        srcRef.Ref.Name,
		})
	}
	opts.Mappings = mappings
	if err := opts.Validate(); err != nil {
		return err
	}
	return o.checkErr(opts.Run(), nil, nil)
}

func (o *MirrorOptions) newMirrorImageOptions(insecure bool) (*mirror.MirrorImageOptions, error) {
	opts := mirror.NewMirrorImageOptions(o.IOStreams)
	opts.SkipMissing = o.SkipMissing
	opts.ContinueOnError = o.ContinueOnError
	opts.DryRun = o.DryRun
	opts.FileDir = filepath.Join(o.Dir, config.SourceDir)
	opts.FromFileDir = o.From
	opts.SecurityOptions.Insecure = insecure
	opts.SecurityOptions.SkipVerification = o.SkipVerification
	opts.FilterOptions = imagemanifest.FilterOptions{FilterByOS: ".*"}
	opts.KeepManifestList = true
	opts.SkipMultipleScopes = true
	opts.ParallelOptions = imagemanifest.ParallelOptions{MaxPerRegistry: o.MaxPerRegistry}
	regctx, err := image.NewContext(o.SkipVerification)
	if err != nil {
		return opts, fmt.Errorf("error creating registry context: %v", err)
	}
	opts.SecurityOptions.CachedContext = regctx

	return opts, nil
}

// generateResults will generate a mapping.txt and allow applicable manifests and write
// the data to files in the specified directory.
func (o *MirrorOptions) generateResults(mapping image.TypedImageMapping, dir string) error {

	mappingResultsPath := filepath.Join(dir, mappingFile)
	if err := writeMappingFile(mappingResultsPath, mapping); err != nil {
		return err
	}

	allICSPs := []operatorv1alpha1.ImageContentSourcePolicy{}
	releases := image.ByCategory(mapping, v1alpha2.TypeOCPRelease, v1alpha2.TypeOCPReleaseContent)
	graphs := image.ByCategory(mapping, v1alpha2.TypeCincinnatiGraph)
	generic := image.ByCategory(mapping, v1alpha2.TypeGeneric)
	operator := image.ByCategory(mapping, v1alpha2.TypeOperatorBundle, v1alpha2.TypeOperatorCatalog, v1alpha2.TypeOperatorRelatedImage)

	getICSP := func(mapping image.TypedImageMapping, name string, builder ICSPBuilder) error {
		icsps, err := GenerateICSP(name, namespaceICSPScope, icspSizeLimit, mapping, builder)
		if err != nil {
			return fmt.Errorf("error generating ICSP manifests")
		}
		allICSPs = append(allICSPs, icsps...)
		return nil
	}

	if len(graphs) == 1 {
		releaseImages := image.ByCategory(releases, v1alpha2.TypeOCPRelease)
		if len(releaseImages) != 0 {
			for _, graph := range graphs {
				var release image.TypedImage
				// Just grab the first release image.
				// The value is used as a repo and all release images
				// are stored in the same repo.
				for _, v := range releaseImages {
					release = v
					break
				}
				if err := WriteUpdateService(release, graph, dir); err != nil {
					return err
				}
			}
		}

	}

	ctlgRefs := image.ByCategory(operator, v1alpha2.TypeOperatorCatalog)
	if len(ctlgRefs) != 0 {
		if err := WriteCatalogSource(ctlgRefs, dir); err != nil {
			return err
		}
	}

	if err := getICSP(releases, "release", &ReleaseBuilder{}); err != nil {
		return err
	}
	if err := getICSP(generic, "generic", &GenericBuilder{}); err != nil {
		return err
	}
	if err := getICSP(operator, "operator", &OperatorBuilder{}); err != nil {
		return err
	}

	return WriteICSPs(dir, allICSPs)
}

// moveToResults will move release signatures and helm charts to
// the specified results directory from the defined source directory
// in the config package.
func (o *MirrorOptions) moveToResults(resultsDir string) error {

	resultsDir = filepath.Clean(resultsDir)

	srcSignaturePath := filepath.Join(o.Dir, config.SourceDir, config.ReleaseSignatureDir)
	dstSignaturePath := filepath.Join(resultsDir, config.ReleaseSignatureDir)
	if err := os.Rename(srcSignaturePath, dstSignaturePath); err != nil {
		return err
	}
	klog.V(1).Infof("Moved any release signatures to %s", resultsDir)

	// Move charts into results dir
	srcHelmPath := filepath.Join(o.Dir, config.SourceDir, config.HelmDir)
	dstHelmPath := filepath.Join(resultsDir, config.HelmDir)
	if err := os.Rename(srcHelmPath, dstHelmPath); err != nil {
		return err
	}
	klog.V(1).Infof("Moved any downloaded Helm charts to %s", resultsDir)
	return nil
}

func (o *MirrorOptions) processAssociationErrors(errs []error) error {
	if errs == nil {
		return nil
	}
	skipErr := func(err error) bool {
		ierr := &image.ErrInvalidImage{}
		cerr := &image.ErrInvalidComponent{}
		return errors.As(err, &ierr) || errors.As(err, &cerr)
	}
	ierr := &image.ErrInvalidImage{}
	for _, e := range errs {
		if o.SkipMissing && errors.As(e, &ierr) {
			klog.V(1).Infof("warning: skipping image: %v", e)
			continue
		}
		if err := o.checkErr(e, skipErr, nil); err != nil {
			return err
		}
	}
	return nil
}

func writeMappingFile(mappingPath string, mapping image.TypedImageMapping) error {
	path := filepath.Clean(mappingPath)
	mappingFile, err := os.Create(path)
	if err != nil {
		return err
	}
	defer mappingFile.Close()
	klog.Infof("Writing image mapping to %s", mappingPath)
	if err := image.WriteImageMapping(mapping, mappingFile); err != nil {
		return err
	}
	return mappingFile.Sync()
}

func (o *MirrorOptions) mirrorToMirrorWrapper(ctx context.Context, cfg v1alpha2.ImageSetConfiguration, cleanup cleanupFunc) error {
	destInsecure := o.DestPlainHTTP || o.DestSkipTLS

	mappingPath := filepath.Join(o.Dir, mappingFile)

	if err := bundle.MakeWorkspaceDirs(o.Dir); err != nil {
		return err
	}
	meta, mapping, err := o.Create(ctx, cfg)
	if err != nil {
		return err
	}

	// Imageset sequence check
	metaImage := o.newMetadataImage(meta.Uid.String())
	targetCfg := &v1alpha2.RegistryConfig{
		ImageURL: metaImage,
		SkipTLS:  destInsecure,
	}

	targetBackend, err := storage.NewRegistryBackend(targetCfg, o.Dir)
	if err != nil {
		return err
	}
	if !o.UseOCIFeature {
		var curr v1alpha2.Metadata
		berr := targetBackend.ReadMetadata(ctx, &curr, config.MetadataBasePath)
		if err := o.checkSequence(meta, curr, berr); err != nil {
			return err
		}
	}

	// Change the destination to registry
	// TODO(jpower432): Investigate whether oc can produce
	// registry to registry mapping
	mapping.ToRegistry(o.ToMirror, o.UserNamespace)

	prunedAssociations, err := o.removePreviouslyMirrored(mapping, meta)
	if err != nil {
		if errors.Is(err, ErrNoUpdatesExist) {
			klog.Infof("No new images detected, process stopping")
			return nil
		}
		return err
	}

	// QUESTION(jpower432): Can you specify different TLS configuration for source
	// and destination with `oc image mirror`?
	if err := o.mirrorMappings(cfg, mapping, destInsecure); err != nil {
		return err
	}

	prevAssociations, err := image.ConvertToAssociationSet(meta.PastAssociations)
	if err != nil {
		return err
	}

	if o.DryRun {
		if err := writeMappingFile(mappingPath, mapping); err != nil {
			return err
		}
		if err := o.outputPruneImagePlan(ctx, prevAssociations, prunedAssociations); err != nil {
			return err
		}
		return cleanup()
	}

	assocs, errs := image.AssociateRemoteImageLayers(ctx, mapping, o.SourceSkipTLS, o.SourcePlainHTTP, o.SkipVerification)
	if errs != nil {
		if err := o.processAssociationErrors(errs.Errors()); err != nil {
			return err
		}
	}

	// Prune the images that differ between the previous Associations and the
	// pruned Associations.
	meta.PastMirror.Associations, err = image.ConvertFromAssociationSet(assocs)
	if err != nil {
		return err
	}
	prunedAssociations.Merge(assocs)

	if err := o.pruneRegistry(ctx, prevAssociations, prunedAssociations); err != nil {
		return fmt.Errorf("error pruning from registry %q: %v", o.ToMirror, err)
	}

	meta.PastAssociations, err = image.ConvertFromAssociationSet(prunedAssociations)
	if err != nil {
		return err
	}

	dir, err := o.createResultsDir()
	if err != nil {
		return err
	}
	// process catalog FBC images
	if len(cfg.Mirror.Operators) > 0 {
		ctlgRefs, err := o.rebuildCatalogs(ctx, filepath.Join(o.Dir, config.SourceDir))
		if err != nil {
			return fmt.Errorf("error rebuilding catalog images from file-based catalogs: %v", err)
		}
		mapping.Merge(ctlgRefs)
	}
	// process Cincinnati graph data image
	if len(cfg.Mirror.Platform.Channels) > 0 {
		if cfg.Mirror.Platform.Graph {
			graphRef, err := o.buildGraphImage(ctx, filepath.Join(o.Dir, config.SourceDir))
			if err != nil {
				return fmt.Errorf("error building cincinnati graph image: %v", err)
			}
			mapping.Merge(graphRef)
		}
	}

	if err := o.generateResults(mapping, dir); err != nil {
		return err
	}

	if err := o.moveToResults(dir); err != nil {
		return err
	}

	// Sync metadata from disk to source and target backends
	if cfg.StorageConfig.IsSet() {
		sourceBackend, err := storage.ByConfig(o.Dir, cfg.StorageConfig)
		if err != nil {
			return err
		}
		workspace := filepath.Join(o.Dir, config.SourceDir)
		if err = metadata.UpdateMetadata(ctx, sourceBackend, &meta, workspace, o.SourceSkipTLS, o.SourcePlainHTTP); err != nil {
			return err
		}
		if err := metadata.SyncMetadata(ctx, sourceBackend, targetBackend); err != nil {
			return err
		}
	}
	return cleanup()
}

// mirrorToDiskWrapper
func (o *MirrorOptions) mirrorToDiskWrapper(ctx context.Context, cfg v1alpha2.ImageSetConfiguration, cleanup cleanupFunc) error {
	sourceInsecure := o.SourcePlainHTTP || o.SourceSkipTLS

	if err := bundle.MakeWorkspaceDirs(o.Dir); err != nil {
		return err
	}

	meta, mapping, err := o.Create(ctx, cfg)
	if err != nil {
		return err
	}

	prunedAssociations, err := o.removePreviouslyMirrored(mapping, meta)
	if err != nil {
		if errors.Is(err, ErrNoUpdatesExist) {
			klog.Infof("No new images detected, process stopping")
			return nil
		}
		return err
	}

	if err := o.mirrorMappings(cfg, mapping, sourceInsecure); err != nil {
		return err
	}

	mappingPath := filepath.Join(o.Dir, mappingFile)
	if o.DryRun {
		if err := writeMappingFile(mappingPath, mapping); err != nil {
			return err
		}
		return cleanup()
	}

	// Create and store associations
	assocDir := filepath.Join(o.Dir, config.SourceDir)
	assocs, errs := image.AssociateLocalImageLayers(assocDir, mapping)
	if errs != nil {
		if err := o.processAssociationErrors(errs.Errors()); err != nil {
			return err
		}
	}

	// Pack the images set
	tmpBackend, err := o.Pack(ctx, prunedAssociations, assocs, &meta, cfg.ArchiveSize)
	if err != nil {
		if errors.Is(err, ErrNoUpdatesExist) {
			klog.Infof("No updates detected, process stopping")
			return nil
		}
		return err
	}

	// Sync metadata from temporary backend to target backend
	if cfg.StorageConfig.IsSet() {
		targetBackend, err := storage.ByConfig(o.Dir, cfg.StorageConfig)
		if err != nil {
			return err
		}
		if err := metadata.SyncMetadata(ctx, tmpBackend, targetBackend); err != nil {
			return err
		}
	}
	return nil
}

func (o *MirrorOptions) diskToMirrorWrapper(ctx context.Context, cleanup cleanupFunc) error {
	// Publish from disk to registry
	// this takes care of syncing the metadata to the
	// registry backends.
	mapping, err := o.Publish(ctx)
	if err != nil {
		serr := &ErrInvalidSequence{}
		if errors.As(err, &serr) {
			return fmt.Errorf("error during publishing, expecting imageset with prefix mirror_seq%d: %v", serr.wantSeq, err)
		}
		return err
	}

	mappingPath := filepath.Join(o.Dir, mappingFile)
	if o.DryRun {
		if err := writeMappingFile(mappingPath, mapping); err != nil {
			return err
		}
		return cleanup()
	}

	if err := o.generateResults(mapping, o.OutputDir); err != nil {
		return err
	}
	return nil
}
