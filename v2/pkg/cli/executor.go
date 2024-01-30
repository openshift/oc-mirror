package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k8s.io/kubectl/pkg/util/templates"

	"github.com/distribution/distribution/v3/configuration"
	dcontext "github.com/distribution/distribution/v3/context"
	"github.com/distribution/distribution/v3/registry"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	distversion "github.com/distribution/distribution/v3/version"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/openshift/oc-mirror/v2/pkg/additional"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/archive"
	"github.com/openshift/oc-mirror/v2/pkg/batch"
	"github.com/openshift/oc-mirror/v2/pkg/clusterresources"
	"github.com/openshift/oc-mirror/v2/pkg/config"
	"github.com/openshift/oc-mirror/v2/pkg/image"
	"github.com/openshift/oc-mirror/v2/pkg/imagebuilder"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/pkg/operator"
	"github.com/openshift/oc-mirror/v2/pkg/release"
	"github.com/spf13/cobra"
)

const (
	dockerProtocol          string = "docker://"
	ociProtocol             string = "oci://"
	dirProtocol             string = "dir://"
	fileProtocol            string = "file://"
	releaseImageDir         string = "release-images"
	logsDir                 string = "logs"
	workingDir              string = "working-dir"
	cacheRelativePath       string = ".oc-mirror/.cache"
	cacheEnvVar             string = "OC_MIRROR_CACHE"
	additionalImages        string = "additional-images"
	releaseImageExtractDir  string = "hold-release"
	operatorImageExtractDir string = "hold-operator"
	signaturesDir           string = "signatures"
	registryLogFilename     string = "registry.log"
	startMessage            string = "starting local storage on localhost:%v"
)

var (
	mirrorlongDesc = templates.LongDesc(
		` 
		Create and publish user-configured mirrors with a declarative configuration input.
		used for authenticating to the registries. 

		The podman location for credentials is also supported as a secondary location.

		1. Destination prefix is docker:// - The current working directory will be used.
		2. Destination prefix is oci:// - The destination directory specified will be used.

		`,
	)
	mirrorExamples = templates.Examples(
		`
		# Mirror to a directory
		oc-mirror oci:mirror --config mirror-config.yaml
		`,
	)
)

type ExecutorSchema struct {
	Log                          clog.PluggableLoggerInterface
	LogsDir                      string
	registryLogFile              *os.File
	Config                       v1alpha2.ImageSetConfiguration
	Opts                         mirror.CopyOptions
	Operator                     operator.CollectorInterface
	Release                      release.CollectorInterface
	AdditionalImages             additional.CollectorInterface
	Mirror                       mirror.MirrorInterface
	Manifest                     manifest.ManifestInterface
	Batch                        batch.BatchInterface
	LocalStorageService          registry.Registry
	localStorageInterruptChannel chan error
	LocalStorageFQDN             string
	LocalStorageDisk             string
	ClusterResources             clusterresources.GeneratorInterface
	ImageBuilder                 imagebuilder.ImageBuilderInterface
	MirrorArchiver               archive.Archiver
	MirrorUnArchiver             archive.UnArchiver
	MakeDir                      MakeDirInterface
}

type MakeDirInterface interface {
	makeDirAll(string, os.FileMode) error
}

type MakeDir struct {
}

func (o MakeDir) makeDirAll(dir string, mode os.FileMode) error {
	return os.MkdirAll(dir, mode)
}

