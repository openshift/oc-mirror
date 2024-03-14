package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/google/uuid"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/archive"
	"github.com/openshift/oc-mirror/v2/pkg/batch"
	"github.com/openshift/oc-mirror/v2/pkg/config"
	"github.com/openshift/oc-mirror/v2/pkg/delete"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/pkg/release"
	"github.com/spf13/cobra"
)

const (
	deleteErrMsg = "[delete] %v"
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
			err := ex.ValidateDelete()
			if err != nil {
				log.Error("%v ", err)
				os.Exit(1)
			}
			err = ex.CompleteDelete()
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
	cmd.PersistentFlags().StringVarP(&opts.Global.DeleteSource, "source", "s", "", "The working directory used to do the initial mirroring")
	cmd.Flags().StringVar(&opts.Global.LogLevel, "loglevel", "info", "Log level one of (info, debug, trace, error)")
	cmd.Flags().StringVar(&opts.Global.DeleteID, "delete-id", "", "Used to append to artifacts created by the delete functioality")
	cmd.Flags().BoolVar(&opts.Global.SkipCacheDelete, "skip-cache-delete", ex.Opts.Global.SkipCacheDelete, "Used to skip deleting of local cache manifests and blobs")
	cmd.Flags().BoolVar(&opts.Global.SkipRegistryDelete, "skip-registry-delete", ex.Opts.Global.SkipRegistryDelete, "Used to skip deleting of remote registry images")
	cmd.Flags().StringVar(&opts.Global.DeleteDestination, "destination", "", "The remote registry to delete from (optional)")
	cmd.Flags().StringVar(&opts.Global.WorkingDir, "dir", workingDir, "Assets directory")
	cmd.Flags().Uint16VarP(&opts.Global.Port, "port", "p", 55000, "HTTP port used by oc-mirror's local storage instance")
	cmd.Flags().BoolVar(&opts.Global.V2, "v2", ex.Opts.Global.V2, "Redirect the flow to oc-mirror v2 - PLEASE DON'T USE it. V2 is still under development and it is not production ready.")
	cmd.Flags().BoolVar(&opts.Global.DryRun, "dry-run", ex.Opts.Global.DryRun, "If set will only output the list of images (manifests and shas) to delete")
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
func (o ExecutorSchema) ValidateDelete() error {
	if len(o.Opts.Global.ConfigPath) == 0 {
		return fmt.Errorf("use the --config flag it is mandatory")
	}
	if len(o.Opts.Global.DeleteSource) == 0 {
		return fmt.Errorf("use the --source flag it is mandatory when using the delete command")
	} else {
		if !strings.Contains(o.Opts.Global.DeleteSource, fileProtocol) {
			return fmt.Errorf("--source flag must have a file:// protocol prefix")
		}
	}
	if len(o.Opts.Global.DeleteDestination) > 1 && !strings.Contains(o.Opts.Global.DeleteDestination, dockerProtocol) {
		return fmt.Errorf("--destination flag must have a docker:// protocol prefix")
	}
	return nil
}

// CompleteDelete - cobra complete
func (o *ExecutorSchema) CompleteDelete() error {

	o.Log.Debug("delete imagesetconfig file %s ", o.Opts.Global.ConfigPath)
	// read and validate the DeleteImageSetConfiguration
	cfg, err := config.ReadConfigDelete(o.Opts.Global.ConfigPath)
	if err != nil {
		return err
	}
	o.Log.Trace("delete imagesetconfig : %v ", cfg)
	if cfg.Kind != "DeleteImageSetConfiguration" {
		return fmt.Errorf("using the delete functiionlity requires the 'DeleteImageSetConfiguration' kind set in the yaml file")
	}

	// update all dependant modules
	mc := mirror.NewMirrorCopy()
	o.Manifest = manifest.New(o.Log)
	o.Mirror = mirror.New(mc, nil)

	// logic to check mode
	// always good to check - but this should have been detected in validate
	if strings.Contains(o.Opts.Global.DeleteSource, fileProtocol) {
		wd := strings.Split(o.Opts.Global.DeleteSource, fileProtocol)
		o.Opts.Global.WorkingDir = filepath.Join(wd[1], workingDir)
	} else {
		return fmt.Errorf("--source flag must have a file:// protocol prefix")
	}

	// we now coerce deleteimagesetconfig to imagesetconfig
	// as we want to use the underlying logic (filtering etc)
	isc := v1alpha2.ImageSetConfiguration{
		ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
			Mirror: v1alpha2.Mirror{
				Platform:         cfg.Delete.Platform,
				Operators:        cfg.Delete.Operators,
				AdditionalImages: cfg.Delete.AdditionalImages,
			},
		},
	}

	o.Config = isc

	// setup logs level, and logsDir under workingDir
	err = o.setupLogsLevelAndDir()
	if err != nil {
		return err
	}
	if o.isLocalStoragePortBound() {
		return fmt.Errorf("%d is already bound and cannot be used", o.Opts.Global.Port)
	}
	o.LocalStorageFQDN = "localhost:" + strconv.Itoa(int(o.Opts.Global.Port))

	// ensure mirror and batch worker use delete logic
	o.Opts.Function = string(mirror.DeleteMode)
	o.Log.Info("executing %s ", o.Opts.Function)

	if o.Opts.Global.DryRun {
		o.Log.Info("dry-run flag set, cache and remote registry deletion will be skipped")
	}

	if o.Opts.Global.SkipCacheDelete && !o.Opts.Global.DryRun {
		o.Log.Info("skip-cache-delete flag set, cache deletion will be skipped")
	}

	if o.Opts.Global.SkipRegistryDelete && !o.Opts.Global.DryRun {
		o.Log.Info("skip-registry-delete flag set, remote registry deletion will be skipped")
	}

	if len(o.Opts.Global.DeleteID) > 0 {
		o.Log.Info("using id %s to update all delete artifacts", o.Opts.Global.DeleteID)
	}

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
	o.Batch = batch.New(o.Log, o.LogsDir, o.Mirror, o.Manifest)
	// instantiate delete module
	bg := archive.NewImageBlobGatherer(o.Opts)
	o.Delete = delete.New(o.Log, *o.Opts, o.Batch, bg, o.Config, o.Manifest, o.LocalStorageDisk, o.LocalStorageFQDN)

	return nil
}

