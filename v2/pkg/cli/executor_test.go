package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otiai10/copy"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/registry"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/config"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestExecutorMirroring - test both mirrorToDisk
// and diskToMirror, using mocks
func TestExecutorMirroring(t *testing.T) {
	testFolder := t.TempDir()
	defer os.RemoveAll(testFolder)
	defer os.Remove("../../pkg/cli/registry.log")

	workDir := filepath.Join(testFolder, "tests")
	//copy tests/hold-test-fake to working-dir
	err := copy.Copy("../../tests/working-dir-fake", workDir)
	if err != nil {
		t.Fatalf("should not fail to copy: %v", err)
	}
	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		TlsVerify:    false,
		SecurePolicy: false,
		Force:        true,
		WorkingDir:   workDir,
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
		Mode:                mirror.MirrorToDisk,
		Destination:         workDir,
	}

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
	cfg, err := config.ReadConfig(opts.Global.ConfigPath)
	if err != nil {
		log.Error("imagesetconfig %v ", err)
	}
	log.Debug("imagesetconfig : %v", cfg)

	nie := NormalStorageInterruptError{}
	nie.Is(fmt.Errorf("interrupt error"))

	t.Run("Testing Executor : mirrorToDisk should pass", func(t *testing.T) {
		collector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: opts}
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

	t.Run("Testing Executor : diskToMirror should pass", func(t *testing.T) {
		collector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: opts}
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
		collector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: opts}
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
	t.Run("Testing Executor : validate should pass", func(t *testing.T) {
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
		opts.Global.ConfigPath = "test"

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

		// check when using docker protocol --from should be used
		opts.Global.ConfigPath = "test"
		err = ex.Validate([]string{"docker://test"})
		assert.Equal(t, "when destination is docker://, diskToMirror workflow is assumed, and the --from argument is mandatory", err.Error())

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

	})
}

