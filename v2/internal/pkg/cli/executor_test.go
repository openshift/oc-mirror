package cli

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/otiai10/copy"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/registry"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
	"github.com/openshift/oc-mirror/v2/internal/pkg/config"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestExecutorMirroring - test both mirrorToDisk
// and diskToMirror, using mocks
func TestExecutorMirroring(t *testing.T) {
	testFolder := t.TempDir()
	defer os.RemoveAll(testFolder)
	defer os.Remove("../../pkg/cli/registry.log")

	workDir := filepath.Join(testFolder, "tests")
	//copy tests/hold-test-fake to working-dir
	err := copy.Copy(common.TestFolder+"working-dir-fake", workDir)
	if err != nil {
		t.Fatalf("should not fail to copy: %v", err)
	}
	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		SecurePolicy: false,
		Force:        true,
		WorkingDir:   workDir,
	}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

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

	cr := MockClusterResources{}
	opts := &mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
		Destination:         workDir,
		LocalStorageFQDN:    regCfg.HTTP.Addr,
	}
	// read the ImageSetConfiguration
	res, err := config.ReadConfig(opts.Global.ConfigPath, v2alpha1.ImageSetConfigurationKind)
	if err != nil {
		log.Error("imagesetconfig %v ", err)
	}
	var cfg v2alpha1.ImageSetConfiguration
	if res == nil {
		cfg = v2alpha1.ImageSetConfiguration{}
	} else {
		cfg = res.(v2alpha1.ImageSetConfiguration)
		log.Debug("imagesetconfig : %v", cfg)
	}

	nie := NormalStorageInterruptError{}
	nie.Is(fmt.Errorf("interrupt error"))

	t.Run("Testing Executor : mirrorToDisk should pass", func(t *testing.T) {
		collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: *opts}
		archiver := MockArchiver{opts.Destination}

		ex := &ExecutorSchema{
			Log:                          log,
			Config:                       cfg,
			Opts:                         opts,
			Operator:                     collector,
			Release:                      collector,
			AdditionalImages:             collector,
			Batch:                        batch,
			MirrorArchiver:               archiver,
			LocalStorageService:          *reg,
			localStorageInterruptChannel: fakeStorageInterruptChan,
			MakeDir:                      MakeDir{},
			LogsDir:                      "/tmp/",
			ClusterResources:             cr,
		}

		res := &cobra.Command{}
		res.SetContext(context.Background())
		res.SilenceUsage = true
		ex.Opts.Mode = mirror.MirrorToDisk
		err := ex.Run(res, []string{"file://" + testFolder})
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
	})

	t.Run("Testing Executor : mirrorToDisk --dry-run should pass", func(t *testing.T) {
		opts.IsDryRun = true

		collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: *opts}
		archiver := MockArchiver{opts.Destination}

		ex := &ExecutorSchema{
			Log:                          log,
			Config:                       cfg,
			Opts:                         opts,
			Operator:                     collector,
			Release:                      collector,
			AdditionalImages:             collector,
			Batch:                        batch,
			MirrorArchiver:               archiver,
			Mirror:                       Mirror{},
			LocalStorageService:          *reg,
			localStorageInterruptChannel: fakeStorageInterruptChan,
			MakeDir:                      MakeDir{},
			LogsDir:                      "/tmp/",
			ClusterResources:             cr,
		}

		res := &cobra.Command{}
		res.SetContext(context.Background())
		res.SilenceUsage = true
		ex.Opts.Mode = mirror.MirrorToDisk
		err := ex.Run(res, []string{"file://" + testFolder})
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
		opts.IsDryRun = false
	})

	t.Run("Testing Executor : diskToMirror --dry-run should pass", func(t *testing.T) {
		opts.IsDryRun = true
		collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: *opts}
		archiver := MockMirrorUnArchiver{}
		cr := MockClusterResources{}
		cfg.Mirror.Platform.Graph = true

		ex := &ExecutorSchema{
			Log:                          log,
			Config:                       cfg,
			Opts:                         opts,
			Operator:                     collector,
			Release:                      collector,
			AdditionalImages:             collector,
			Batch:                        batch,
			Mirror:                       Mirror{},
			MirrorUnArchiver:             archiver,
			LocalStorageService:          *reg,
			localStorageInterruptChannel: fakeStorageInterruptChan,
			ClusterResources:             cr,
			MakeDir:                      MakeDir{},
			LogsDir:                      "/tmp/",
		}

		res := &cobra.Command{}
		res.SetContext(context.Background())
		res.SilenceUsage = true
		ex.Opts.Mode = mirror.DiskToMirror
		err := ex.Run(res, []string{"docker://test/test"})
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
		opts.IsDryRun = false
	})

	t.Run("Testing Executor : diskToMirror should pass", func(t *testing.T) {
		collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: *opts}
		archiver := MockMirrorUnArchiver{}
		cr := MockClusterResources{}
		cfg.Mirror.Platform.Graph = true

		ex := &ExecutorSchema{
			Log:                          log,
			Config:                       cfg,
			Opts:                         opts,
			Operator:                     collector,
			Release:                      collector,
			AdditionalImages:             collector,
			Batch:                        batch,
			Mirror:                       Mirror{},
			MirrorUnArchiver:             archiver,
			LocalStorageService:          *reg,
			localStorageInterruptChannel: fakeStorageInterruptChan,
			ClusterResources:             cr,
			MakeDir:                      MakeDir{},
			LogsDir:                      "/tmp/",
		}

		res := &cobra.Command{}
		res.SetContext(context.Background())
		res.SilenceUsage = true
		ex.Opts.Mode = mirror.DiskToMirror
		err := ex.Run(res, []string{"docker://test/test"})
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
	})

	t.Run("Testing Executor : diskToMirror should fail", func(t *testing.T) {
		collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: *opts}
		archiver := MockMirrorUnArchiver{Fail: true}
		cr := MockClusterResources{}
		cfg.Mirror.Platform.Graph = true

		ex := &ExecutorSchema{
			Log:                          log,
			Config:                       cfg,
			Opts:                         opts,
			Operator:                     collector,
			Release:                      collector,
			AdditionalImages:             collector,
			Batch:                        batch,
			Mirror:                       Mirror{},
			MirrorUnArchiver:             archiver,
			LocalStorageService:          *reg,
			localStorageInterruptChannel: fakeStorageInterruptChan,
			ClusterResources:             cr,
			MakeDir:                      MakeDir{},
			LogsDir:                      "/tmp/",
		}

		res := &cobra.Command{}
		res.SetContext(context.Background())
		res.SilenceUsage = true
		ex.Opts.Mode = mirror.DiskToMirror
		err := ex.Run(res, []string{"docker://test/test"})
		if err == nil {
			t.Fatalf("should fail")
		}
	})
}

