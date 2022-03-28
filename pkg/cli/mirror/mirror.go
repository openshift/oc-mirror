package mirror

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	imagemanifest "github.com/openshift/oc/pkg/cli/image/manifest"
	"github.com/openshift/oc/pkg/cli/image/mirror"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/bundle"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/cli/mirror/describe"
	"github.com/openshift/oc-mirror/pkg/cli/mirror/list"
	"github.com/openshift/oc-mirror/pkg/cli/mirror/version"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
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
		Use:   filepath.Base(os.Args[0]),
		Short: "Manage mirrors per user configuration",
		Long: templates.LongDesc(`
			Create and publish user-configured mirrors with
            a declarative configuration input.
		`),
		Example: templates.Examples(`
			# Mirror to a directory
			oc-mirror --config mirror-config.yaml file://mirror

			# Mirror to mirror publish
			oc-mirror --config mirror-config.yaml docker://localhost:5000

			# Publish a previously created mirror archive
			oc-mirror --from mirror_seq1_000000.tar docker://localhost:5000

			# Publish to a registry and add a top-level namespace
			oc-mirror --from mirror_seq1_000000.tar docker://localhost:5000/namespace
		`),
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
	case "docker":
		mirror, err := imagesource.ParseReference(ref)
		if err != nil {
			return err
		}
		o.ToMirror = mirror.Ref.Registry
		o.UserNamespace = mirror.Ref.AsRepository().RepositoryName()
		if mirror.Ref.ID != "" || mirror.Ref.Tag != "" {
			return fmt.Errorf("destination registry must consist of registry host and namespace(s) only")
		}
	default:
		return fmt.Errorf("unknown destination scheme %q", typStr)
	}

	if len(o.FilterOptions) == 0 {
		o.FilterOptions = []string{"amd64"}
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
	}

	var destInsecure bool
	if o.DestPlainHTTP || o.DestSkipTLS {
		destInsecure = true
	}

	// Attempt to login to registry
	// FIXME(jpower432): CheckPushPermissions is slated for deprecation
	// must replace with its replacement
	if len(o.ToMirror) > 0 {
		logrus.Infof("Checking push permissions for %s", o.ToMirror)
		ref := path.Join(o.ToMirror, o.UserNamespace, "oc-mirror")
		logrus.Debugf("Using image %s to check permissions", ref)
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

	var supportedArchs = map[string]struct{}{"amd64": {}, "ppc64le": {}, "s390x": {}}
	for _, arch := range o.FilterOptions {
		if _, ok := supportedArchs[arch]; !ok {
			return fmt.Errorf("architecture %q is not a supported release architecture", arch)
		}
	}

	return nil
}

