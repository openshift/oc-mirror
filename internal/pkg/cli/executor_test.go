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
	// copy tests/hold-test-fake to working-dir
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
			Log:                 log,
			Config:              cfg,
			Opts:                opts,
			Operator:            collector,
			Release:             collector,
			AdditionalImages:    collector,
			HelmCollector:       collector,
			Batch:               batch,
			MirrorArchiver:      archiver,
			LocalStorageService: *reg,
			MakeDir:             MakeDir{},
			LogsDir:             "/tmp/",
			ClusterResources:    cr,
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
			Log:                 log,
			Config:              cfg,
			Opts:                opts,
			Operator:            collector,
			Release:             collector,
			AdditionalImages:    collector,
			HelmCollector:       collector,
			Batch:               batch,
			MirrorArchiver:      archiver,
			Mirror:              Mirror{},
			LocalStorageService: *reg,
			MakeDir:             MakeDir{},
			LogsDir:             "/tmp/",
			ClusterResources:    cr,
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
			Log:                 log,
			Config:              cfg,
			Opts:                opts,
			Operator:            collector,
			Release:             collector,
			AdditionalImages:    collector,
			HelmCollector:       collector,
			Batch:               batch,
			Mirror:              Mirror{},
			MirrorUnArchiver:    archiver,
			LocalStorageService: *reg,
			ClusterResources:    cr,
			MakeDir:             MakeDir{},
			LogsDir:             "/tmp/",
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
			Log:                 log,
			Config:              cfg,
			Opts:                opts,
			Operator:            collector,
			Release:             collector,
			AdditionalImages:    collector,
			HelmCollector:       collector,
			Batch:               batch,
			Mirror:              Mirror{},
			MirrorUnArchiver:    archiver,
			LocalStorageService: *reg,
			ClusterResources:    cr,
			MakeDir:             MakeDir{},
			LogsDir:             "/tmp/",
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
			Log:                 log,
			Config:              cfg,
			Opts:                opts,
			Operator:            collector,
			Release:             collector,
			AdditionalImages:    collector,
			HelmCollector:       collector,
			Batch:               batch,
			Mirror:              Mirror{},
			MirrorUnArchiver:    archiver,
			LocalStorageService: *reg,
			ClusterResources:    cr,
			MakeDir:             MakeDir{},
			LogsDir:             "/tmp/",
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
	// copy tests/hold-test-fake to working-dir
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
			HelmCollector:       collector,
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
			HelmCollector:       collector,
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
			HelmCollector:       collector,
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
	t.Run("Testing Executor : ParallelLayerImages = 5, validate should pass", func(t *testing.T) {
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
		opts.Global.From = "" // reset
		opts.ParallelLayerImages = 5
		opts.ParallelImages = 5
		opts.Global.WorkingDir = "file://test"
		assert.NoError(t, ex.Validate([]string{"docker://test"}))
		log.AssertNotCalled(t, "Warn", mock.Anything)

		// check that since is a valid date
		opts.Global.ConfigPath = "test"
		opts.Global.From = ""
		opts.Global.WorkingDir = ""
		opts.Global.SinceString = "2024-01-01"
		assert.NoError(t, ex.Validate([]string{"file://test"}))

		// should be able to run mirror-to-mirror with a specific workingDir (--workspace)
		opts.Global.ConfigPath = "test"
		opts.Global.From = "" // reset
		opts.Global.WorkingDir = "file://test"
		assert.NoError(t, ex.Validate([]string{"docker://test"}))

	})

	t.Run("Testing Executor : validate should fail", func(t *testing.T) {
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

		// check ParallelImages
		opts.ParallelImages = 11
		err := ex.Validate([]string{"file://test"})
		assert.EqualError(t, err, "the flag parallel-images must be between the range 1 to 10")

		opts.ParallelImages = 0
		err = ex.Validate([]string{"file://test"})
		assert.EqualError(t, err, "the flag parallel-images must be between the range 1 to 10")

		// check ParallelLayerImages
		opts.ParallelImages = 5
		opts.ParallelLayerImages = 11
		err = ex.Validate([]string{"file://test"})
		assert.EqualError(t, err, "the flag parallel-layers must be between the range 1 to 10")

		opts.ParallelLayerImages = 0
		err = ex.Validate([]string{"file://test"})
		assert.EqualError(t, err, "the flag parallel-layers must be between the range 1 to 10")

		opts.ParallelImages = 5
		opts.ParallelLayerImages = 4

		// check for config path error
		opts.Global.ConfigPath = ""
		err = ex.Validate([]string{"file://test"})
		assert.EqualError(t, err, "use the --config flag it is mandatory")

		// check when using file protocol --from should not be used
		opts.Global.ConfigPath = "test"
		opts.Global.From = "test"
		err = ex.Validate([]string{"file://test"})
		assert.EqualError(t, err, "when destination is file://, mirrorToDisk workflow is assumed, and the --from argument is not needed")

		// check when using --from protocol must be of type file://
		opts.Global.ConfigPath = "test"
		opts.Global.From = "test"
		err = ex.Validate([]string{"docker://test"})
		assert.EqualError(t, err, "when --from is used, it must have file:// prefix")

		// check destination protocol
		opts.Global.ConfigPath = "test"
		opts.Global.From = ""
		err = ex.Validate([]string{"test"})
		assert.EqualError(t, err, "destination must have either file:// (mirror to disk) or docker:// (diskToMirror) protocol prefixes")

		// check error is returned when since is an invalid date
		opts.Global.ConfigPath = "test"
		opts.Global.From = ""
		opts.Global.SinceString = "224-44-01"
		err = ex.Validate([]string{"file://test"})
		assert.EqualError(t, err, "--since flag needs to be in format yyyy-MM-dd")

		// should not be able to use --workspace in mirror-to-disk workflow
		opts.Global.SinceString = "" // reset
		opts.Global.ConfigPath = "test"
		opts.Global.From = ""
		opts.Global.WorkingDir = "file://test"
		err = ex.Validate([]string{"file://test"})
		assert.EqualError(t, err, "when destination is file://, mirrorToDisk workflow is assumed, and the --workspace argument is not needed")

		// should not be able to use --workspace and --from together at the same time
		opts.Global.ConfigPath = "test"
		opts.Global.From = "file://abc"
		opts.Global.WorkingDir = "file://test"
		err = ex.Validate([]string{"docker://test"})
		assert.EqualError(t, err, "when destination is docker://, --from (assumes disk to mirror workflow) and --workspace (assumes mirror to mirror workflow) cannot be used together")

		// should not be able to run mirror-to-mirror  without specifying workspace
		opts.Global.ConfigPath = "test"
		opts.Global.From = ""       // reset
		opts.Global.WorkingDir = "" // reset
		err = ex.Validate([]string{"docker://test"})
		assert.EqualError(t, err, "when destination is docker://, either --from (assumes disk to mirror workflow) or --workspace (assumes mirror to mirror workflow) need to be provided")

	})
}