// RunDelete - cobra run
func (o *ExecutorSchema) RunDelete(cmd *cobra.Command) error {

	startTime := time.Now()
	o.Log.Debug("config %v", o.Config)
	o.Log.Info(startMessage, o.Opts.Global.Port)
	go startLocalRegistry(&o.LocalStorageService, o.localStorageInterruptChannel)

	// lets get the release images from local disk
	_, releaseFolder, err := o.Release.IdentifyReleases()
	if err != nil {
		o.Log.Error(deleteErrMsg, err)
	}

	// collect release images from local file system
	var allImages []v1alpha3.CopyImageSchema
	for _, i := range releaseFolder {
		allImages, err = o.Delete.CollectReleaseImages(i)
		if err != nil {
			o.Log.Error(deleteErrMsg, err)
		}
	}

	// collect operator images
	oi, err := o.Delete.CollectOperatorImages()
	if err != nil {
		o.Log.Error(" %v", err)
	}
	allImages = append(allImages, oi...)

	// collect additional images
	ai, err := o.Delete.CollectAdditionalImages()
	if err != nil {
		o.Log.Error(deleteErrMsg, err)
	}
	allImages = append(allImages, ai...)

	collectionFinish := time.Now()

	err = o.Delete.DeleteCacheBlobs(context.Background(), allImages)
	if err != nil {
		return err
	}

	err = o.Delete.DeleteRegistryImages(context.Background(), allImages)
	if err != nil {
		return err
	}

	// finally just verify that the yaml file is well formed
	_, err = o.Delete.ReadDeleteMetaData()
	if err != nil {
		return err
	}

	deleteFinish := time.Now()
	o.Log.Info("start time      : %v", startTime)
	o.Log.Info("collection time : %v", collectionFinish)
	o.Log.Info("delete time     : %v", deleteFinish)

	return nil
}
