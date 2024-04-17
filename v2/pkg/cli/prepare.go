package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/google/uuid"
	"github.com/openshift/oc-mirror/v2/pkg/additional"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/config"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/pkg/operator"
	"github.com/openshift/oc-mirror/v2/pkg/release"
	"github.com/spf13/cobra"
)

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

	opts := &mirror.CopyOptions{
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
	cmd.Flags().BoolVar(&opts.Global.V2, "v2", opts.Global.V2, "Redirect the flow to oc-mirror v2 - PLEASE DO NOT USE it. V2 is still under development and it is not production ready.")
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
	cfg, err := config.ReadConfig(o.Opts.Global.ConfigPath, v1alpha2.ImageSetConfigurationKind)
	if err != nil {
		return err
	}
	o.Log.Trace("imagesetconfig : %v ", cfg)

	// update all dependant modules
	mc := mirror.NewMirrorCopy()
	o.Manifest = manifest.New(o.Log)
	o.Mirror = mirror.New(mc, nil)
	o.Config = cfg.(v1alpha2.ImageSetConfiguration)

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

	signature := release.NewSignatureClient(o.Log, o.Config, *o.Opts)
	cn := release.NewCincinnati(o.Log, &o.Config, *o.Opts, client, false, signature)
	o.Release = release.New(o.Log, o.LogsDir, o.Config, *o.Opts, o.Mirror, o.Manifest, cn, o.LocalStorageFQDN, o.ImageBuilder)
	o.Operator = operator.New(o.Log, o.LogsDir, o.Config, *o.Opts, o.Mirror, o.Manifest, o.LocalStorageFQDN)
	o.AdditionalImages = additional.New(o.Log, o.Config, *o.Opts, o.Mirror, o.Manifest, o.LocalStorageFQDN)
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
		exists, err := o.Mirror.Check(cmd.Context(), img.Destination, o.Opts)
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
