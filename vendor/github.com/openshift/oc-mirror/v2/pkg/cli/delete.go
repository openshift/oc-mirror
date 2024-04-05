package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/google/uuid"

	"github.com/openshift/oc-mirror/v2/pkg/additional"
	"github.com/openshift/oc-mirror/v2/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/pkg/archive"
	"github.com/openshift/oc-mirror/v2/pkg/batch"
	"github.com/openshift/oc-mirror/v2/pkg/config"
	"github.com/openshift/oc-mirror/v2/pkg/delete"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/pkg/operator"
	"github.com/openshift/oc-mirror/v2/pkg/release"
	"github.com/spf13/cobra"
)

const (
	deleteErrMsg = "[delete] %v"
	deleteYaml   = "/delete/delete-images.yaml"
	deleteDir    = "/delete/"

	deletePrefix = "[RunDelete] "
	errMsg       = deletePrefix + "%s"
)

// NewDeleteCommand - setup all the relevant support structs
// to eventually execute the 'delete' sub command
func NewDeleteCommand(log clog.PluggableLoggerInterface) *cobra.Command {

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
		Function:            string(mirror.DeleteMode),
	}

	mkd := MakeDir{}
	ex := &ExecutorSchema{
		Log:     log,
		Opts:    opts,
		MakeDir: mkd,
	}

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Deletes all related images and manifests from a remote repository or local cache or both",
		Run: func(cmd *cobra.Command, args []string) {
			log.Info("👋 Hello, welcome to oc-mirror")
			log.Info("⚙️  setting up the environment for you...")

			err := ex.ValidateDelete(args)
			if err != nil {
				log.Error("%v ", err)
				os.Exit(1)
			}
			err = ex.CompleteDelete(args)
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

			err = ex.RunDelete(cmd)
			if err != nil {
				log.Error("%v ", err)
				os.Exit(1)
			}
		},
	}
	cmd.PersistentFlags().StringVarP(&opts.Global.ConfigPath, "config", "c", "", "Path to delete imageset configuration file")
	cmd.PersistentFlags().StringVarP(&opts.Global.WorkingDir, "workspace", "w", "", "oc-mirror workspace where resources and internal artifacts are generated")
	cmd.Flags().StringVar(&opts.Global.LogLevel, "loglevel", "info", "Log level one of (info, debug, trace, error)")
	cmd.Flags().StringVar(&opts.Global.DeleteID, "delete-id", "", "Used to differentiate between versions for files created by the delete functionality")
	cmd.Flags().StringVar(&opts.Global.DeleteYaml, "delete-yaml-file", "", "If set will use the generated or updated yaml file to delete contents")
	cmd.Flags().BoolVar(&opts.Global.ForceCacheDelete, "force-cache-delete", false, "Used to force delete  the local cache manifests and blobs")
	cmd.Flags().Uint16VarP(&opts.Global.Port, "port", "p", 55000, "HTTP port used by oc-mirror's local storage instance")
	cmd.Flags().BoolVar(&opts.Global.V2, "v2", ex.Opts.Global.V2, "Redirect the flow to oc-mirror v2 - This is Tech Preview, it is still under development and it is not production ready.")
	cmd.Flags().BoolVar(&opts.Global.DeleteGenerate, "generate", false, "Used to generate the delete yaml for the list of manifests and blobs , used in the step to actually delete from local cahce and remote registry")
	// nolint: errcheck
	cmd.Flags().MarkHidden("v2")
	cmd.Flags().AddFlagSet(&flagSharedOpts)
	cmd.Flags().AddFlagSet(&flagRetryOpts)
	cmd.Flags().AddFlagSet(&flagDepTLS)
	cmd.Flags().AddFlagSet(&flagSrcOpts)
	cmd.Flags().AddFlagSet(&flagDestOpts)

	// hide flags
	HideFlags(cmd)

	return cmd
}