// NewMirrorCmd - cobra entry point
func NewMirrorCmd(log clog.PluggableLoggerInterface) *cobra.Command {

	global := &mirror.GlobalOptions{
		TlsVerify:    false,
		SecurePolicy: false,
	}

	flagSharedOpts, sharedOpts := mirror.SharedImageFlags()
	flagDepTLS, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	flagSrcOpts, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	flagDestOpts, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	flagRetryOpts, retryOpts := mirror.RetryFlags()

	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Dev:                 false,
	}

	mkd := MakeDir{}
	ex := &ExecutorSchema{
		Log:     log,
		Opts:    opts,
		MakeDir: mkd,
	}

	cmd := &cobra.Command{
		Use:           fmt.Sprintf("%v <destination type>:<destination location>", filepath.Base(os.Args[0])),
		Version:       "v2.0.0-dev-01",
		Short:         "Manage mirrors per user configuration",
		Long:          mirrorlongDesc,
		Example:       mirrorExamples,
		Args:          cobra.MinimumNArgs(1),
		SilenceErrors: false,
		SilenceUsage:  false,
		Run: func(cmd *cobra.Command, args []string) {
			err := ex.Validate(args)
			if err != nil {
				log.Error("%v ", err)
				os.Exit(1)
			}
			err = ex.Complete(args)
			if err != nil {
				log.Error(" %v ", err)
				os.Exit(1)
			}
			// prepare internal storage
			err = ex.setupLocalStorage()
			if err != nil {
				log.Error(" %v ", err)
				os.Exit(1)
			}

			err = ex.Run(cmd, args)
			if err != nil {
				log.Error("%v ", err)
				os.Exit(1)
			}
		},
	}
	cmd.AddCommand(NewPrepareCommand(log))
	cmd.PersistentFlags().StringVarP(&opts.Global.ConfigPath, "config", "c", "", "Path to imageset configuration file")
	cmd.Flags().StringVar(&opts.Global.LogLevel, "loglevel", "info", "Log level one of (info, debug, trace, error)")
	cmd.Flags().StringVar(&opts.Global.WorkingDir, "dir", workingDir, "Assets directory")
	cmd.Flags().StringVar(&opts.Global.From, "from", "", "local storage directory for disk to mirror workflow")
	cmd.Flags().Uint16VarP(&opts.Global.Port, "port", "p", 55000, "HTTP port used by oc-mirror's local storage instance")
	cmd.Flags().BoolVarP(&opts.Global.Quiet, "quiet", "q", false, "enable detailed logging when copying images")
	cmd.Flags().BoolVarP(&opts.Global.Force, "force", "f", false, "force the copy and mirror functionality")
	cmd.Flags().BoolVar(&opts.Global.V2, "v2", opts.Global.V2, "Redirect the flow to oc-mirror v2 - PLEASE DO NOT USE that. V2 is still under development and it is not ready to be used.")
	cmd.Flags().BoolVar(&opts.Global.SecurePolicy, "secure-policy", opts.Global.SecurePolicy, "If set (default is false), will enable signature verification (secure policy for signature verification).")
	cmd.Flags().IntVar(&opts.Global.MaxNestedPaths, "max-nested-paths", 0, "Number of nested paths, for destination registries that limit nested paths")
	// nolint: errcheck
	cmd.Flags().MarkHidden("v2")
	cmd.Flags().AddFlagSet(&flagSharedOpts)
	cmd.Flags().AddFlagSet(&flagRetryOpts)
	cmd.Flags().AddFlagSet(&flagDepTLS)
	cmd.Flags().AddFlagSet(&flagSrcOpts)
	cmd.Flags().AddFlagSet(&flagDestOpts)
	return cmd
}

// Validate - cobra validation
func (o ExecutorSchema) Validate(dest []string) error {
	if len(o.Opts.Global.ConfigPath) == 0 {
		return fmt.Errorf("use the --config flag it is mandatory")
	}
	if strings.Contains(dest[0], dockerProtocol) && o.Opts.Global.From == "" {
		return fmt.Errorf("when destination is docker://, diskToMirror workflow is assumed, and the --from argument is mandatory")
	}
	if strings.Contains(dest[0], fileProtocol) && o.Opts.Global.From != "" {
		return fmt.Errorf("when destination is file://, mirrorToDisk workflow is assumed, and the --from argument is not needed")
	}
	if len(o.Opts.Global.From) > 0 && !strings.Contains(o.Opts.Global.From, fileProtocol) {
		return fmt.Errorf("when --from is used, it must have file:// prefix")
	}
	if strings.Contains(dest[0], fileProtocol) || strings.Contains(dest[0], dockerProtocol) {
		return nil
	} else {
		return fmt.Errorf("destination must have either file:// (mirror to disk) or docker:// (diskToMirror) protocol prefixes")
	}
}

