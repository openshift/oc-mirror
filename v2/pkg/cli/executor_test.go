package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
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
)

func TestExecutor(t *testing.T) {
	testFolder := t.TempDir()
	defer os.RemoveAll(testFolder)

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

	// this test should cover over 80%

	t.Run("Testing Executor : should pass", func(t *testing.T) {
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

	t.Run("Testing Executor : should fail (batch worker)", func(t *testing.T) {
		collector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: opts, Fail: true}
		ex := &ExecutorSchema{
			Log:                          log,
			Config:                       cfg,
			Opts:                         opts,
			Operator:                     collector,
			Release:                      collector,
			AdditionalImages:             collector,
			Batch:                        batch,
			LocalStorageService:          *reg,
			localStorageInterruptChannel: fakeStorageInterruptChan,
		}

		res := &cobra.Command{}
		res.SilenceUsage = true
		res.SetContext(context.Background())
		ex.Opts.Mode = mirror.MirrorToDisk
		err := ex.Run(res, []string{"docker://test"})
		if err == nil {
			t.Fatalf("should fail")
		}
	})

	t.Run("Testing Executor : should fail (release collector)", func(t *testing.T) {
		releaseCollector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: true}
		operatorCollector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: false}
		batch := &Batch{Log: log, Config: cfg, Opts: opts, Fail: false}
		ex := &ExecutorSchema{
			Log:                          log,
			Config:                       cfg,
			Opts:                         opts,
			Operator:                     operatorCollector,
			Release:                      releaseCollector,
			AdditionalImages:             releaseCollector,
			Batch:                        batch,
			LocalStorageService:          *reg,
			localStorageInterruptChannel: fakeStorageInterruptChan,
		}

		res := &cobra.Command{}
		res.SilenceUsage = true
		res.SetContext(context.Background())
		ex.Opts.Mode = mirror.MirrorToDisk
		err := ex.Run(res, []string{"oci://test"})
		if err == nil {
			t.Fatalf("should fail")
		}
	})

	t.Run("Testing Executor : should fail (operator collector)", func(t *testing.T) {
		releaseCollector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: false}
		operatorCollector := &Collector{Log: log, Config: cfg, Opts: opts, Fail: true}
		batch := &Batch{Log: log, Config: cfg, Opts: opts, Fail: false}
		ex := &ExecutorSchema{
			Log:                          log,
			Config:                       cfg,
			Opts:                         opts,
			Operator:                     operatorCollector,
			Release:                      releaseCollector,
			AdditionalImages:             releaseCollector,
			Batch:                        batch,
			LocalStorageService:          *reg,
			localStorageInterruptChannel: fakeStorageInterruptChan,
		}

		res := &cobra.Command{}
		res.SilenceUsage = true
		res.SetContext(context.Background())
		ex.Opts.Mode = mirror.MirrorToDisk
		err := ex.Run(res, []string{"oci://test"})
		if err == nil {
			t.Fatalf("should fail")
		}
	})

	t.Run("Testing Executor : should pass", func(t *testing.T) {
		ex := &ExecutorSchema{
			Log:                          log,
			Config:                       cfg,
			Opts:                         opts,
			LocalStorageService:          *reg,
			localStorageInterruptChannel: fakeStorageInterruptChan,
		}
		res := NewMirrorCmd(log)
		res.SilenceUsage = true
		ex.Opts.Global.ConfigPath = "hello"
		err := ex.Validate([]string{"file://test"})
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
	})

	t.Run("Testing Executor : should fail", func(t *testing.T) {
		ex := &ExecutorSchema{
			Log:                          log,
			Config:                       cfg,
			Opts:                         opts,
			LocalStorageService:          *reg,
			localStorageInterruptChannel: fakeStorageInterruptChan,
		}
		res := NewMirrorCmd(log)
		res.SilenceUsage = true
		err := ex.Validate([]string{"test"})
		if err == nil {
			t.Fatalf("should fail")
		}
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
	configYamlV0_1 := `
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
      #auth:
      #htpasswd:
      #realm: basic-realm
      #path: /etc/registry
health:
  storagedriver:
    enabled: true
    interval: 10s
    threshold: 3
`
	configYamlV0_1 = fmt.Sprintf(configYamlV0_1, testFolder, 6000)
	config, err := configuration.Parse(bytes.NewReader([]byte(configYamlV0_1)))

	if err != nil {
		return &configuration.Configuration{}, fmt.Errorf("error parsing local storage configuration : %v\n %s", err, configYamlV0_1)
	}
	return config, nil
}
