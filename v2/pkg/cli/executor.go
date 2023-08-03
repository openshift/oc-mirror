package cli

import (
	"bytes"
	"fmt"
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

	"github.com/openshift/oc-mirror/v2/pkg/additional"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/batch"
	"github.com/openshift/oc-mirror/v2/pkg/config"
	"github.com/openshift/oc-mirror/v2/pkg/diff"
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
	diskToMirror            string = "diskToMirror"
	mirrorToDisk            string = "mirrorToDisk"
	releaseImageDir         string = "release-images"
	logsDir                 string = "logs"
	workingDir              string = "working-dir"
	additionalImages        string = "additional-images"
	releaseImageExtractDir  string = "hold-release"
	operatorImageExtractDir string = "hold-operator"
	signaturesDir           string = "signatures"
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
	Log              clog.PluggableLoggerInterface
	Config           v1alpha2.ImageSetConfiguration
	MetaData         diff.SequenceSchema
	Opts             mirror.CopyOptions
	Operator         operator.CollectorInterface
	Release          release.CollectorInterface
	AdditionalImages additional.CollectorInterface
	Mirror           mirror.MirrorInterface
	Manifest         manifest.ManifestInterface
	Batch            batch.BatchInterface
	Diff             diff.DiffInterface
	LocalStorage     registry.Registry
}

// NewMirrorCmd - cobra entry point
func NewMirrorCmd(log clog.PluggableLoggerInterface) *cobra.Command {

	global := &mirror.GlobalOptions{
		TlsVerify:      false,
		InsecurePolicy: true,
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
			// prepare internal storage
			err = ex.PrepareStorage()
			if err != nil {
				log.Error(" %v ", err)
				os.Exit(1)
			}
			ex.Complete(args)

			err = ex.Run(cmd, args)
			if err != nil {
				log.Error("%v ", err)
				os.Exit(1)
			}
		},
	}

	cmd.PersistentFlags().StringVarP(&opts.Global.ConfigPath, "config", "c", "", "Path to imageset configuration file")
	cmd.Flags().StringVar(&opts.Global.LogLevel, "loglevel", "info", "Log level one of (info, debug, trace, error)")
	cmd.Flags().StringVar(&opts.Global.Dir, "dir", "working-dir", "Assets directory")
	cmd.Flags().StringVar(&opts.Global.From, "from", "working-dir", "local storage directory for disk to mirror workflow")
	cmd.Flags().Uint16VarP(&opts.Global.Port, "port", "p", 5000, "HTTP port used by oc-mirror's local storage instance")
	cmd.Flags().BoolVarP(&opts.Global.Quiet, "quiet", "q", false, "enable detailed logging when copying images")
	cmd.Flags().BoolVarP(&opts.Global.Force, "force", "f", false, "force the copy and mirror functionality")
	cmd.Flags().BoolVar(&opts.Global.V2, "v2", opts.Global.V2, "Redirect the flow to oc-mirror v2 - PLEASE DO NOT USE that. V2 is still under development and it is not ready to be used.")
	cmd.Flags().MarkHidden("v2")
	cmd.Flags().AddFlagSet(&flagSharedOpts)
	cmd.Flags().AddFlagSet(&flagRetryOpts)
	cmd.Flags().AddFlagSet(&flagDepTLS)
	cmd.Flags().AddFlagSet(&flagSrcOpts)
	cmd.Flags().AddFlagSet(&flagDestOpts)
	return cmd
}

// Validate - cobra validation
func (o *ExecutorSchema) Validate(dest []string) error {
	if len(o.Opts.Global.ConfigPath) == 0 {
		return fmt.Errorf("use the --config flag it is mandatory")
	}
	if strings.Contains(dest[0], dockerProtocol) && o.Opts.Global.From == "" {
		return fmt.Errorf("when destination is file://, diskToMirror workflow is assumed, and the --from argument become mandatory")

	}
	if strings.Contains(dest[0], fileProtocol) || strings.Contains(dest[0], dockerProtocol) {
		return nil
	} else {
		return fmt.Errorf("destination must have either file:// (mirror to disk) or docker:// (diskToMirror) protocol prefixes")
	}
}