// Complete - do the final setup of modules
func (o *ExecutorSchema) Complete(args []string) error {

	o.Log.Debug("imagesetconfig file %s ", o.Opts.Global.ConfigPath)
	// read the ImageSetConfiguration
	cfg, err := config.ReadConfig(o.Opts.Global.ConfigPath)
	if err != nil {
		return err
	}
	o.Log.Trace("imagesetconfig : %v ", cfg)

	// update all dependant modules
	mc := mirror.NewMirrorCopy()
	md := mirror.NewMirrorDelete()
	o.Manifest = manifest.New(o.Log)
	o.Mirror = mirror.New(mc, md)
	o.Config = cfg

	// logic to check mode
	var rootDir string
	if strings.Contains(args[0], fileProtocol) {
		o.Opts.Mode = mirror.MirrorToDisk
		rootDir = strings.TrimPrefix(args[0], fileProtocol)
		o.Log.Debug("destination %s ", rootDir)
	} else if strings.Contains(args[0], dockerProtocol) {
		rootDir = strings.TrimPrefix(o.Opts.Global.From, fileProtocol)
		o.Opts.Mode = mirror.DiskToMirror
	} else {
		o.Log.Error("unable to determine the mode (the destination must be either file:// or docker://)")
	}
	o.Opts.Destination = args[0]
	o.Opts.Global.WorkingDir = filepath.Join(rootDir, workingDir)
	o.Log.Info("mode %s ", o.Opts.Mode)

	// setup logs level, and logsDir under workingDir
	err = o.setupLogsLevelAndDir()
	if err != nil {
		return err
	}

	if o.isLocalStoragePortBound() {
		return fmt.Errorf("%d is already bound and cannot be used", o.Opts.Global.Port)
	}
	o.LocalStorageFQDN = "localhost:" + strconv.Itoa(int(o.Opts.Global.Port))

	err = o.setupWorkingDir()
	if err != nil {
		return err
	}

	err = o.setupLocalStorageDir()
	if err != nil {
		return err
	}

	client, _ := release.NewOCPClient(uuid.New())

	o.ImageBuilder = imagebuilder.NewBuilder(o.Log, o.Opts)

	signature := release.NewSignatureClient(o.Log, o.Config, o.Opts)
	cn := release.NewCincinnati(o.Log, &o.Config, o.Opts, client, false, signature)
	o.Release = release.New(o.Log, o.LogsDir, o.Config, o.Opts, o.Mirror, o.Manifest, cn, o.LocalStorageFQDN, o.ImageBuilder)
	o.Operator = operator.New(o.Log, o.LogsDir, o.Config, o.Opts, o.Mirror, o.Manifest, o.LocalStorageFQDN)
	o.AdditionalImages = additional.New(o.Log, o.Config, o.Opts, o.Mirror, o.Manifest, o.LocalStorageFQDN)
	o.ClusterResources = clusterresources.New(o.Log, o.Opts.Global.WorkingDir, o.Config)
	o.Batch = batch.New(o.Log, o.LogsDir, o.Mirror, o.Manifest)

	if o.Opts.IsMirrorToDisk() {
		o.MirrorArchiver, err = archive.NewMirrorArchive(&o.Opts, rootDir, o.Opts.Global.ConfigPath, o.Opts.Global.WorkingDir, o.LocalStorageDisk, o.Config.ImageSetConfigurationSpec.ArchiveSize, o.Log)
		if err != nil {
			return err
		}
	} else if o.Opts.IsDiskToMirror() { // if added so that the unArchiver is not instanciated for the prepare workflow
		o.MirrorUnArchiver, err = archive.NewArchiveExtractor(rootDir, o.Opts.Global.WorkingDir, o.LocalStorageDisk)
		if err != nil {
			return err
		}
	}
	return nil
}