func TestRunMirrorToMirror(t *testing.T) {
	testFolder := t.TempDir()
	defer os.RemoveAll(testFolder)
	defer os.Remove("../../pkg/cli/registry.log")

	workDir := filepath.Join(testFolder, "tests")
	//copy tests/hold-test-fake to working-dir
	err := copy.Copy(common.TestFolder+"working-dir-fake", workDir)
	if err != nil {
		t.Fatalf("should not fail to copy: %v", err)
	}
	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		SecurePolicy: false,
		Force:        true,
		WorkingDir:   workDir,
	}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

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

	opts := &mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Dev:                 false,
		Mode:                mirror.MirrorToMirror,
		Destination:         "docker://test",
		LocalStorageFQDN:    regCfg.HTTP.Addr,
	}

	// read the ImageSetConfiguration
	res, err := config.ReadConfig(opts.Global.ConfigPath, v2alpha1.ImageSetConfigurationKind)
	if err != nil {
		log.Error("imagesetconfig %v ", err)
	}
	var cfg v2alpha1.ImageSetConfiguration
	if res == nil {
		cfg = v2alpha1.ImageSetConfiguration{}
	} else {
		cfg = res.(v2alpha1.ImageSetConfiguration)
		log.Debug("imagesetconfig : %v", cfg)
	}

	log.Debug("imagesetconfig : %v", cfg)

	nie := NormalStorageInterruptError{}
	nie.Is(fmt.Errorf("interrupt error"))

	t.Run("Testing Executor : mirrorToMirror should pass", func(t *testing.T) {
		collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: *opts}
		cr := MockClusterResources{}

		ex := &ExecutorSchema{
			Log:                 log,
			Config:              cfg,
			Opts:                opts,
			Operator:            collector,
			Release:             collector,
			AdditionalImages:    collector,
			Mirror:              Mirror{},
			Batch:               batch,
			MakeDir:             MakeDir{},
			LogsDir:             "/tmp/",
			ClusterResources:    cr,
			LocalStorageService: *reg,
		}

		res := &cobra.Command{}
		res.SetContext(context.Background())
		res.SilenceUsage = true
		err := ex.Run(res, []string{"docker://test"})
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
	})

	t.Run("Testing Executor : mirrorToMirror --dry-run should pass", func(t *testing.T) {
		opts.IsDryRun = true
		collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: *opts}
		cr := MockClusterResources{}

		ex := &ExecutorSchema{
			Log:                 log,
			Config:              cfg,
			Opts:                opts,
			Operator:            collector,
			Release:             collector,
			AdditionalImages:    collector,
			Batch:               batch,
			MakeDir:             MakeDir{},
			LogsDir:             "/tmp/",
			ClusterResources:    cr,
			LocalStorageService: *reg,
		}

		res := &cobra.Command{}
		res.SetContext(context.Background())
		res.SilenceUsage = true
		err := ex.Run(res, []string{"docker://test"})
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
		opts.IsDryRun = false
	})

	t.Run("Testing Executor : mirrorToMirror --dry-run - failing collector: should fail", func(t *testing.T) {
		opts.IsDryRun = true
		collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: true}
		batch := &Batch{Log: log, Config: cfg, Opts: *opts}
		cr := MockClusterResources{}

		ex := &ExecutorSchema{
			Log:                 log,
			Config:              cfg,
			Opts:                opts,
			Operator:            collector,
			Release:             collector,
			AdditionalImages:    collector,
			Batch:               batch,
			MakeDir:             MakeDir{},
			LogsDir:             "/tmp/",
			ClusterResources:    cr,
			LocalStorageService: *reg,
		}

		res := &cobra.Command{}
		res.SetContext(context.Background())
		res.SilenceUsage = true
		err := ex.Run(res, []string{"docker://test"})
		assert.Error(t, err)
		opts.IsDryRun = false
	})
}