// Validate - cobra validation
func (o ExecutorSchema) ValidateDelete(args []string) error {
	if o.Opts.Global.DeleteGenerate {
		if len(o.Opts.Global.WorkingDir) == 0 {
			return fmt.Errorf("use the --workspace flag, it is mandatory when using the delete command with the --generate flag")
		}
		if !strings.Contains(o.Opts.Global.WorkingDir, fileProtocol) {
			return fmt.Errorf("--workspace flag must have a file:// protocol prefix")
		}
		if len(o.Opts.Global.ConfigPath) == 0 {
			return fmt.Errorf("the --config flag is mandatory when used with the --generate flag")
		}
	} else {
		if len(o.Opts.Global.DeleteYaml) == 0 {
			return fmt.Errorf("the --delete-yaml-file flag is mandatory when not using the --generate flag")
		}
	}
	if len(args) < 1 {
		return fmt.Errorf("the destination registry is missing in the command arguments")
	}
	if len(args[0]) > 1 && !strings.Contains(args[0], dockerProtocol) {
		return fmt.Errorf("the destination registry argument must have a docker:// protocol prefix")
	}

	delete_file := o.Opts.Global.DeleteYaml

	_, err := os.Stat(delete_file)
	if len(o.Opts.Global.DeleteYaml) > 0 && !o.Opts.Global.DeleteGenerate && os.IsNotExist(err) {
		return fmt.Errorf("file not found %s", delete_file)
	}
	return nil
}

// CompleteDelete - cobra complete
func (o *ExecutorSchema) CompleteDelete(args []string) error {
	if args[0] == "" {
		return fmt.Errorf("the destination registry was not found in the command line arguments")
	}
	if !strings.HasPrefix(args[0], dockerProtocol) {
		return fmt.Errorf("the destination registry must be prefixed by docker://")
	}
	o.Opts.Destination = args[0]
	o.Opts.Global.DeleteDestination = args[0]
	if o.Opts.Global.DeleteGenerate {
		o.Log.Debug("delete imagesetconfig file %s ", o.Opts.Global.ConfigPath)
		// read and validate the DeleteImageSetConfiguration
		cfg, err := config.ReadConfig(o.Opts.Global.ConfigPath, v2alpha1.DeleteImageSetConfigurationKind)
		if err != nil {
			return err
		}
		converted := cfg.(v2alpha1.DeleteImageSetConfiguration)
		o.Log.Trace("delete imagesetconfig : %v ", converted)
		if converted.Kind != "DeleteImageSetConfiguration" {
			return fmt.Errorf("using the delete functionality requires the 'DeleteImageSetConfiguration' kind set in the yaml file")
		}
		// we now coerce deleteimagesetconfig to imagesetconfig
		// as we want to use the underlying logic (filtering etc)
		isc := v2alpha1.ImageSetConfiguration{
			ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform:         converted.Delete.Platform,
					Operators:        converted.Delete.Operators,
					AdditionalImages: converted.Delete.AdditionalImages,
				},
			},
		}
		o.Config = isc
		// fake a diskToMirror Mode
		o.Opts.Mode = mirror.DiskToMirror
		o.Opts.RemoveSignatures = true
	}

	// update all dependant modules
	mc := mirror.NewMirrorCopy()
	o.Manifest = manifest.New(o.Log)
	o.Mirror = mirror.New(mc, nil)

	// logic to check mode
	// always good to check - but this should have been detected in validate
	if o.Opts.Global.DeleteGenerate {
		if strings.Contains(o.Opts.Global.WorkingDir, fileProtocol) {
			wd := strings.Split(o.Opts.Global.WorkingDir, fileProtocol)
			o.Opts.Global.WorkingDir = filepath.Join(wd[1], workingDir)
		} else {
			return fmt.Errorf("--workspace flag must have a file:// protocol prefix")
		}
	}

	// setup logs level, and logsDir under workingDir
	err := o.setupLogsLevelAndDir()
	if err != nil {
		return err
	}
	if o.isLocalStoragePortBound() {
		return fmt.Errorf("%d is already bound and cannot be used", o.Opts.Global.Port)
	}
	o.LocalStorageFQDN = "localhost:" + strconv.Itoa(int(o.Opts.Global.Port))

	// ensure mirror and batch worker use delete logic
	o.Opts.Function = string(mirror.DeleteMode)
	o.Log.Info("🔀 workflow mode: %s / %s", o.Opts.Mode, o.Opts.Function)

	if o.Opts.Global.DeleteGenerate {
		err = o.setupWorkingDir()
		if err != nil {
			return err
		}
		absPath, err := filepath.Abs(o.Opts.Global.WorkingDir + deleteDir)
		if err != nil {
			o.Log.Error("absolute path %v", err)
		}
		if len(o.Opts.Global.DeleteID) > 0 {
			o.Log.Debug("using id %s to update all delete generated files", o.Opts.Global.DeleteID)
		}
		o.Log.Debug("generate flag set, files will be created in %s", absPath)
	}

	if o.Opts.Global.ForceCacheDelete && !o.Opts.Global.DeleteGenerate {
		o.Log.Debug("force-cache-delete flag set, cache deletion will be forced")
	}

	err = o.setupLocalStorageDir()
	if err != nil {
		return err
	}

	client, _ := release.NewOCPClient(uuid.New())
	signature := release.NewSignatureClient(o.Log, o.Config, *o.Opts)
	cn := release.NewCincinnati(o.Log, &o.Config, *o.Opts, client, false, signature)
	o.Release = release.New(o.Log, o.LogsDir, o.Config, *o.Opts, o.Mirror, o.Manifest, cn, o.LocalStorageFQDN, o.ImageBuilder)
	o.Batch = batch.New(o.Log, o.LogsDir, o.Mirror, o.Manifest)
	o.Operator = operator.New(o.Log, o.LogsDir, o.Config, *o.Opts, o.Mirror, o.Manifest, o.LocalStorageFQDN)
	o.AdditionalImages = additional.New(o.Log, o.Config, *o.Opts, o.Mirror, o.Manifest, o.LocalStorageFQDN)

	// instantiate delete module
	bg := archive.NewImageBlobGatherer(o.Opts)
	o.Delete = delete.New(o.Log, *o.Opts, o.Batch, bg, o.Config, o.Manifest, o.LocalStorageDisk, o.LocalStorageFQDN)

	return nil
}