// Run - start the mirror functionality
func (o *ExecutorSchema) Run(cmd *cobra.Command, args []string) error {

	// make sure we always get multi-arch images
	o.Opts.MultiArch = "all"
	var err error
	if o.Opts.IsMirrorToDisk() {
		err = o.RunMirrorToDisk(cmd, args)

	} else {
		err = o.RunDiskToMirror(cmd, args)

	}
	if err != nil {
		o.closeAll()
		return err
	}

	defer o.closeAll()
	return nil
}

// setupLocalStorage - private function that sets up
// a local (distribution) registry
func (o *ExecutorSchema) setupLocalStorage() error {

	// create config file for local registry
	// sonarqube scanner variable declaration convention
	configYamlV01 := `
version: 0.1
log:
  accesslog:
    disabled: $$PLACEHOLDER_ACCESS_LOG_OFF$$
  level: $$PLACEHOLDER_LOG_LEVEL$$
  formatter: text
  fields:
    service: registry
storage:
  cache:
    blobdescriptor: inmemory
  filesystem:
    rootdirectory: $$PLACEHOLDER_ROOT$$
http:
  addr: :$$PLACEHOLDER_PORT$$
  headers:
    X-Content-Type-Options: [nosniff]
      #auth:
      #htpasswd:
      #realm: basic-realm
      #path: /etc/registry
health:
  storagedriver:
    enabled: true
    interval: 10s
    threshold: 3
`

	if _, err := os.Stat(o.LocalStorageDisk); err != nil {
		// something went wrong
		return fmt.Errorf("error using the local storage folder for caching")
	}
	configYamlV01 = strings.Replace(configYamlV01, "$$PLACEHOLDER_ROOT$$", o.LocalStorageDisk, 1)
	configYamlV01 = strings.Replace(configYamlV01, "$$PLACEHOLDER_PORT$$", strconv.Itoa(int(o.Opts.Global.Port)), 1)
	configYamlV01 = strings.Replace(configYamlV01, "$$PLACEHOLDER_LOG_LEVEL$$", o.Opts.Global.LogLevel, 1)
	if o.Opts.Global.LogLevel == "debug" {
		configYamlV01 = strings.Replace(configYamlV01, "$$PLACEHOLDER_ACCESS_LOG_OFF$$", "false", 1)
	} else {
		configYamlV01 = strings.Replace(configYamlV01, "$$PLACEHOLDER_ACCESS_LOG_OFF$$", "true", 1)
	}

	config, err := configuration.Parse(bytes.NewReader([]byte(configYamlV01)))

	if err != nil {
		return fmt.Errorf("error parsing local storage configuration : %v\n %s", err, configYamlV01)
	}

	regLogger := logrus.New()
	// prepare the logger
	registryLogPath := filepath.Join(o.LogsDir, registryLogFilename)
	o.registryLogFile, err = os.Create(registryLogPath)
	if err != nil {
		regLogger.Warn("Failed to create log file for local storage registry, using default stderr")
	} else {
		regLogger.Out = o.registryLogFile
	}
	absPath, err := filepath.Abs(registryLogPath)
	o.Log.Info("local storage registry will log to %s", absPath)
	if err != nil {
		o.Log.Error(err.Error())
	}
	regLogEntry := logrus.NewEntry(regLogger)

	// setup the context
	dcontext.SetDefaultLogger(regLogEntry)
	ctx := dcontext.WithVersion(dcontext.Background(), distversion.Version)
	ctx = dcontext.WithLogger(ctx, regLogEntry)

	errchan := make(chan error)

	reg, err := registry.NewRegistry(ctx, config)
	if err != nil {
		return err
	}
	o.LocalStorageService = *reg
	o.localStorageInterruptChannel = errchan

	go panicOnRegistryError(errchan)
	return nil
}

// startLocalRegistry - private function to start the
// local registry
func startLocalRegistry(reg *registry.Registry, errchan chan error) {
	err := reg.ListenAndServe()
	errchan <- err
}