// TestExecutorComplete
func TestExecutorComplete(t *testing.T) {
	t.Run("Testing Executor : complete should pass", func(t *testing.T) {
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

		ex := &ExecutorSchema{
			Log:     log,
			Opts:    opts,
			MakeDir: MakeDir{},
			LogsDir: "/tmp/",
		}

		os.Setenv(cacheEnvVar, "/tmp/")

		// file protocol
		err := ex.Complete([]string{"file:///tmp/test"})
		if err != nil {
			t.Fatalf("should not fail")
		}

		// docker protocol
		err = ex.Complete([]string{"docker:///tmp/test"})
		if err != nil {
			t.Fatalf("should not fail")
		}

		defer os.RemoveAll("../../pkg/cli/test")
		defer os.RemoveAll("../../pkg/cli/tmp")
		defer os.RemoveAll("../../pkg/cli/working-dir")
	})
}

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
			Opts:    opts,
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
		cfg, err := config.ReadConfig(opts.Global.ConfigPath)
		if err != nil {
			log.Error("imagesetconfig %v ", err)
		}

		collector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: false}
		mockMirror := Mirror{}

		ex := &ExecutorSchema{
			Log:                          log,
			Opts:                         opts,
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

// TestExecutorLocalStorage
func TestExecutorSetupLocalStorage(t *testing.T) {
	t.Run("Testing Executor : setup local storage should pass", func(t *testing.T) {
		log := clog.New("trace")

		global := &mirror.GlobalOptions{
			TlsVerify:    false,
			SecurePolicy: false,
			Port:         7777,
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

		ex := &ExecutorSchema{
			Log:              log,
			Opts:             opts,
			LocalStorageDisk: "../../tests/cache-fake",
			MakeDir:          MockMakeDir{},
			LogsDir:          "/tmp/",
		}
		err := ex.setupLocalStorage()
		if err != nil {
			t.Fatalf("should not fail")
		}
	})
}

// TestExecutorSetupWorkingDir
func TestExecutorSetupWorkingDir(t *testing.T) {
	t.Run("Testing Executor : setup working dir should pass", func(t *testing.T) {
		log := clog.New("trace")

		global := &mirror.GlobalOptions{
			TlsVerify:    false,
			SecurePolicy: false,
			WorkingDir:   "/root",
		}

		opts := mirror.CopyOptions{
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
			TlsVerify:    false,
			SecurePolicy: false,
		}

		opts := mirror.CopyOptions{
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
	t.Run("Testing Executor : colelct all should pass", func(t *testing.T) {
		log := clog.New("trace")
		global := &mirror.GlobalOptions{
			TlsVerify:    false,
			SecurePolicy: false,
			Force:        true,
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
			Mode:                mirror.MirrorToDisk,
		}

		// read the ImageSetConfiguration
		cfg, _ := config.ReadConfig("../../tests/isc.yaml")
		failCollector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: true}
		collector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: false}

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

// setup mocks

type Mirror struct{}

// for this test scenario we only need to mock
// ReleaseImageCollector, OperatorImageCollector and Batchr
type Collector struct {
	Log    clog.PluggableLoggerInterface
	Config v1alpha2.ImageSetConfiguration
	Opts   mirror.CopyOptions
	Fail   bool
	Name   string
}

type Batch struct {
	Log    clog.PluggableLoggerInterface
	Config v1alpha2.ImageSetConfiguration
	Opts   mirror.CopyOptions
	Fail   bool
}

type Diff struct {
	Log    clog.PluggableLoggerInterface
	Config v1alpha2.ImageSetConfiguration
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

func (o Mirror) Check(ctx context.Context, dest string, opts *mirror.CopyOptions) (bool, error) {
	return true, nil
}

func (o Mirror) Run(context.Context, string, string, mirror.Mode, *mirror.CopyOptions, bufio.Writer) error {
	return nil
}

func (o MockMirrorUnArchiver) Unarchive() error {
	if o.Fail {
		return fmt.Errorf("forced unarchive error")
	}
	return nil
}

func (o MockMirrorUnArchiver) Close() error {

	return nil
}

func (o MockClusterResources) IDMS_ITMSGenerator(allRelatedImages []v1alpha3.CopyImageSchema, forceRepositoryScope bool) error {
	return nil
}
func (o MockClusterResources) UpdateServiceGenerator(graphImage, releaseImage string) error {
	return nil
}
func (o MockClusterResources) CatalogSourceGenerator(catalogImage string) error {
	return nil
}

func (o *Diff) DeleteImages(ctx context.Context) error {
	return nil
}

func (o *Diff) CheckDiff(prevCfg v1alpha2.ImageSetConfiguration) (bool, error) {
	return false, nil
}

func (o *Batch) Worker(ctx context.Context, images []v1alpha3.CopyImageSchema, opts mirror.CopyOptions) error {
	if o.Fail {
		return fmt.Errorf("forced error")
	}
	return nil
}

func (o *Collector) OperatorImageCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error) {
	if o.Fail {
		return []v1alpha3.CopyImageSchema{}, fmt.Errorf("forced error operator collector")
	}
	test := []v1alpha3.CopyImageSchema{
		{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
	}
	return test, nil
}

func (o *Collector) ReleaseImageCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error) {
	if o.Fail {
		return []v1alpha3.CopyImageSchema{}, fmt.Errorf("forced error release collector")
	}
	test := []v1alpha3.CopyImageSchema{
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

func (o *Collector) ReleaseImage() (string, error) {
	return "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64", nil
}

func (o *Collector) AdditionalImagesCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error) {
	if o.Fail {
		return []v1alpha3.CopyImageSchema{}, fmt.Errorf("forced error additionalImages collector")
	}
	test := []v1alpha3.CopyImageSchema{
		{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
	}
	return test, nil
}

func (o MockArchiver) BuildArchive(ctx context.Context, collectedImages []v1alpha3.CopyImageSchema) (string, error) {
	return filepath.Join(o.destination, "mirror_000001.tar"), nil
}

func (o MockArchiver) Close() error {
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
	configYamlV01 = fmt.Sprintf(configYamlV01, testFolder, 6000)
	config, err := configuration.Parse(bytes.NewReader([]byte(configYamlV01)))

	if err != nil {
		return &configuration.Configuration{}, fmt.Errorf("error parsing local storage configuration : %v\n %s", err, configYamlV01)
	}
	return config, nil
}