// TestExecutorComplete
func TestExecutorComplete(t *testing.T) {
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

	t.Run("Testing Executor : complete should fail", func(t *testing.T) {
		// docker protocol - disk to mirror
		testFolder := t.TempDir()
		ex.Opts.Global.From = "file://" + testFolder
		err := ex.Complete([]string{"docker://tmp/test"})
		assert.ErrorContains(t, err, "no tar archives matching")
	})

	t.Run("Testing Executor : complete should pass", func(t *testing.T) {
		// file protocol
		err := ex.Complete([]string{"file:///tmp/test"})
		assert.NoError(t, err, "should pass with file protocol")

		testFolder := t.TempDir()

		// docker protocol - mirror to mirror
		ex.Opts.Global.From = ""
		ex.Opts.Global.WorkingDir = "file://" + testFolder
		err = ex.Complete([]string{"docker://tmp/test"})
		assert.NoError(t, err, "should pass with docker protocol - m2m")
		assert.Equal(t, filepath.Join(testFolder, workingDir), ex.Opts.Global.WorkingDir)

		// mirrorToDisk - using since
		ex.Opts.Global.From = "file://" + testFolder
		ex.Opts.Global.WorkingDir = ""
		ex.Opts.Global.SinceString = "2024-01-01"
		err = ex.Complete([]string{"file:///tmp/test"})
		assert.NoError(t, err, "should pass m2d with --since")
		expectedSince, err := time.Parse(time.DateOnly, "2024-01-01")
		assert.NoError(t, err, "should parse time")
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
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := ex.setupLocalStorage(ctx)
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
	})
}