func (o *MirrorOptions) Run(cmd *cobra.Command, f kcmdutil.Factory) (err error) {
	if o.OutputDir != "" {
		if err := os.MkdirAll(o.OutputDir, 0750); err != nil {
			return err
		}
	}

	var sourceInsecure bool
	if o.SourcePlainHTTP || o.SourceSkipTLS {
		sourceInsecure = true
	}
	var destInsecure bool
	if o.DestPlainHTTP || o.DestSkipTLS {
		destInsecure = true
	}

	cleanup := func() error {
		if !o.SkipCleanup {
			return os.RemoveAll(filepath.Join(o.Dir, config.SourceDir))
		}
		return nil
	}

	var mapping image.TypedImageMapping
	var meta v1alpha2.Metadata
	switch {
	case o.ManifestsOnly:
		logrus.Info("Not implemented yet")
	case len(o.OutputDir) > 0 && o.From == "":
		cfg, err := config.ReadConfig(o.ConfigPath)
		if err != nil {
			return err
		}

		if err := bundle.MakeCreateDirs(o.Dir); err != nil {
			return err
		}

		meta, mapping, err = o.Create(cmd.Context(), cfg)
		if err != nil {
			return err
		}

		prevAssociations, err := image.ConvertToAssociationSet(meta.PastAssociations)
		if err != nil {
			return err
		}
		if !o.IgnoreHistory {
			// Prune out old associations if applicable
			// Ignore history while transitioning to range could result
			// in unexpected behavior
			prevAssociations = removePreviouslyMirrored(mapping, prevAssociations)
			if len(mapping) == 0 {
				logrus.Infof("no new images detected, process stopping")
				return nil
			}
		}

		if o.DryRun {
			mappingPath := filepath.Join(o.Dir, mappingFile)
			logrus.Infof("Writing image mapping to %s", mappingPath)
			if err := image.WriteImageMapping(mapping, mappingPath); err != nil {
				return err
			}
			return cleanup()
		}

		// Mirror planned images
		if err := o.mirrorMappings(cfg, mapping, sourceInsecure); err != nil {
			return err
		}

		// Create and store associations
		assocDir := filepath.Join(o.Dir, config.SourceDir)
		assocs, errs := image.AssociateLocalImageLayers(assocDir, mapping)

		skipErr := func(err error) bool {
			ierr := &image.ErrInvalidImage{}
			cerr := &image.ErrInvalidComponent{}
			return errors.As(err, &ierr) || errors.As(err, &cerr)
		}

		if errs != nil {
			for _, e := range errs.Errors() {
				if err := o.checkErr(e, skipErr); err != nil {
					return err
				}
			}
		}

		// Pack the images set
		tmpBackend, err := o.Pack(cmd.Context(), prevAssociations, assocs, &meta, cfg.ArchiveSize)
		if err != nil {
			if errors.Is(err, ErrNoUpdatesExist) {
				logrus.Infof("no updates detected, process stopping")
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
			if err := metadata.SyncMetadata(cmd.Context(), tmpBackend, targetBackend); err != nil {
				return err
			}
		}
	case len(o.ToMirror) > 0 && len(o.From) > 0:
		// Publish from disk to registry
		// this takes care of syncing the metadata to the
		// registry backends and generating the CatalogSource
		mapping, err = o.Publish(cmd.Context())
		if err != nil {
			serr := &SequenceError{}
			if errors.As(err, &serr) {
				return fmt.Errorf(
					"error occurred during publishing, expecting imageset with prefix mirror_seq%d: %v",
					serr.wantSeq,
					err,
				)
			}
			return err
		}
		dir, err := o.createResultsDir()
		if err != nil {
			return err
		}
		if err := o.generateAllManifests(mapping, dir); err != nil {
			return err
		}
	case len(o.ToMirror) > 0 && len(o.ConfigPath) > 0:
		cfg, err := config.ReadConfig(o.ConfigPath)
		if err != nil {
			return err
		}
		if err := bundle.MakeCreateDirs(o.Dir); err != nil {
			return err
		}
		meta, mapping, err = o.Create(cmd.Context(), cfg)
		if err != nil {
			return err
		}
		// Change the destination to registry
		// TODO(jpower432): Investigate whether oc can produce
		// registry to registry mapping
		mapping.ToRegistry(o.ToMirror, o.UserNamespace)

		prevAssociations, err := image.ConvertToAssociationSet(meta.PastAssociations)
		if err != nil {
			return err
		}
		if !o.IgnoreHistory {
			// Prune out old associations if applicable
			// Ignore history while transitioning to range could result
			// in unexpected behavior
			prevAssociations = removePreviouslyMirrored(mapping, prevAssociations)
			if len(mapping) == 0 {
				logrus.Infof("no new images detected, process stopping")
				return nil
			}
		}

		if o.DryRun {
			mappingPath := filepath.Join(o.Dir, mappingFile)
			logrus.Infof("Writing image mapping to %s", mappingPath)
			if err := image.WriteImageMapping(mapping, mappingPath); err != nil {
				return err
			}
			return cleanup()
		}

		// Mirror planned images
		// TODO(jpower432): Investigate how to mirror to mirror and
		// specific source and dest TLS configuration
		if err := o.mirrorMappings(cfg, mapping, destInsecure); err != nil {
			return err
		}
		// Create associations
		assocs, errs := image.AssociateRemoteImageLayers(cmd.Context(), mapping, sourceInsecure, o.SkipVerification)
		skipErr := func(err error) bool {
			ierr := &image.ErrInvalidImage{}
			cerr := &image.ErrInvalidComponent{}
			return errors.As(err, &ierr) || errors.As(err, &cerr)
		}

		if errs != nil {
			for _, e := range errs.Errors() {
				if err := o.checkErr(e, skipErr); err != nil {
					return err
				}
			}
		}

		meta.PastMirror.Associations, err = image.ConvertFromAssociationSet(assocs)
		if err != nil {
			return err
		}
		prevAssociations.Merge(assocs)
		meta.PastAssociations, err = image.ConvertFromAssociationSet(prevAssociations)
		if err != nil {
			return err
		}

		dir, err := o.createResultsDir()
		if err != nil {
			return err
		}

		// process catalog FBC images
		if len(cfg.Mirror.Operators) > 0 {
			ctlgRefs, err := o.rebuildCatalogs(cmd.Context(), filepath.Join(o.Dir, config.SourceDir))
			if err != nil {
				return fmt.Errorf("error rebuilding catalog images from file-based catalogs: %v", err)
			}
			mapping.Merge(ctlgRefs)
		}
		// process Cincinnati graph data image
		if len(cfg.Mirror.Platform.Channels) > 0 && cfg.Mirror.Platform.Graph {
			graphRef, err := o.buildGraphImage(cmd.Context(), filepath.Join(o.Dir, config.SourceDir))
			if err != nil {
				return fmt.Errorf("error building cincinnati graph image: %v", err)
			}
			mapping.Merge(graphRef)
		}
		if err := o.generateAllManifests(mapping, dir); err != nil {
			return err
		}

		// Move release signatures into results dir
		srcSignaturePath := filepath.Join(o.Dir, config.SourceDir, config.ReleaseSignatureDir)
		dstSignaturePath := filepath.Join(dir, config.ReleaseSignatureDir)
		if err := os.Rename(srcSignaturePath, dstSignaturePath); err != nil {
			return err
		}
		logrus.Debugf("Moved any release signatures to %s", dir)

		// Move charts into results dir
		srcHelmPath := filepath.Join(o.Dir, config.SourceDir, config.HelmDir)
		dstHelmPath := filepath.Join(dir, config.HelmDir)
		if err := os.Rename(srcHelmPath, dstHelmPath); err != nil {
			return err
		}
		logrus.Debugf("Moved any downloaded Helm charts to %s", dir)
		// Sync metadata from disk to source and target backends
		if cfg.StorageConfig.IsSet() {
			sourceBackend, err := storage.ByConfig(o.Dir, cfg.StorageConfig)
			if err != nil {
				return err
			}
			metaImage := o.newMetadataImage(meta.Uid.String())
			targetCfg := v1alpha2.StorageConfig{
				Registry: &v1alpha2.RegistryConfig{
					ImageURL: metaImage,
					SkipTLS:  destInsecure,
				},
			}

			targetBackend, err := storage.ByConfig(o.Dir, targetCfg)
			if err != nil {
				return err
			}
			// Update source metadata
			err = metadata.UpdateMetadata(cmd.Context(), sourceBackend, &meta, filepath.Join(o.Dir, config.SourceDir), o.SourceSkipTLS, o.SourcePlainHTTP)
			if err != nil {
				return err
			}
			// Sync target metadata
			err = metadata.SyncMetadata(cmd.Context(), sourceBackend, targetBackend)
			if err != nil {
				return err
			}
		}
	}

	if o.continuedOnError {
		return fmt.Errorf("one or more errors occurred")
	}

	return cleanup()
}

// removePreviouslyMirrored will check if an image has been previously mirrored
// and remove it from the mapping if found. The new past associations are returned.
func removePreviouslyMirrored(images image.TypedImageMapping, prevDownloads image.AssociationSet) image.AssociationSet {

	newPrevious := image.AssociationSet{}

	for srcRef := range images {
		// The skip-image-pin flag could create some unexpected behavior.
		// All keys need to specify image with digest.
		if found := prevDownloads.SetContainsKey(srcRef.Ref.String()); found {
			logrus.Debugf("skipping previously mirrored image %s", srcRef.Ref.String())
			images.Remove(srcRef)
			assoc, _ := prevDownloads.Search(srcRef.Ref.String())
			newPrevious.Add(srcRef.Ref.String(), assoc...)
		}
	}
	return newPrevious
}

// mirrorImage downloads individual images from an image mapping
func (o *MirrorOptions) mirrorMappings(cfg v1alpha2.ImageSetConfiguration, images image.TypedImageMapping, insecure bool) error {

	opts, err := o.newMirrorImageOptions(insecure)
	if err != nil {
		return err
	}

	// Create mapping from source and destination images
	var mappings []mirror.Mapping
	for srcRef, dstRef := range images {
		if bundle.IsBlocked(cfg.Mirror.BlockedImages, srcRef.Ref) {
			logrus.Warnf("skipping blocked image %s", srcRef.String())
			continue
		}

		mappings = append(mappings, mirror.Mapping{
			Source:      srcRef.TypedImageReference,
			Destination: dstRef.TypedImageReference,
			Name:        srcRef.Ref.Name,
		})
	}
	opts.Mappings = mappings
	if err := opts.Validate(); err != nil {
		return err
	}
	return o.checkErr(opts.Run(), nil)
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

func (o *MirrorOptions) generateAllManifests(mapping image.TypedImageMapping, dir string) error {

	allICSPs := []operatorv1alpha1.ImageContentSourcePolicy{}
	releases := image.ByCategory(mapping, v1alpha2.TypeOCPRelease, v1alpha2.TypeOCPReleaseContent)
	graphs := image.ByCategory(mapping, v1alpha2.TypeCincinnatiGraph)
	generic := image.ByCategory(mapping, v1alpha2.TypeGeneric)
	operator := image.ByCategory(mapping, v1alpha2.TypeOperatorBundle, v1alpha2.TypeOperatorCatalog)

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
				// are stored in the same repo
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

func (o *MirrorOptions) checkErr(err error, acceptableErr func(error) bool) error {

	if err == nil {
		return nil
	}

	var skip, skipAllTypes bool
	if acceptableErr != nil {
		skip = acceptableErr(err)
	} else {
		skipAllTypes = true
	}
	// Instead of returning an error, just log it.
	if o.ContinueOnError && (skip || skipAllTypes) {
		logrus.Warn(err)
		o.continuedOnError = true
	} else {
		return err
	}

	return nil
}