// panicOnRegistryError - handle errors from local registry
func panicOnRegistryError(errchan chan error) {
	err := <-errchan
	if err != nil && !errors.Is(err, &NormalStorageInterruptError{}) {
		panic(err)
	}
}

// isLocalStoragePortBound - private utility to check if port is bound
func (o *ExecutorSchema) isLocalStoragePortBound() bool {

	// Check if the port is already bound
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", o.Opts.Global.Port))
	if err != nil {
		return true
	}
	listener.Close()
	return false
}

// setupLocalStorageDir - private utility to setup
// the correct local storage directory
func (o *ExecutorSchema) setupLocalStorageDir() error {

	requestedCachePath := os.Getenv(cacheEnvVar)
	if requestedCachePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		// ensure cache dir exists
		o.LocalStorageDisk = filepath.Join(homeDir, cacheRelativePath)
	} else {
		o.LocalStorageDisk = filepath.Join(requestedCachePath, cacheRelativePath)
	}
	err := os.MkdirAll(o.LocalStorageDisk, 0755)
	if err != nil {
		o.Log.Error("unable to setup folder for oc-mirror local storage: %v ", err)
		return err
	}
	return nil
}

// setupWorkingDir - private utility to setup
// all the relevant working directory structures
func (o *ExecutorSchema) setupWorkingDir() error {
	// ensure working dir exists
	err := o.MakeDir.makeDirAll(o.Opts.Global.WorkingDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	// create signatures directory
	o.Log.Trace("creating signatures directory %s ", o.Opts.Global.WorkingDir+"/"+signaturesDir)
	err = o.MakeDir.makeDirAll(o.Opts.Global.WorkingDir+"/"+signaturesDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	// create release-images directory
	o.Log.Trace("creating release images directory %s ", o.Opts.Global.WorkingDir+"/"+releaseImageDir)
	err = o.MakeDir.makeDirAll(o.Opts.Global.WorkingDir+"/"+releaseImageDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	// create release cache dir
	o.Log.Trace("creating release cache directory %s ", o.Opts.Global.WorkingDir+"/"+releaseImageExtractDir)
	err = o.MakeDir.makeDirAll(o.Opts.Global.WorkingDir+"/"+releaseImageExtractDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	// create operator cache dir
	o.Log.Trace("creating operator cache directory %s ", o.Opts.Global.WorkingDir+"/"+operatorImageExtractDir)
	err = o.MakeDir.makeDirAll(o.Opts.Global.WorkingDir+"/"+operatorImageExtractDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}
	return nil
}

// RunMirrorToDisk - execute the mirror to disk functionality
func (o *ExecutorSchema) RunMirrorToDisk(cmd *cobra.Command, args []string) error {
	startTime := time.Now()

	o.Log.Info(startMessage, o.Opts.Global.Port)
	go startLocalRegistry(&o.LocalStorageService, o.localStorageInterruptChannel)

	allImages, err := o.CollectAll(cmd.Context())
	if err != nil {
		return err
	}
	collectionFinish := time.Now()

	//call the batch worker
	err = o.Batch.Worker(cmd.Context(), allImages, o.Opts)
	if err != nil {
		return err
	}

	// Prepare tar.gz when mirror to disk
	// First stop the registry
	interruptSig := NormalStorageInterruptErrorf("end of mirroring to disk. Stopping local storage to prepare the archive")
	o.localStorageInterruptChannel <- interruptSig

	// Next, generate the archive
	archiveFile, err := o.MirrorArchiver.BuildArchive(cmd.Context(), allImages)
	if err != nil {
		return err
	}
	defer o.MirrorArchiver.Close()
	o.Log.Info("archive file generated: %v ", archiveFile)
	mirrorFinish := time.Now()
	o.Log.Info("start time      : %v", startTime)
	o.Log.Info("collection time : %v", collectionFinish)
	o.Log.Info("mirror time     : %v", mirrorFinish)
	if err != nil {
		return err
	}
	return nil
}

// RunDiskToMirror execute the disk to mirror functionality
func (o *ExecutorSchema) RunDiskToMirror(cmd *cobra.Command, args []string) error {
	startTime := time.Now()

	// extract the archive
	err := o.MirrorUnArchiver.Unarchive()
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	// start the local storage registry
	o.Log.Info(startMessage, o.Opts.Global.Port)
	go startLocalRegistry(&o.LocalStorageService, o.localStorageInterruptChannel)

	// collect
	allImages, err := o.CollectAll(cmd.Context())
	if err != nil {
		return err
	}
	collectionFinish := time.Now()

	// Apply max-nested-paths processing if MaxNestedPaths>0
	if o.Opts.Global.MaxNestedPaths > 0 {
		allImages, err = withMaxNestedPaths(allImages, o.Opts.Global.MaxNestedPaths)
		if err != nil {
			return err
		}
	}
	//call the batch worker
	err = o.Batch.Worker(cmd.Context(), allImages, o.Opts)
	if err != nil {
		return err
	}
	//create IDMS/ITMS
	forceRepositoryScope := o.Opts.Global.MaxNestedPaths > 0
	err = o.ClusterResources.IDMS_ITMSGenerator(allImages, forceRepositoryScope)
	if err != nil {
		return err
	}

	err = o.ClusterResources.CatalogSourceGenerator(allImages)
	if err != nil {
		return err
	}

	// create updateService
	if o.Config.Mirror.Platform.Graph {
		graphImage, err := o.Release.GraphImage()
		if err != nil {
			return err
		}
		releaseImage, err := o.Release.ReleaseImage()
		if err != nil {
			return err
		}
		err = o.ClusterResources.UpdateServiceGenerator(graphImage, releaseImage)
		if err != nil {
			return err
		}
	}

	mirrorFinish := time.Now()
	o.Log.Info("start time      : %v", startTime)
	o.Log.Info("collection time : %v", collectionFinish)
	o.Log.Info("mirror time     : %v", mirrorFinish)
	if err != nil {
		return err
	}
	return nil
}

// setupLogsLevelAndDir - private utility to setup log
// level and relevant directory
func (o *ExecutorSchema) setupLogsLevelAndDir() error {
	// override log level
	o.Log.Level(o.Opts.Global.LogLevel)
	// set up location of logs dir
	o.LogsDir = filepath.Join(o.Opts.Global.WorkingDir, logsDir)
	// clean up logs directory
	os.RemoveAll(o.LogsDir)

	// create logs directory
	err := o.MakeDir.makeDirAll(o.LogsDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}
	return nil
}

// CollectAll - collect all relevant images for
// release, operators and additonalImages
func (o *ExecutorSchema) CollectAll(ctx context.Context) ([]v1alpha3.CopyImageSchema, error) {
	var allRelatedImages []v1alpha3.CopyImageSchema

	// do releases
	imgs, err := o.Release.ReleaseImageCollector(ctx)
	if err != nil {
		o.closeAll()
		return []v1alpha3.CopyImageSchema{}, err
	}
	o.Log.Info("total release images to copy %d ", len(imgs))
	o.Opts.ImageType = "release"
	allRelatedImages = mergeImages(allRelatedImages, imgs)

	// do operators
	imgs, err = o.Operator.OperatorImageCollector(ctx)
	if err != nil {
		o.closeAll()
		return []v1alpha3.CopyImageSchema{}, err
	}
	o.Log.Info("total operator images to copy %d ", len(imgs))
	o.Opts.ImageType = "operator"
	allRelatedImages = mergeImages(allRelatedImages, imgs)

	// do additionalImages
	imgs, err = o.AdditionalImages.AdditionalImagesCollector(ctx)
	if err != nil {
		o.closeAll()
		return []v1alpha3.CopyImageSchema{}, err
	}
	o.Log.Info("total additional images to copy %d ", len(imgs))
	allRelatedImages = mergeImages(allRelatedImages, imgs)

	return allRelatedImages, nil
}

// mergeImages - simple function to append related images
// nolint
func mergeImages(base, in []v1alpha3.CopyImageSchema) []v1alpha3.CopyImageSchema {
	base = append(base, in...)
	return base
}

// closeAll - utility to close any open files
func (o *ExecutorSchema) closeAll() {
	// close registry log file
	err := o.registryLogFile.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error closing log file %s: %v\n", registryLogFilename, err)
	}
}

func withMaxNestedPaths(in []v1alpha3.CopyImageSchema, maxNestedPaths int) ([]v1alpha3.CopyImageSchema, error) {
	out := []v1alpha3.CopyImageSchema{}
	for _, img := range in {
		dst, err := image.WithMaxNestedPaths(img.Destination, maxNestedPaths)
		if err != nil {
			return nil, err
		}
		img.Destination = dst
		out = append(out, img)
	}
	return out, nil
}

// NewPrepareCommand - setup all the relevant support structs
// to eventually execute the 'prepare' sub command
func NewPrepareCommand(log clog.PluggableLoggerInterface) *cobra.Command {
	global := &mirror.GlobalOptions{
		TlsVerify:    false,
		SecurePolicy: false,
	}

	flagSharedOpts, sharedOpts := mirror.SharedImageFlags()
	flagDepTLS, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	flagSrcOpts, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	flagDestOpts, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	flagRetryOpts, retryOpts := mirror.RetryFlags()

	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Dev:                 false,
	}

	ex := &ExecutorSchema{
		Log:  log,
		Opts: opts,
	}
	cmd := &cobra.Command{
		Use:   "prepare",
		Short: "Queries Cincinnati for the required releases to mirror, and verifies their existence in the local cache",
		Run: func(cmd *cobra.Command, args []string) {
			err := ex.ValidatePrepare(args)
			if err != nil {
				log.Error("%v ", err)
				os.Exit(1)
			}
			err = ex.CompletePrepare(args)
			if err != nil {
				log.Error("%v ", err)
				os.Exit(1)
			}
			// prepare internal storage
			err = ex.setupLocalStorage()
			if err != nil {
				log.Error(" %v ", err)
				os.Exit(1)
			}

			err = ex.RunPrepare(cmd, args)
			if err != nil {
				log.Error("%v ", err)
				os.Exit(1)
			}
		},
	}
	cmd.PersistentFlags().StringVarP(&opts.Global.ConfigPath, "config", "c", "", "Path to imageset configuration file")
	cmd.Flags().StringVar(&opts.Global.LogLevel, "loglevel", "info", "Log level one of (info, debug, trace, error)")
	cmd.Flags().StringVar(&opts.Global.WorkingDir, "dir", workingDir, "Assets directory")
	cmd.Flags().StringVar(&opts.Global.From, "from", "", "local storage directory for disk to mirror workflow")
	cmd.Flags().Uint16VarP(&opts.Global.Port, "port", "p", 55000, "HTTP port used by oc-mirror's local storage instance")
	cmd.Flags().BoolVar(&opts.Global.V2, "v2", opts.Global.V2, "Redirect the flow to oc-mirror v2 - PLEASE DO NOT USE it. V2 is still under development and it is not ready to be used.")
	// nolint: errcheck
	cmd.Flags().MarkHidden("v2")
	cmd.Flags().AddFlagSet(&flagSharedOpts)
	cmd.Flags().AddFlagSet(&flagRetryOpts)
	cmd.Flags().AddFlagSet(&flagDepTLS)
	cmd.Flags().AddFlagSet(&flagSrcOpts)
	cmd.Flags().AddFlagSet(&flagDestOpts)
	return cmd
}

// Validate - cobra validation
func (o ExecutorSchema) ValidatePrepare(dest []string) error {
	if len(o.Opts.Global.ConfigPath) == 0 {
		return fmt.Errorf("use the --config flag it is mandatory")
	}
	if o.Opts.Global.From == "" {
		return fmt.Errorf("with prepare command, the --from argument is mandatory (prefix : file://)")
	}
	if !strings.Contains(o.Opts.Global.From, fileProtocol) {
		return fmt.Errorf("when --from is used, it must have file:// prefix")
	}
	return nil
}

// CompletePrepare - cobra complete
func (o *ExecutorSchema) CompletePrepare(args []string) error {

	o.Log.Debug("imagesetconfig file %s ", o.Opts.Global.ConfigPath)
	// read the ImageSetConfiguration
	cfg, err := config.ReadConfig(o.Opts.Global.ConfigPath)
	if err != nil {
		return err
	}
	o.Log.Trace("imagesetconfig : %v ", cfg)

	// update all dependant modules
	mc := mirror.NewMirrorCopy()
	o.Manifest = manifest.New(o.Log)
	o.Mirror = mirror.New(mc, nil)
	o.Config = cfg

	o.Opts.Global.WorkingDir = filepath.Join(strings.Split(o.Opts.Global.From, "://")[1], workingDir)

	// setup logs level, and logsDir under workingDir
	err = o.setupLogsLevelAndDir()
	if err != nil {
		return err
	}
	if o.isLocalStoragePortBound() {
		return fmt.Errorf("%d is already bound and cannot be used", o.Opts.Global.Port)
	}
	o.LocalStorageFQDN = "localhost:" + strconv.Itoa(int(o.Opts.Global.Port))
	o.Opts.Mode = mirror.Prepare
	o.Log.Info("mode %s ", o.Opts.Mode)

	err = o.setupWorkingDir()
	if err != nil {
		return err
	}
	err = o.setupLocalStorageDir()
	if err != nil {
		return err
	}
	client, _ := release.NewOCPClient(uuid.New())

	signature := release.NewSignatureClient(o.Log, o.Config, o.Opts)
	cn := release.NewCincinnati(o.Log, &o.Config, o.Opts, client, false, signature)
	o.Release = release.New(o.Log, o.LogsDir, o.Config, o.Opts, o.Mirror, o.Manifest, cn, o.LocalStorageFQDN, o.ImageBuilder)
	o.Operator = operator.New(o.Log, o.LogsDir, o.Config, o.Opts, o.Mirror, o.Manifest, o.LocalStorageFQDN)
	o.AdditionalImages = additional.New(o.Log, o.Config, o.Opts, o.Mirror, o.Manifest, o.LocalStorageFQDN)
	return nil
}

// RunPrepare - cobra run
func (o *ExecutorSchema) RunPrepare(cmd *cobra.Command, args []string) error {

	// creating file for storing list of cached images
	cachedImagesFilePath := filepath.Join(o.LogsDir, "cached-images.txt")
	cachedImagesFile, err := os.Create(cachedImagesFilePath)
	if err != nil {
		return err
	}
	defer cachedImagesFile.Close()

	o.Log.Info(startMessage, o.Opts.Global.Port)
	go startLocalRegistry(&o.LocalStorageService, o.localStorageInterruptChannel)

	allImages, err := o.CollectAll(cmd.Context())
	if err != nil {
		return err
	}

	imagesAvailable := map[string]bool{}
	atLeastOneMissing := false
	var buff bytes.Buffer
	for _, img := range allImages {
		buff.WriteString(img.Destination + "\n")
		exists, err := o.Mirror.Check(cmd.Context(), img.Destination, &o.Opts)
		if err != nil {
			o.Log.Warn("unable to check existence of %s in local cache: %v", img.Destination, err)
		}
		if err != nil || !exists {
			atLeastOneMissing = true
		}
		imagesAvailable[img.Destination] = exists

	}

	_, err = cachedImagesFile.Write(buff.Bytes())
	if err != nil {
		return err
	}
	if atLeastOneMissing {
		o.Log.Error("missing images: ")
		for img, exists := range imagesAvailable {
			if !exists {
				o.Log.Error("%s", img)
			}
		}
		return fmt.Errorf("all images necessary for mirroring are not available in the cache. \nplease re-run the mirror to disk process")
	}

	o.Log.Info("all %d images required for mirroring are available in local cache. You may proceed with mirroring from disk to disconnected registry", len(imagesAvailable))
	o.Log.Info("full list in : %s", cachedImagesFilePath)
	return nil
}
