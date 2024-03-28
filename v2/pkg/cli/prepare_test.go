package cli

import (
	"context"
	"os"
	"testing"

	"github.com/distribution/distribution/v3/registry"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/config"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestExecutorValidatePrepare
func TestExecutorValidatePrepare(t *testing.T) {
	t.Run("Testing Executor : validate prepare should pass", func(t *testing.T) {

		log := clog.New("trace")

		global := &mirror.GlobalOptions{
			TlsVerify:    false,
			SecurePolicy: false,
		}

		_, sharedOpts := mirror.SharedImageFlags()
		_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
		_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
		_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
		_, retryOpts := mirror.RetryFlags()

		opts := &mirror.CopyOptions{
			Global:              global,
			DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
			SrcImage:            srcOpts,
			DestImage:           destOpts,
			RetryOpts:           retryOpts,
			Dev:                 false,
		}
		opts.Global.ConfigPath = "../../tests/isc.yaml"
		opts.Global.From = "file://test"

		ex := &ExecutorSchema{
			Log:     log,
			Opts:    opts,
			LogsDir: "/tmp/",
		}

		defer os.RemoveAll("../../pkg/cli/test")
		defer os.RemoveAll("../../pkg/cli/tmp")
		defer os.RemoveAll("../../pkg/cli/working-dir")

		err := ex.ValidatePrepare([]string{"file://test"})
		if err != nil {
			t.Fatalf("should not fail")
		}

		// check for config path error
		opts.Global.ConfigPath = ""
		err = ex.ValidatePrepare([]string{"file://test"})
		assert.Equal(t, "use the --config flag it is mandatory", err.Error())

		// check when from is used it should not be empty
		opts.Global.ConfigPath = "../../tests/isc.yaml"
		opts.Global.From = ""
		err = ex.ValidatePrepare([]string{"docker://test"})
		assert.Equal(t, "with prepare command, the --from argument is mandatory (prefix : file://)", err.Error())

		// check when from is used it should have file protocol
		opts.Global.ConfigPath = "../../tests/isc.yaml"
		opts.Global.From = "test"
		err = ex.ValidatePrepare([]string{"docker://test"})
		assert.Equal(t, "when --from is used, it must have file:// prefix", err.Error())
	})
}

// TestExecutorCompletePrepare
func TestExecutorCompletePrepare(t *testing.T) {
	t.Run("Testing Executor : complete prepare should pass", func(t *testing.T) {
		log := clog.New("trace")

		global := &mirror.GlobalOptions{
			TlsVerify:    false,
			SecurePolicy: false,
		}

		_, sharedOpts := mirror.SharedImageFlags()
		_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
		_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
		_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
		_, retryOpts := mirror.RetryFlags()

		opts := mirror.CopyOptions{
			Global:              global,
			DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
			SrcImage:            srcOpts,
			DestImage:           destOpts,
			RetryOpts:           retryOpts,
			Dev:                 false,
		}
		opts.Global.ConfigPath = "../../tests/isc.yaml"
		opts.Global.From = "file://test"

		ex := &ExecutorSchema{
			Log:     log,
			Opts:    &opts,
			MakeDir: MakeDir{},
			LogsDir: "/tmp/",
		}

		os.Setenv(cacheEnvVar, "/tmp/")

		defer os.RemoveAll("../../pkg/cli/test")
		defer os.RemoveAll("../../pkg/cli/tmp")
		defer os.RemoveAll("../../pkg/cli/working-dir")

		err := ex.CompletePrepare([]string{"file://test"})
		if err != nil {
			t.Fatalf("should not fail")
		}

	})
}

// TestExecutorRunPrepare
func TestExecutorRunPrepare(t *testing.T) {
	t.Run("Testing Executor : run prepare should pass", func(t *testing.T) {
		log := clog.New("trace")

		global := &mirror.GlobalOptions{
			TlsVerify:    false,
			SecurePolicy: false,
		}

		_, sharedOpts := mirror.SharedImageFlags()
		_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
		_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
		_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
		_, retryOpts := mirror.RetryFlags()

		opts := mirror.CopyOptions{
			Global:              global,
			DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
			SrcImage:            srcOpts,
			DestImage:           destOpts,
			RetryOpts:           retryOpts,
			Dev:                 false,
		}
		opts.Global.ConfigPath = "../../tests/isc.yaml"
		opts.Global.From = "file://test"

		testFolder := t.TempDir()
		defer os.RemoveAll(testFolder)

		// storage cache for test
		regCfg, err := setupRegForTest(testFolder)
		if err != nil {
			t.Errorf("storage cache error: %v ", err)
		}
		reg, err := registry.NewRegistry(context.Background(), regCfg)
		if err != nil {
			t.Errorf("storage cache error: %v ", err)
		}
		fakeStorageInterruptChan := make(chan error)
		go skipSignalsToInterruptStorage(fakeStorageInterruptChan)

		// read the ImageSetConfiguration
		cfg, err := config.ReadConfig(opts.Global.ConfigPath, v1alpha2.ImageSetConfigurationKind)
		if err != nil {
			log.Error("imagesetconfig %v ", err)
		}

		collector := &Collector{Log: log, Config: cfg.(v1alpha2.ImageSetConfiguration), Opts: opts, Fail: false}
		mockMirror := Mirror{}

		ex := &ExecutorSchema{
			Log:                          log,
			Opts:                         &opts,
			Operator:                     collector,
			Release:                      collector,
			AdditionalImages:             collector,
			Mirror:                       mockMirror,
			LocalStorageService:          *reg,
			localStorageInterruptChannel: fakeStorageInterruptChan,
			LogsDir:                      "/tmp/",
		}

		res := &cobra.Command{}
		res.SilenceUsage = true
		res.SetContext(context.Background())

		err = ex.RunPrepare(res, []string{"file://test"})
		if err != nil {
			t.Fatalf("should not fail")
		}
	})
}