// TestExecutorSetupLogsLevelAndDir
func TestExecutorSetupLogsLevelAndDir(t *testing.T) {
	t.Run("Testing Executor : setup logs level and dir should pass", func(t *testing.T) {
		tmpDir := t.TempDir()
		log := clog.New("trace")

		global := &mirror.GlobalOptions{
			SecurePolicy: false,
			WorkingDir:   tmpDir,
		}

		opts := &mirror.CopyOptions{
			Global: global,
		}

		ex := &ExecutorSchema{
			Log:        log,
			LogsDir:    tmpDir,
			Opts:       opts,
			MakeDir:    MakeDir{},
			WorkingDir: tmpDir,
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
	t.Run("Testing Executor : collect all", func(t *testing.T) {
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

		t.Run("should fail if release collection fails", func(t *testing.T) {
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
			assert.Equal(t, "collection error: forced error release collector", err.Error())
		})
		t.Run("should fail if operator collection fails", func(t *testing.T) {
			ex := &ExecutorSchema{
				Log:              log,
				LogsDir:          "/tmp/",
				Opts:             opts,
				MakeDir:          mkdir,
				Operator:         failCollector,
				Release:          collector,
				AdditionalImages: collector,
				HelmCollector:    collector,
			}

			// force operator error
			_, err := ex.CollectAll(context.Background())
			assert.Equal(t, "collection error: forced error operator collector", err.Error())
		})
		t.Run("should fail if additional images collection fails", func(t *testing.T) {
			ex := &ExecutorSchema{
				Log:              log,
				LogsDir:          "/tmp/",
				Opts:             opts,
				MakeDir:          mkdir,
				Operator:         collector,
				Release:          collector,
				AdditionalImages: failCollector,
				HelmCollector:    collector,
			}

			// force additionalImages error
			_, err := ex.CollectAll(context.Background())
			assert.Equal(t, "collection error: forced error additionalImages collector", err.Error())
		})
		t.Run("should fail if helm collection fails", func(t *testing.T) {
			ex := &ExecutorSchema{
				Log:              log,
				LogsDir:          "/tmp/",
				Opts:             opts,
				MakeDir:          mkdir,
				Operator:         collector,
				Release:          collector,
				AdditionalImages: collector,
				HelmCollector:    failCollector,
			}

			// force additionalImages error
			_, err := ex.CollectAll(context.Background())
			assert.Equal(t, "collection error: forced error helm collector", err.Error())
		})
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

func TestExecutorCheckRegistryAccess(t *testing.T) {
	const validRegistry = "localhost:5000"
	testFolder := t.TempDir()
	regCfg, err := setupRegForTest(testFolder)
	assert.NoError(t, err, "failed to parse local registry config")
	reg, err := registry.NewRegistry(context.Background(), regCfg)
	assert.NoError(t, err, "failed to create local registry service")
	global := &mirror.GlobalOptions{
		SecurePolicy: false,
		Force:        true,
		WorkingDir:   filepath.Join(testFolder, "tests"),
	}
	fsShared, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	fsDest, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	opts := &mirror.CopyOptions{
		Global:    global,
		DestImage: destOpts,
	}
	ex := &ExecutorSchema{
		Log:                 clog.New("debug"),
		LocalStorageService: *reg,
		Opts:                opts,
	}

	go ex.startLocalRegistry()
	// Make sure registry has started up
	time.Sleep(5 * time.Second)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ex.stopLocalRegistry(ctx)
	})

	t.Run("checkRegistryAccess should fail when", func(t *testing.T) {
		t.Run("cannot resolve registry hostname", func(t *testing.T) {
			const invalidRegistry = "invalid-registry-url.io"
			err := ex.checkRegistryAccess(context.TODO(), invalidRegistry)
			assert.ErrorContains(t, err, "no such host")
		})
		t.Run("registry is not accessible", func(t *testing.T) {
			const invalidRegistry = "localhost:9999"
			err := ex.checkRegistryAccess(context.TODO(), invalidRegistry)
			assert.ErrorContains(t, err, "connect: connection refused")
		})
		t.Run("http registry but tls-verify=true", func(t *testing.T) {
			err := ex.checkRegistryAccess(context.TODO(), validRegistry)
			assert.ErrorContains(t, err, "http: server gave HTTP response to HTTPS client")
		})
		t.Run("invalid creds", func(t *testing.T) {
			err := fsShared.Set("authfile", "/tmp/invalid-creds.json")
			assert.NoError(t, err, "should set flag")
			t.Cleanup(func() { _ = fsShared.Set("authfile", "") })
			err = ex.checkRegistryAccess(context.TODO(), "quay.io/redhat")
			assert.ErrorContains(t, err, "unable to retrieve auth token: invalid username/password: unauthorized")
		})
	})

	t.Run("checkRegistryAccess should succeed", func(t *testing.T) {
		t.Run("against local http registry when tls-verify=false ", func(t *testing.T) {
			err := fsDest.Set("dest-tls-verify", "false")
			assert.NoError(t, err, "should set flag")
			t.Cleanup(func() { _ = fsDest.Set("dest-tls-verify", "") })
			err = ex.checkRegistryAccess(context.TODO(), validRegistry)
			assert.NoError(t, err)
		})
		// TODO: add HTTPS and cert tests
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

type MockClusterResources struct{}

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

func (o MockClusterResources) GenerateSignatureConfigMap(allRelatedImages []v2alpha1.CopyImageSchema) error {
	return nil
}

func (o MockClusterResources) ClusterCatalogGenerator(allRelatedImages []v2alpha1.CopyImageSchema) error {
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

func (o *Collector) HelmImageCollector(ctx context.Context) ([]v2alpha1.CopyImageSchema, error) {
	if o.Fail {
		return []v2alpha1.CopyImageSchema{}, fmt.Errorf("forced error helm collector")
	}
	return []v2alpha1.CopyImageSchema{}, nil
}

func (o MockArchiver) BuildArchive(ctx context.Context, collectedImages []v2alpha1.CopyImageSchema) error {
	// return filepath.Join(o.destination, "mirror_000001.tar"), nil
	return nil
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
