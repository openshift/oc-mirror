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
	"github.com/otiai10/copy"
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

		ex.Opts.Global.WorkingDir = "file://test"
		opts.Global.ConfigPath = "../../tests/isc.yaml"

		err := ex.ValidateDelete([]string{"docker://test"})
		if err == nil {
			t.Fatalf("should fail")
		}

		// check for config path error
		opts.Global.ConfigPath = ""
		opts.Global.DeleteGenerate = true
		err = ex.ValidateDelete([]string{"docker://test"})
		assert.Equal(t, "the --config flag is mandatory when used with the --generate flag", err.Error())

		// check when workspace is not set
		opts.Global.ConfigPath = "../../tests/isc.yaml"
		ex.Opts.Global.WorkingDir = ""
		err = ex.ValidateDelete([]string{"docker://test"})
		assert.Equal(t, "use the --workspace flag, it is mandatory when using the delete command with the --generate flag", err.Error())

		// check when delete yaml file
		ex.Opts.Global.WorkingDir = "file://test"
		opts.Global.ConfigPath = "../../tests/isc.yaml"
		opts.Global.DeleteGenerate = false
		err = ex.ValidateDelete([]string{"test"})
		assert.Equal(t, "the --delete-yaml-file flag is mandatory when not using the --generate flag", err.Error())

		// check when destination is set but no protocol
		ex.Opts.Global.WorkingDir = "file://test"
		opts.Global.ConfigPath = "../../tests/isc.yaml"
		opts.Global.DeleteGenerate = false
		opts.Global.DeleteYaml = "../../tests/delete/delete-images.yaml"
		err = ex.ValidateDelete([]string{"test"})
		assert.Equal(t, "the destination registry argument must have a docker:// protocol prefix", err.Error())

		// check when destination is set yaml file not found
		ex.Opts.Global.WorkingDir = "file://test"
		opts.Global.ConfigPath = "../../tests/isc.yaml"
		opts.Global.DeleteGenerate = false
		opts.Global.DeleteYaml = "../../nothing"
		err = ex.ValidateDelete([]string{"docker://test"})
		assert.Equal(t, "file not found ../../nothing", err.Error())

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
		opts.Global.WorkingDir = "file://test"
		opts.Global.DeleteGenerate = true
		opts.Global.DeleteID = "test"

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

		err := ex.CompleteDelete([]string{"docker://myregistry:5000"})
		if err != nil {
			t.Fatalf("should not fail")
		}

		// using imagesetconfig not deleteimagesetconfig - should fail
		opts.Global.ConfigPath = "../../tests/isc.yaml"
		err = ex.CompleteDelete([]string{"docker://myregistry:5000"})
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
			WorkingDir:   "../../tests/temp-dir/tests/",
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
		dcfg, err := config.ReadConfig(opts.Global.ConfigPath, v1alpha2.DeleteImageSetConfigurationKind)
		if err != nil {
			log.Error("imagesetconfig %v ", err)
		}
		converted := dcfg.(v1alpha2.DeleteImageSetConfiguration)

		// we now coerce deleteimagesetconfig to imagesetconfig
		isc := v1alpha2.ImageSetConfiguration{
			ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
				Mirror: v1alpha2.Mirror{
					Platform:         converted.Delete.Platform,
					Operators:        converted.Delete.Operators,
					AdditionalImages: converted.Delete.AdditionalImages,
				},
			},
		}

		collector := &Collector{Log: log, Config: isc, Opts: opts, Fail: false}
		mockMirror := Mirror{}
		mockBatch := Batch{}
		_ = os.MkdirAll(testFolder+"/docker/registry/v2/repositories", 0755)
		_ = os.MkdirAll("../../tests/cache-fake-temp", 0755)
		defer os.RemoveAll("../../tests/cache-fake-temp")

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
			Delete:                       MockDelete{},
			LocalStorageDisk:             "../../tests/cache-fake-temp/",
		}

		res := &cobra.Command{}
		res.SilenceUsage = true
		res.SetContext(context.Background())
		ex.Opts.Mode = mirror.MirrorToDisk

		// copy cache-fake to cache-fake-temp for testing
		err = copy.Copy("../../tests/cache-fake/", "../../tests/cache-fake-temp/")
		if err != nil {
			t.Fatalf("should not fail : %v", err)
		}

		err = ex.RunDelete(res)
		if err != nil {
			t.Fatalf("should not fail : %v", err)
		}

		// test generate
		ex.Opts.Global.DeleteGenerate = true
		ex.Opts.Global.DeleteID = "test"
		ex.Opts.Global.WorkingDir = "file://test"
		err = ex.RunDelete(res)
		if err != nil {
			t.Fatalf("should not fail : %v", err)
		}

	})
}
