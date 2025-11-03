package cli

import (
	"context"
	"os"
	"testing"

	"github.com/distribution/distribution/v3/registry"
	"github.com/otiai10/copy"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
	"github.com/openshift/oc-mirror/v2/internal/pkg/config"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	mirrormock "github.com/openshift/oc-mirror/v2/internal/pkg/mirror/mock"
)

// TestExecutorValidateDelete
func TestExecutorValidateDelete(t *testing.T) {
	t.Run("Testing Delete Executor : validate delete should pass", func(t *testing.T) {
		log := clog.New("trace")

		global := &mirror.GlobalOptions{
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
		opts.Global.ConfigPath = common.TestFolder + "isc.yaml"

		ex := &DeleteSchema{
			ExecutorSchema: ExecutorSchema{
				Log:     log,
				Opts:    opts,
				LogsDir: "/tmp/",
			},
		}

		defer os.RemoveAll("../../pkg/cli/test")
		defer os.RemoveAll("../../pkg/cli/tmp")
		defer os.RemoveAll("../../pkg/cli/working-dir")

		ex.Opts.Global.WorkingDir = "file://test"
		opts.Global.ConfigPath = common.TestFolder + "isc.yaml"

		err := ex.ValidateDelete([]string{"docker://test"})
		assert.Error(t, err)

		// check for config path error
		opts.Global.ConfigPath = ""
		opts.Global.DeleteGenerate = true
		err = ex.ValidateDelete([]string{"docker://test"})
		assert.ErrorContains(t, err, "the --config flag is mandatory when used with the --generate flag")

		// check when workspace is not set
		opts.Global.ConfigPath = common.TestFolder + "isc.yaml"
		ex.Opts.Global.WorkingDir = ""
		err = ex.ValidateDelete([]string{"docker://test"})
		assert.ErrorContains(t, err, "use the --workspace flag, it is mandatory when using the delete command with the --generate flag")

		// check when delete yaml file
		ex.Opts.Global.WorkingDir = "file://test"
		opts.Global.ConfigPath = common.TestFolder + "isc.yaml"
		opts.Global.DeleteGenerate = false
		err = ex.ValidateDelete([]string{"test"})
		assert.ErrorContains(t, err, "the --delete-yaml-file flag is mandatory when not using the --generate flag")

		// check when destination is set but no protocol
		ex.Opts.Global.WorkingDir = "file://test"
		opts.Global.ConfigPath = common.TestFolder + "isc.yaml"
		opts.Global.DeleteGenerate = false
		opts.Global.DeleteYaml = common.TestFolder + "delete/delete-images.yaml"
		err = ex.ValidateDelete([]string{"test"})
		assert.ErrorContains(t, err, "the destination registry argument must have a docker:// protocol prefix")

		// check when destination is set yaml file not found
		ex.Opts.Global.WorkingDir = "file://test"
		opts.Global.ConfigPath = common.TestFolder + "isc.yaml"
		opts.Global.DeleteGenerate = false
		opts.Global.DeleteYaml = "../../nothing"
		err = ex.ValidateDelete([]string{"docker://test"})
		assert.ErrorContains(t, err, "file not found ../../nothing")
	})
}

// TestExecutorCompleteDelete
func TestExecutorCompleteDelete(t *testing.T) {
	t.Run("Testing Executor : complete delete should pass", func(t *testing.T) {
		log := clog.New("trace")

		global := &mirror.GlobalOptions{
			SecurePolicy: false,
			CacheDir:     "/tmp/",
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
		opts.Global.ConfigPath = common.TestFolder + "delete-isc.yaml"
		opts.Global.WorkingDir = "file://test"
		opts.Global.DeleteGenerate = true
		opts.Global.DeleteID = "test"

		ex := &DeleteSchema{
			ExecutorSchema: ExecutorSchema{
				Log:     log,
				Opts:    &opts,
				MakeDir: MakeDir{},
				LogsDir: "/tmp/",
			},
		}

		defer os.RemoveAll("../../pkg/cli/test")
		defer os.RemoveAll("../../pkg/cli/tmp")
		defer os.RemoveAll("../../pkg/cli/working-dir")

		err := ex.CompleteDelete([]string{"docker://myregistry:5000"})
		assert.NoError(t, err)

		// using imagesetconfig not deleteimagesetconfig - should fail
		opts.Global.ConfigPath = common.TestFolder + "isc.yaml"
		err = ex.CompleteDelete([]string{"docker://myregistry:5000"})
		assert.Error(t, err)
	})
}

// TestExecutorRunDelete
func TestExecutorRunDelete(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mirrorMock := mirrormock.NewMockMirrorInterface(mockCtrl)

	t.Run("Testing Executor : run delete should pass", func(t *testing.T) {
		log := clog.New("trace")

		global := &mirror.GlobalOptions{
			SecurePolicy: false,
			WorkingDir:   common.TestFolder + "temp-dir/tests/",
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
		opts.Global.ConfigPath = common.TestFolder + "delete-isc.yaml"
		opts.Global.From = ""

		testFolder := t.TempDir()

		// storage cache for test
		regCfg, err := setupRegForTest(testFolder)
		assert.NoError(t, err)
		reg, err := registry.NewRegistry(context.Background(), regCfg)
		assert.NoError(t, err)

		// read the DeleteImageSetConfiguration
		dcfg, err := config.ReadConfig(opts.Global.ConfigPath, v2alpha1.DeleteImageSetConfigurationKind)
		assert.NoError(t, err)
		converted := dcfg.(v2alpha1.DeleteImageSetConfiguration)

		// we now coerce deleteimagesetconfig to imagesetconfig
		isc := v2alpha1.ImageSetConfiguration{
			ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform:         converted.Delete.Platform,
					Operators:        converted.Delete.Operators,
					AdditionalImages: converted.Delete.AdditionalImages,
				},
			},
		}

		collector := &Collector{Log: log, Config: isc, Opts: opts, Fail: false}
		mockBatch := Batch{}
		_ = os.MkdirAll(testFolder+"/docker/registry/v2/repositories", 0o755)
		_ = os.MkdirAll(common.TestFolder+"cache-fake-temp", 0o755)
		defer os.RemoveAll(common.TestFolder + "cache-fake-temp")
		opts.LocalStorageFQDN = regCfg.HTTP.Addr

		ex := &DeleteSchema{
			ExecutorSchema: ExecutorSchema{
				Log:                 log,
				Opts:                &opts,
				Operator:            collector,
				Release:             collector,
				AdditionalImages:    collector,
				HelmCollector:       collector,
				Mirror:              mirrorMock,
				Batch:               &mockBatch,
				LocalStorageService: *reg,
				LogsDir:             "/tmp/",
				Delete:              MockDelete{},
				LocalStorageDisk:    common.TestFolder + "cache-fake-temp/",
			},
		}

		res := &cobra.Command{}
		res.SilenceUsage = true
		res.SetContext(context.Background())
		ex.Opts.Mode = mirror.MirrorToDisk

		// copy cache-fake to cache-fake-temp for testing
		err = copy.Copy(common.TestFolder+"cache-fake/", common.TestFolder+"cache-fake-temp/")
		assert.NoError(t, err)

		err = ex.RunDelete(res)
		assert.NoError(t, err)
	})

	t.Run("Testing Executor : run delete --generate should pass", func(t *testing.T) {
		log := clog.New("trace")

		global := &mirror.GlobalOptions{
			SecurePolicy:   false,
			WorkingDir:     "file://test",
			DeleteGenerate: true,
			DeleteID:       "test",
			ConfigPath:     common.TestFolder + "delete-isc.yaml",
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

		testFolder := t.TempDir()
		defer os.RemoveAll(testFolder)

		// storage cache for test
		regCfg, err := setupRegForTest(testFolder)
		assert.NoError(t, err)
		reg, err := registry.NewRegistry(context.Background(), regCfg)
		assert.NoError(t, err)

		// read the DeleteImageSetConfiguration
		dcfg, err := config.ReadConfig(opts.Global.ConfigPath, v2alpha1.DeleteImageSetConfigurationKind)
		assert.NoError(t, err)
		converted := dcfg.(v2alpha1.DeleteImageSetConfiguration)

		// we now coerce deleteimagesetconfig to imagesetconfig
		isc := v2alpha1.ImageSetConfiguration{
			ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform:         converted.Delete.Platform,
					Operators:        converted.Delete.Operators,
					AdditionalImages: converted.Delete.AdditionalImages,
				},
			},
		}

		collector := &Collector{Log: log, Config: isc, Opts: opts, Fail: false}
		mockBatch := Batch{}
		_ = os.MkdirAll(testFolder+"/docker/registry/v2/repositories", 0o755)
		_ = os.MkdirAll(common.TestFolder+"cache-fake-temp", 0o755)
		defer os.RemoveAll(common.TestFolder + "cache-fake-temp")
		opts.LocalStorageFQDN = regCfg.HTTP.Addr

		ex := &DeleteSchema{
			ExecutorSchema: ExecutorSchema{
				Log:                 log,
				Opts:                &opts,
				Operator:            collector,
				Release:             collector,
				AdditionalImages:    collector,
				HelmCollector:       collector,
				Mirror:              mirrorMock,
				Batch:               &mockBatch,
				LocalStorageService: *reg,
				LogsDir:             "/tmp/",
				Delete:              MockDelete{},
				LocalStorageDisk:    common.TestFolder + "cache-fake-temp/",
			},
		}

		res := &cobra.Command{}
		res.SilenceUsage = true
		res.SetContext(context.Background())
		ex.Opts.Mode = mirror.MirrorToDisk

		// copy cache-fake to cache-fake-temp for testing
		err = copy.Copy(common.TestFolder+"cache-fake/", common.TestFolder+"cache-fake-temp/")
		assert.NoError(t, err)

		// test generate
		err = ex.RunDelete(res)
		assert.NoError(t, err)
	})
}
