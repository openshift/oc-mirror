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

// TestExecutorValidateDelete
func TestExecutorValidateDelete(t *testing.T) {
	t.Run("Testing Delete Executor : validate delete should pass", func(t *testing.T) {

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

		ex := &ExecutorSchema{
			Log:     log,
			Opts:    opts,
			LogsDir: "/tmp/",
		}

		defer os.RemoveAll("../../pkg/cli/test")
		defer os.RemoveAll("../../pkg/cli/tmp")
		defer os.RemoveAll("../../pkg/cli/working-dir")

		ex.Opts.Global.DeleteSource = "file://test"
		ex.Opts.Global.DeleteDestination = "docker://test"
		opts.Global.ConfigPath = "../../tests/isc.yaml"

		err := ex.ValidateDelete()
		if err != nil {
			t.Fatalf("should not fail")
		}

		// check for config path error
		opts.Global.ConfigPath = ""
		err = ex.ValidateDelete()
		assert.Equal(t, "use the --config flag it is mandatory", err.Error())

		// check when source is not set
		opts.Global.ConfigPath = "../../tests/isc.yaml"
		ex.Opts.Global.DeleteSource = ""
		ex.Opts.Global.DeleteDestination = "docker://test"
		err = ex.ValidateDelete()
		assert.Equal(t, "use the --source flag it is mandatory when using the delete command", err.Error())

		// check when destination is set but no protocol
		ex.Opts.Global.DeleteSource = "file://test"
		ex.Opts.Global.DeleteDestination = "test"
		opts.Global.ConfigPath = "../../tests/isc.yaml"
		err = ex.ValidateDelete()
		assert.Equal(t, "--destination flag must have a docker:// protocol prefix", err.Error())

	})
}

// TestExecutorCompleteDelete
func TestExecutorCompleteDelete(t *testing.T) {
	t.Run("Testing Executor : complete delete should pass", func(t *testing.T) {
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
		opts.Global.ConfigPath = "../../tests/delete-isc.yaml"
		opts.Global.DeleteSource = "file://test"

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

		err := ex.CompleteDelete()
		if err != nil {
			t.Fatalf("should not fail")
		}

		// using imagesetconfig not deleteimagesetconfig - should fail
		opts.Global.ConfigPath = "../../tests/isc.yaml"
		err = ex.CompleteDelete()
		if err == nil {
			t.Fatalf("should fail")
		}

	})
}

// TestExecutorRunDelete
func TestExecutorRunDelete(t *testing.T) {
	t.Run("Testing Executor : run delete should pass", func(t *testing.T) {
		log := clog.New("trace")

		global := &mirror.GlobalOptions{
			TlsVerify:    false,
			SecurePolicy: false,
			WorkingDir:   "../../tests/",
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
		opts.Global.ConfigPath = "../../tests/delete-isc.yaml"
		opts.Global.From = ""

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

		// read the DeleteImageSetConfiguration
		dcfg, err := config.ReadConfigDelete(opts.Global.ConfigPath)
		if err != nil {
			log.Error("imagesetconfig %v ", err)
		}

		// we now coerce deleteimagesetconfig to imagesetconfig
		isc := v1alpha2.ImageSetConfiguration{
			ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
				Mirror: v1alpha2.Mirror{
					Platform:         dcfg.Delete.Platform,
					Operators:        dcfg.Delete.Operators,
					AdditionalImages: dcfg.Delete.AdditionalImages,
				},
			},
		}

		collector := &Collector{Log: log, Config: isc, Opts: opts, Fail: false}
		mockMirror := Mirror{}
		mockBatch := Batch{}

		ex := &ExecutorSchema{
			Log:                          log,
			Opts:                         &opts,
			Operator:                     collector,
			Release:                      collector,
			AdditionalImages:             collector,
			Mirror:                       mockMirror,
			Batch:                        &mockBatch,
			LocalStorageService:          *reg,
			localStorageInterruptChannel: fakeStorageInterruptChan,
			LogsDir:                      "/tmp/",
			Delete:                       &MockDelete{},
		}

		res := &cobra.Command{}
		res.SilenceUsage = true
		res.SetContext(context.Background())
		ex.Opts.Mode = mirror.MirrorToDisk
		ex.Opts.Global.WorkingDir = testFolder

		err = ex.RunDelete(res)
		if err != nil {
			t.Fatalf("should not fail")
		}

		ex.Opts.Global.WorkingDir = "../../tests"
		err = ex.RunDelete(res)
		if err != nil {
			t.Fatalf("should not fail")
		}

	})
}