// TestNewMirrorCommand this covers both NewMirrorCmd and NewPrepareCommand
// we ignore any return values - we are only intersted in code coverage
func TestExecutorNewMirrorCommand(t *testing.T) {
	t.Run("Testing Executor : new mirror command should pass", func(t *testing.T) {
		log := clog.New("trace")
		NewMirrorCmd(log)
	})
}

// TestExecutorValidate
func TestExecutorValidate(t *testing.T) {
	t.Run("Testing Executor : maxParallelDownloads =2, validate should pass with warning", func(t *testing.T) {
		log := new(LogMock)

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
		opts.Global.ConfigPath = "test"

		ex := &ExecutorSchema{
			Log:     log,
			Opts:    opts,
			LogsDir: "/tmp/",
		}

		warnArgs := []interface{}{uint(2), uint(2), uint(10), uint(10)}
		log.On("Warn", "⚠️ --max-parallel-downloads set to %d: %d < %d. Flag ignored. Setting max-parallel-downloads = %d", warnArgs).Return(nil)

		opts.Global.LogLevel = "info"
		opts.Global.ConfigPath = "test"
		opts.Global.From = "" //reset
		opts.MaxParallelDownloads = 2
		opts.Global.WorkingDir = "file://test"
		assert.NoError(t, ex.Validate([]string{"docker://test"}))
		log.AssertExpectations(t)

	})

	t.Run("Testing Executor : maxParallelDownloads =20, validate should pass", func(t *testing.T) {
		log := new(LogMock)

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
		opts.Global.ConfigPath = "test"

		ex := &ExecutorSchema{
			Log:     log,
			Opts:    opts,
			LogsDir: "/tmp/",
		}
		opts.Global.LogLevel = "info"
		opts.Global.ConfigPath = "test"
		opts.Global.From = "" //reset
		opts.MaxParallelDownloads = 20
		opts.Global.WorkingDir = "file://test"
		assert.NoError(t, ex.Validate([]string{"docker://test"}))
		log.AssertNotCalled(t, "Warn", mock.Anything)
	})
	t.Run("Testing Executor : maxParallelDownloads =300, validate should pass with warning", func(t *testing.T) {
		log := new(LogMock)

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
		opts.Global.ConfigPath = "test"

		ex := &ExecutorSchema{
			Log:     log,
			Opts:    opts,
			LogsDir: "/tmp/",
		}
		warnArgs := []interface{}{uint(300), uint(300), uint(200), uint(200)}
		log.On("Warn", "⚠️ --max-parallel-downloads set to %d: %d > %d. Flag ignored. Setting max-parallel-downloads = %d", warnArgs).Return(nil)

		opts.Global.LogLevel = "info"
		opts.Global.ConfigPath = "test"
		opts.Global.From = "" //reset
		opts.MaxParallelDownloads = 300
		opts.Global.WorkingDir = "file://test"
		assert.NoError(t, ex.Validate([]string{"docker://test"}))
		log.AssertExpectations(t)

	})

	t.Run("Testing Executor : validate should pass", func(t *testing.T) {
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
		opts.Global.ConfigPath = "test"
		opts.Global.LogLevel = "info"

		ex := &ExecutorSchema{
			Log:     log,
			Opts:    opts,
			LogsDir: "/tmp/",
		}

		err := ex.Validate([]string{"file://test"})
		if err != nil {
			t.Fatalf("should not fail")
		}

		// check for config path error
		opts.Global.ConfigPath = ""
		err = ex.Validate([]string{"file://test"})
		assert.Equal(t, "use the --config flag it is mandatory", err.Error())

		// check when using file protocol --from should not be used
		opts.Global.ConfigPath = "test"
		opts.Global.From = "test"
		err = ex.Validate([]string{"file://test"})
		assert.Equal(t, "when destination is file://, mirrorToDisk workflow is assumed, and the --from argument is not needed", err.Error())

		// check when using --from protocol must be of type file://
		opts.Global.ConfigPath = "test"
		opts.Global.From = "test"
		err = ex.Validate([]string{"docker://test"})
		assert.Equal(t, "when --from is used, it must have file:// prefix", err.Error())

		// check destination protocol
		opts.Global.ConfigPath = "test"
		opts.Global.From = ""
		err = ex.Validate([]string{"test"})
		assert.Equal(t, "destination must have either file:// (mirror to disk) or docker:// (diskToMirror) protocol prefixes", err.Error())

		// check that since is a valid date
		opts.Global.ConfigPath = "test"
		opts.Global.From = ""
		opts.Global.SinceString = "2024-01-01"
		assert.NoError(t, ex.Validate([]string{"file://test"}))

		// check error is returned when since is an invalid date
		opts.Global.ConfigPath = "test"
		opts.Global.From = ""
		opts.Global.SinceString = "224-44-01"
		assert.Equal(t, "--since flag needs to be in format yyyy-MM-dd", ex.Validate([]string{"file://test"}).Error())

		// should not be able to use --workspace in mirror-to-disk workflow
		opts.Global.SinceString = "" //reset
		opts.Global.ConfigPath = "test"
		opts.Global.From = ""
		opts.Global.WorkingDir = "file://test"
		assert.Equal(t, "when destination is file://, mirrorToDisk workflow is assumed, and the --workspace argument is not needed", ex.Validate([]string{"file://test"}).Error())

		// should not be able to use --workspace and --from together at the same time
		opts.Global.ConfigPath = "test"
		opts.Global.From = "file://abc"
		opts.Global.WorkingDir = "file://test"
		assert.Equal(t, "when destination is docker://, --from (assumes disk to mirror workflow) and --workspace (assumes mirror to mirror workflow) cannot be used together", ex.Validate([]string{"docker://test"}).Error())

		// should be able to run mirror-to-mirror with a specific workingDir (--workspace)
		opts.Global.ConfigPath = "test"
		opts.Global.From = "" //reset
		opts.Global.WorkingDir = "file://test"
		assert.NoError(t, ex.Validate([]string{"docker://test"}))

		// should not be able to run mirror-to-mirror  without specifying workspace
		opts.Global.ConfigPath = "test"
		opts.Global.From = ""       //reset
		opts.Global.WorkingDir = "" //reset
		assert.Equal(t, "when destination is docker://, either --from (assumes disk to mirror workflow) or --workspace (assumes mirror to mirror workflow) need to be provided", ex.Validate([]string{"docker://test"}).Error())

	})
}