func (o *ExecutorSchema) PrepareStorage() error {
	configYamlV0_1 := `
version: 0.1
log:
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

	rootDir := ""

	if o.Opts.Mode == mirrorToDisk {
		rootDir = strings.TrimPrefix(o.Opts.Destination, fileProtocol)
	} else {
		rootDir = strings.TrimPrefix(o.Opts.Global.From, fileProtocol)
	}

	if rootDir == "" {
		// something went wrong
		return fmt.Errorf("error determining the local storage folder to use")
	}
	configYamlV0_1 = strings.Replace(configYamlV0_1, "$$PLACEHOLDER_ROOT$$", rootDir, 1)
	configYamlV0_1 = strings.Replace(configYamlV0_1, "$$PLACEHOLDER_PORT$$", strconv.Itoa(int(o.Opts.Global.Port)), 1)
	config, err := configuration.Parse(bytes.NewReader([]byte(configYamlV0_1)))

	if err != nil {
		return fmt.Errorf("error parsing local storage configuration : %v\n %s\n", err, configYamlV0_1)
	}

	ctx := dcontext.WithVersion(dcontext.Background(), distversion.Version)
	reg, err := registry.NewRegistry(ctx, config)
	if err != nil {
		return err
	}
	o.LocalStorage = *reg
	errchan := make(chan error)

	o.Log.Info("starting local storage on %v", config.HTTP.Addr)

	go startLocalRegistry(reg, errchan)
	go panicOnRegistryError(errchan)
	return nil
}

func startLocalRegistry(reg *registry.Registry, errchan chan error) {
	err := reg.ListenAndServe()
	errchan <- err
}

func panicOnRegistryError(errchan chan error) {
	err := <-errchan
	if err != nil {
		panic(err)
	}
}

// Complete - do the final setup of modules
func (o *ExecutorSchema) Complete(args []string) {
	// override log level
	o.Log.Level(o.Opts.Global.LogLevel)
	o.Log.Debug("imagesetconfig file %s ", o.Opts.Global.ConfigPath)
	// read the ImageSetConfiguration
	cfg, err := config.ReadConfig(o.Opts.Global.ConfigPath)
	if err != nil {
		o.Log.Error("imagesetconfig %v ", err)
	}
	o.Log.Trace("imagesetconfig : %v ", cfg)

	// update all dependant modules
	mc := mirror.NewMirrorCopy()
	md := mirror.NewMirrorDelete()
	o.Manifest = manifest.New(o.Log)
	o.Mirror = mirror.New(mc, md)
	o.Config = cfg
	o.Batch = batch.New(o.Log, o.Mirror, o.Manifest)

	// logic to check mode
	var dest string
	if strings.Contains(args[0], fileProtocol) {
		o.Opts.Mode = mirrorToDisk
		dest = workingDir + "/" + strings.Split(args[0], "://")[1]
		o.Log.Debug("destination %s ", dest)
	} else if strings.Contains(args[0], dockerProtocol) {
		dest = workingDir
		o.Opts.Mode = diskToMirror
	} else {
		o.Log.Error("unable to determine the mode (the destination must be either file:// or docker://)")
	}
	o.Opts.Destination = args[0]
	o.Opts.Global.Dir = dest
	o.Log.Info("mode %s ", o.Opts.Mode)

	client, _ := release.NewOCPClient(uuid.New())

	signature := release.NewSignatureClient(o.Log, &o.Config, &o.Opts)
	cn := release.NewCincinnati(o.Log, &o.Config, &o.Opts, client, false, signature)
	o.Release = release.New(o.Log, o.Config, o.Opts, o.Mirror, o.Manifest, cn)
	o.Operator = operator.NewWithLocalStorage(o.Log, o.Config, o.Opts, o.Mirror, o.Manifest, "localhost:"+strconv.Itoa(int(o.Opts.Global.Port)))
	o.AdditionalImages = additional.NewWithLocalStorage(o.Log, o.Config, o.Opts, o.Mirror, o.Manifest, "localhost:"+strconv.Itoa(int(o.Opts.Global.Port)))

}

// Run - start the mirror functionality
func (o *ExecutorSchema) Run(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	// clean up logs directory
	os.RemoveAll(logsDir)

	// create logs directory
	err := os.MkdirAll(logsDir, 0755)
	if err != nil {
		o.Log.Error(" %v ", err)
		return err
	}

	if o.Opts.Mode == mirrorToDisk {
		// ensure working dir exists
		err := os.MkdirAll(workingDir, 0755)
		if err != nil {
			o.Log.Error(" %v ", err)
			return err
		}

		// create signatures directory
		o.Log.Trace("creating signatures directory %s ", o.Opts.Global.Dir+"/"+signaturesDir)
		err = os.MkdirAll(o.Opts.Global.Dir+"/"+signaturesDir, 0755)
		if err != nil {
			o.Log.Error(" %v ", err)
			return err
		}

		// create release-images directory
		o.Log.Trace("creating release images directory %s ", o.Opts.Global.Dir+"/"+releaseImageDir)
		err = os.MkdirAll(o.Opts.Global.Dir+"/"+releaseImageDir, 0755)
		if err != nil {
			o.Log.Error(" %v ", err)
			return err
		}

		// create release cache dir
		o.Log.Trace("creating release cache directory %s ", o.Opts.Global.Dir+"/"+releaseImageExtractDir)
		err = os.MkdirAll(o.Opts.Global.Dir+"/"+releaseImageExtractDir, 0755)
		if err != nil {
			o.Log.Error(" %v ", err)
			return err
		}

		// create operator cache dir
		o.Log.Trace("creating operator cache directory %s ", o.Opts.Global.Dir+"/"+operatorImageExtractDir)
		err = os.MkdirAll(o.Opts.Global.Dir+"/"+operatorImageExtractDir, 0755)
		if err != nil {
			o.Log.Error(" %v ", err)
			return err
		}
	}

	var allRelatedImages []v1alpha3.CopyImageSchema

	// do releases
	imgs, err := o.Release.ReleaseImageCollector(cmd.Context())
	if err != nil {
		cleanUp()
		return err
	}
	o.Log.Info("total release images to copy %d ", len(imgs))
	o.Opts.ImageType = "release"
	allRelatedImages = mergeImages(allRelatedImages, imgs)

	// do operators
	imgs, err = o.Operator.OperatorImageCollector(cmd.Context())
	if err != nil {
		cleanUp()
		return err
	}
	o.Log.Info("total operator images to copy %d ", len(imgs))
	o.Opts.ImageType = "operator"
	allRelatedImages = mergeImages(allRelatedImages, imgs)

	// do additionalImages
	imgs, err = o.AdditionalImages.AdditionalImagesCollector(cmd.Context())
	if err != nil {
		cleanUp()
		return err
	}
	o.Log.Info("total additional images to copy %d ", len(imgs))
	allRelatedImages = mergeImages(allRelatedImages, imgs)

	collectionFinish := time.Now()

	//call the batch worker
	err = o.Batch.Worker(cmd.Context(), allRelatedImages, o.Opts)
	mirrorFinish := time.Now()
	o.Log.Info("start time: %v\ncollection time: %v\nmirror time: %v", startTime, collectionFinish, mirrorFinish)
	if err != nil {
		cleanUp()
		return err
	}

	return nil
}

// mergeImages - simple function to append related images
// nolint
func mergeImages(base, in []v1alpha3.CopyImageSchema) []v1alpha3.CopyImageSchema {
	base = append(base, in...)
	return base
}

// cleanUp - utility to clean directories
func cleanUp() {
	// clean up logs directory
	os.RemoveAll(logsDir)

}