// RunDelete - cobra run
func (o *ExecutorSchema) RunDelete(cmd *cobra.Command) error {
	startTime := time.Now()
	o.Log.Debug("config %v", o.Config)
	o.Log.Debug(startMessage, o.Opts.Global.Port)

	go startLocalRegistry(&o.LocalStorageService, o.localStorageInterruptChannel)

	if o.Opts.Global.DeleteGenerate {

		// lets get the release images from local disk
		releaseImages, err := o.Delete.FilterReleasesForDelete()
		if err != nil {
			o.Log.Error(errMsg, err.Error())
		}

		o.Log.Info("🕵️  going to discover the necessary images...")
		o.Log.Info("🔍 collecting release images...")
		// convert release images
		var allImages []v2alpha1.CopyImageSchema
		for _, i := range releaseImages {
			ai, err := o.Delete.ConvertReleaseImages(i)
			if err != nil {
				o.Log.Error(errMsg, err.Error())
			}
			allImages = append(allImages, ai...)
		}

		o.Log.Info("🔍 collecting operator images...")
		// collect operator images
		oImgs, err := o.Operator.OperatorImageCollector(cmd.Context())
		if err != nil {
			o.Log.Error(" %v", err)
		}
		allImages = append(allImages, oImgs...)

		o.Log.Info("🔍 collecting additional images...")
		// collect additional images
		ai, err := o.AdditionalImages.AdditionalImagesCollector(cmd.Context())
		if err != nil {
			o.Log.Error(errMsg, err.Error())
		}
		allImages = append(allImages, ai...)

		err = o.Delete.WriteDeleteMetaData(allImages)
		if err != nil {
			return err
		}
	} else {

		deleteList, err := o.Delete.ReadDeleteMetaData()
		if err != nil {
			return err
		}

		err = o.Delete.DeleteRegistryImages(deleteList)
		if err != nil {
			return err
		}
	}

	endTime := time.Now()
	execTime := endTime.Sub(startTime)
	o.Log.Info("delete time     : %v", execTime)

	if o.Opts.Global.ForceCacheDelete {
		// finally execute the garbage collector
		// this will delete all relevant blobs
		err := o.startLocalRegistryGarbageCollect()
		if err != nil {
			return err
		}
	}

	o.Log.Info("👋 Goodbye, thank you for using oc-mirror")

	return nil
}

// startLocalRegistryGarbageCollect
func (o *ExecutorSchema) startLocalRegistryGarbageCollect() error {
	// setup storage driver for garbage-collect
	config, err := o.setupLocalRegistryConfig()
	if err != nil {
		return err
	}
	storageDriver, err := factory.Create(config.Storage.Type(), config.Storage.Parameters())
	if err != nil {
		return err
	}

	opts := storage.GCOpts{
		DryRun:         false,
		RemoveUntagged: false,
	}

	ctx := context.Background()

	// used for the garbage-collect
	storageReg, err := storage.NewRegistry(ctx, storageDriver)
	if err != nil {
		return err
	}

	// set up garbage-collect to remove blobs
	err = storage.MarkAndSweep(ctx, storageDriver, storageReg, opts)
	if err != nil {
		return err
	}

	return nil
}