// TestExecutorComplete
func TestExecutorComplete(t *testing.T) {
	t.Run("Testing Executor : complete should pass", func(t *testing.T) {
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

		ex := &ExecutorSchema{
			Log:     log,
			Opts:    opts,
			MakeDir: MakeDir{},
			LogsDir: "/tmp/",
		}

		t.Setenv(cacheEnvVar, "/tmp/")

		// file protocol
		err := ex.Complete([]string{"file:///tmp/test"})
		if err != nil {
			t.Fatalf("should not fail")
		}

		// docker protocol - disk to mirror
		testFolder := t.TempDir()
		defer os.RemoveAll(testFolder)
		ex.Opts.Global.From = "file://" + testFolder
		err = ex.Complete([]string{"docker://tmp/test"})
		if err != nil {
			t.Fatalf("should not fail")
		}

		// docker protocol - mirror to mirror
		ex.Opts.Global.From = ""
		ex.Opts.Global.WorkingDir = "file://" + testFolder
		err = ex.Complete([]string{"docker://tmp/test"})
		if err != nil {
			t.Fatalf("should not fail")
		}
		assert.Equal(t, filepath.Join(testFolder, workingDir), ex.Opts.Global.WorkingDir)

		// diskToMirror - using since
		ex.Opts.Global.From = "file://" + testFolder
		ex.Opts.Global.WorkingDir = ""
		ex.Opts.Global.SinceString = "2024-01-01"
		err = ex.Complete([]string{"file:///tmp/test"})
		if err != nil {
			t.Fatalf("should not fail")
		}
		expectedSince, err := time.Parse(time.DateOnly, "2024-01-01")
		if err != nil {
			t.Fatalf("should not fail")
		}
		assert.Equal(t, expectedSince, ex.Opts.Global.Since)

		ex.Opts.Global.SinceString = "12345"
		assert.Error(t, ex.Complete([]string{"file:///tmp/test"}))

		defer os.RemoveAll("../../pkg/cli/test")
		defer os.RemoveAll("../../pkg/cli/tmp")
		defer os.RemoveAll("../../pkg/cli/working-dir")
	})
}

// TestExecutorLocalStorage
func TestExecutorSetupLocalStorage(t *testing.T) {
	t.Run("Testing Executor : setup local storage should pass", func(t *testing.T) {
		log := clog.New("trace")

		global := &mirror.GlobalOptions{
			SecurePolicy: false,
			Port:         7777,
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

		ex := &ExecutorSchema{
			Log:              log,
			Opts:             opts,
			LocalStorageDisk: common.TestFolder + "cache-fake",
			MakeDir:          MockMakeDir{},
			LogsDir:          "/tmp/",
		}
		err := ex.setupLocalStorage()
		if err != nil {
			t.Fatalf("should not fail %v", err)
		}
	})
}

// TestExecutorSetupWorkingDir
func TestExecutorSetupWorkingDir(t *testing.T) {
	workingDir := t.TempDir()
	defer os.RemoveAll(workingDir)
	t.Run("Testing Executor : setup working dir should pass", func(t *testing.T) {
		log := clog.New("trace")

		global := &mirror.GlobalOptions{
			SecurePolicy: false,
			WorkingDir:   workingDir,
		}

		opts := &mirror.CopyOptions{
			Global: global,
		}

		mkdir := MockMakeDir{}

		ex := &ExecutorSchema{
			Log:     log,
			Opts:    opts,
			MakeDir: mkdir,
		}

		err := ex.setupWorkingDir()
		if err != nil {
			t.Fatalf("should not fail")
		}

		ex.MakeDir = MockMakeDir{Fail: true, Dir: ""}
		err = ex.setupWorkingDir()
		assert.Equal(t, "forced mkdir working-dir error", err.Error())

		ex.MakeDir = MockMakeDir{Fail: true, Dir: signaturesDir}
		err = ex.setupWorkingDir()
		assert.Equal(t, "forced mkdir signatures error", err.Error())

		ex.MakeDir = MockMakeDir{Fail: true, Dir: releaseImageDir}
		err = ex.setupWorkingDir()
		assert.Equal(t, "forced mkdir release-images error", err.Error())

		ex.MakeDir = MockMakeDir{Fail: true, Dir: releaseImageExtractDir}
		err = ex.setupWorkingDir()
		assert.Equal(t, "forced mkdir hold-release error", err.Error())

		ex.MakeDir = MockMakeDir{Fail: true, Dir: operatorImageExtractDir}
		err = ex.setupWorkingDir()
		assert.Equal(t, "forced mkdir hold-operator error", err.Error())

	})
}

// TestExecutorSetupLogsLevelAndDir
func TestExecutorSetupLogsLevelAndDir(t *testing.T) {
	t.Run("Testing Executor : setup logs level and dir should pass", func(t *testing.T) {
		log := clog.New("trace")

		global := &mirror.GlobalOptions{
			SecurePolicy: false,
		}

		opts := &mirror.CopyOptions{
			Global: global,
		}

		mkdir := MockMakeDir{}

		ex := &ExecutorSchema{
			Log:     log,
			LogsDir: "/tmp/",
			Opts:    opts,
			MakeDir: mkdir,
		}

		err := ex.setupLogsLevelAndDir()
		if err != nil {
			t.Fatalf("should not fail")
		}

		ex.MakeDir = MockMakeDir{Fail: true, Dir: "logs"}
		err = ex.setupLogsLevelAndDir()
		assert.Equal(t, "forced mkdir logs error", err.Error())

	})
}

// TestExecutorCollectAll
func TestExecutorCollectAll(t *testing.T) {
	t.Run("Testing Executor : collect all should pass", func(t *testing.T) {
		log := clog.New("trace")
		global := &mirror.GlobalOptions{
			SecurePolicy: false,
			Force:        true,
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
			Mode:                mirror.MirrorToDisk,
		}

		// read the ImageSetConfiguration
		cfg, _ := config.ReadConfig(common.TestFolder+"isc.yaml", v2alpha1.ImageSetConfigurationKind)
		failCollector := &Collector{Log: log, Config: cfg.(v2alpha1.ImageSetConfiguration), Opts: *opts, Fail: true}
		collector := &Collector{Log: log, Config: cfg.(v2alpha1.ImageSetConfiguration), Opts: *opts, Fail: false}

		mkdir := MockMakeDir{}

		ex := &ExecutorSchema{
			Log:              log,
			LogsDir:          "/tmp/",
			Opts:             opts,
			MakeDir:          mkdir,
			Operator:         collector,
			Release:          failCollector,
			AdditionalImages: collector,
		}

		// force release error
		_, err := ex.CollectAll(context.Background())
		assert.Equal(t, "forced error release collector", err.Error())

		// force operator error
		ex.Operator = failCollector
		ex.Release = collector
		_, err = ex.CollectAll(context.Background())
		assert.Equal(t, "forced error operator collector", err.Error())

		// force additionalImages error
		ex.Operator = collector
		ex.Release = collector
		ex.AdditionalImages = failCollector
		_, err = ex.CollectAll(context.Background())
		assert.Equal(t, "forced error additionalImages collector", err.Error())

	})
}

func TestExcludeImages(t *testing.T) {
	allCollectedImages := []v2alpha1.CopyImageSchema{
		{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testa"},
		{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testb"},
		{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testc"},
		{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testd"},
		{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:teste"},
		{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testf"},
	}
	type testCase struct {
		caseName        string
		collectedImages []v2alpha1.CopyImageSchema
		blockedImages   []v2alpha1.Image
		expectedImages  []v2alpha1.CopyImageSchema
	}

	testCases := []testCase{
		{
			caseName:        "empty blocked images should pass",
			collectedImages: allCollectedImages,
			blockedImages:   []v2alpha1.Image{},
			expectedImages:  allCollectedImages,
		},
		{
			caseName:        "non matching blocked images should pass",
			collectedImages: allCollectedImages,
			blockedImages: []v2alpha1.Image{
				{
					Name: "registry/name/namespace/sometestimage-z@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				},
			},
			expectedImages: allCollectedImages,
		},
		{
			caseName:        "matching blocked images should pass",
			collectedImages: allCollectedImages,
			blockedImages: []v2alpha1.Image{
				{
					Name: "registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				},
				{
					Name: "registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				},
				{
					Name: "registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				},
			},
			expectedImages: []v2alpha1.CopyImageSchema{
				{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testb"},
				{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testd"},
				{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testf"},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			actualCollected := excludeImages(tc.collectedImages, tc.blockedImages)
			assert.ElementsMatch(t, tc.expectedImages, actualCollected)
		})
	}
}

func TestCalculateMaxBatchSize(t *testing.T) {
	t.Run("Testing CalculateMaxBatchSize with maxParallelDownloads = 0 should return 0", func(t *testing.T) {
		maxParallelDownloads := uint(0)
		expectedBatchSize := uint(1)

		batchSize := calculateMaxBatchSize(maxParallelDownloads, uint(0))

		assert.Equal(t, expectedBatchSize, batchSize)
	})

	t.Run("Testing CalculateMaxBatchSize with maxParallelDownloads = 1 should return 1", func(t *testing.T) {
		maxParallelDownloads := uint(1)
		expectedBatchSize := uint(1)

		batchSize := calculateMaxBatchSize(maxParallelDownloads, uint(0))

		assert.Equal(t, expectedBatchSize, batchSize)
	})

	t.Run("Testing CalculateMaxBatchSize with maxParallelDownloads = 8 should return 1", func(t *testing.T) {
		maxParallelDownloads := uint(8)
		expectedBatchSize := uint(1)

		batchSize := calculateMaxBatchSize(maxParallelDownloads, uint(0))

		assert.Equal(t, expectedBatchSize, batchSize)
	})
	t.Run("Testing CalculateMaxBatchSize with maxParallelDownloads = 50 should return 5", func(t *testing.T) {
		maxParallelDownloads := uint(50)
		expectedBatchSize := uint(5)

		batchSize := calculateMaxBatchSize(maxParallelDownloads, uint(0))

		assert.Equal(t, expectedBatchSize, batchSize)
	})
	t.Run("Testing CalculateMaxBatchSize with maxParallelDownloads = 400 should return 20", func(t *testing.T) {
		maxParallelDownloads := uint(400)
		expectedBatchSize := uint(20)

		batchSize := calculateMaxBatchSize(maxParallelDownloads, uint(0))

		assert.Equal(t, expectedBatchSize, batchSize)
	})

	t.Run("Testing CalculateMaxBatchSize with parallel-batch-images provided should match parallel-batch-images", func(t *testing.T) {
		maxParallelDownloads := uint(400)
		parallelBatchImages := uint(12)
		batchSize := calculateMaxBatchSize(maxParallelDownloads, parallelBatchImages)

		assert.Equal(t, parallelBatchImages, batchSize)
	})
}

// setup mocks

type Mirror struct {
	Fail bool
}

// for this test scenario we only need to mock
// ReleaseImageCollector, OperatorImageCollector and Batchr
type Collector struct {
	Log    clog.PluggableLoggerInterface
	Config v2alpha1.ImageSetConfiguration
	Opts   mirror.CopyOptions
	Fail   bool
	Name   string
}

type Batch struct {
	Log    clog.PluggableLoggerInterface
	Config v2alpha1.ImageSetConfiguration
	Opts   mirror.CopyOptions
	Fail   bool
}

type Diff struct {
	Log    clog.PluggableLoggerInterface
	Config v2alpha1.ImageSetConfiguration
	Opts   mirror.CopyOptions
	Mirror Mirror
	Fail   bool
}

type MockArchiver struct {
	destination string
}

type MockMirrorUnArchiver struct {
	Fail bool
}

type MockClusterResources struct {
}

type MockMakeDir struct {
	Fail bool
	Dir  string
}

func (o MockMakeDir) makeDirAll(dir string, mode os.FileMode) error {
	if o.Fail && len(o.Dir) == 0 {
		return fmt.Errorf("forced mkdir working-dir error")
	}
	if o.Fail && strings.Contains(dir, o.Dir) {
		return fmt.Errorf("forced mkdir %s error", o.Dir)
	}
	return nil
}

func (o Mirror) Check(ctx context.Context, dest string, opts *mirror.CopyOptions, asCopySrc bool) (bool, error) {
	if !o.Fail {
		return true, nil
	} else {
		return false, fmt.Errorf("fake error from check")
	}
}

func (o Mirror) Run(context.Context, string, string, mirror.Mode, *mirror.CopyOptions) error {
	return nil
}

func (o MockMirrorUnArchiver) Unarchive() error {
	if o.Fail {
		return fmt.Errorf("forced unarchive error")
	}
	return nil
}

func (o MockClusterResources) IDMS_ITMSGenerator(allRelatedImages []v2alpha1.CopyImageSchema, forceRepositoryScope bool) error {
	return nil
}
func (o MockClusterResources) UpdateServiceGenerator(graphImage, releaseImage string) error {
	return nil
}
func (o MockClusterResources) CatalogSourceGenerator(allRelatedImages []v2alpha1.CopyImageSchema) error {
	return nil
}
func (o MockClusterResources) GenerateSignatureConfigMap() error {
	return nil
}

func (o Batch) Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.CollectorSchema, error) {
	copiedImages := v2alpha1.CollectorSchema{
		AllImages:             []v2alpha1.CopyImageSchema{},
		TotalReleaseImages:    0,
		TotalOperatorImages:   0,
		TotalAdditionalImages: 0,
	}
	if o.Fail {
		return copiedImages, fmt.Errorf("forced error")
	}
	return collectorSchema, nil
}

func (o *Collector) OperatorImageCollector(ctx context.Context) (v2alpha1.CollectorSchema, error) {
	if o.Fail {
		return v2alpha1.CollectorSchema{}, fmt.Errorf("forced error operator collector")
	}
	test := []v2alpha1.CopyImageSchema{
		{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
	}
	return v2alpha1.CollectorSchema{AllImages: test}, nil
}

func (o *Collector) ReleaseImageCollector(ctx context.Context) ([]v2alpha1.CopyImageSchema, error) {
	if o.Fail {
		return []v2alpha1.CopyImageSchema{}, fmt.Errorf("forced error release collector")
	}
	test := []v2alpha1.CopyImageSchema{
		{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
	}
	return test, nil
}

func (o *Collector) GraphImage() (string, error) {
	return "localhost:5000/openshift/graph-image:latest", nil
}

func (o *Collector) ReleaseImage(ctx context.Context) (string, error) {
	return "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64", nil
}

func (o *Collector) AdditionalImagesCollector(ctx context.Context) ([]v2alpha1.CopyImageSchema, error) {
	if o.Fail {
		return []v2alpha1.CopyImageSchema{}, fmt.Errorf("forced error additionalImages collector")
	}
	test := []v2alpha1.CopyImageSchema{
		{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
	}
	return test, nil
}

func (o MockArchiver) BuildArchive(ctx context.Context, collectedImages []v2alpha1.CopyImageSchema) error {
	// return filepath.Join(o.destination, "mirror_000001.tar"), nil
	return nil
}

func skipSignalsToInterruptStorage(errchan chan error) {
	err := <-errchan
	if err != nil {
		fmt.Printf("registry communication channel received %v", err)
	}
}

func setupRegForTest(testFolder string) (*configuration.Configuration, error) {
	configYamlV01 := `
version: 0.1
log:
  accesslog:
    disabled: true
  level: error
  formatter: text
  fields:
    service: registry
storage:
  cache:
    blobdescriptor: inmemory
  filesystem:
    rootdirectory: %v
http:
  addr: :%d
  headers:
    X-Content-Type-Options: [nosniff]
health:
  storagedriver:
    enabled: false
`
	port := 5000
	for {
		addr := fmt.Sprintf("localhost:%d", port)
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			conn.Close()
			port++
		} else {
			break
		}
	}
	configYamlV01 = fmt.Sprintf(configYamlV01, testFolder, port)
	config, err := configuration.Parse(bytes.NewReader([]byte(configYamlV01)))

	if err != nil {
		return &configuration.Configuration{}, fmt.Errorf("error parsing local storage configuration : %v %s", err, configYamlV01)
	}
	return config, nil
}

type LogMock struct {
	level string
	mock.Mock
}

func (l *LogMock) Error(msg string, val ...interface{}) {}
func (l *LogMock) Info(msg string, val ...interface{})  {}
func (l *LogMock) Debug(msg string, val ...interface{}) {}
func (l *LogMock) Trace(msg string, val ...interface{}) {}
func (l *LogMock) Warn(msg string, val ...interface{}) {
	l.Called(msg, val)
	// l.MethodCalled()
}
func (l *LogMock) Level(level string) { l.level = level }
func (l *LogMock) GetLevel() string   { return l.level }
