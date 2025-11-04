package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/distribution/distribution/v3/registry"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/config"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	mirrormock "github.com/openshift/oc-mirror/v2/internal/pkg/mirror/mock"
)

// TestExecutorRunPrepare
func TestDryRun(t *testing.T) {
	imgs := []v2alpha1.CopyImageSchema{
		{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-h@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: "docker://registry/name/namespace/sometestimage-i@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
	}
	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		SecurePolicy: false,
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	t.Run("Testing Executor : dryrun M2D should pass", func(t *testing.T) {
		testFolder := t.TempDir()
		defer os.RemoveAll(testFolder)

		global.WorkingDir = testFolder

		// storage cache for test
		regCfg, err := setupRegForTest(testFolder)
		assert.NoError(t, err)
		reg, err := registry.NewRegistry(context.Background(), regCfg)
		assert.NoError(t, err)

		opts := &mirror.CopyOptions{
			Global:              global,
			DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
			SrcImage:            srcOpts,
			DestImage:           destOpts,
			RetryOpts:           retryOpts,
			IsDryRun:            true,
			Mode:                mirror.MirrorToDisk,
			Dev:                 false,
			LocalStorageFQDN:    regCfg.HTTP.Addr,
		}
		cfg := v2alpha1.ImageSetConfiguration{}
		// read the ImageSetConfiguration
		res, err := config.ReadConfig(opts.Global.ConfigPath, v2alpha1.ImageSetConfigurationKind)
		if err != nil {
			log.Error("imagesetconfig %v", err)
		} else {
			cfg = res.(v2alpha1.ImageSetConfiguration)
		}
		collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: false}

		mirrorMock := mirrormock.NewMockMirrorInterface(mockCtrl)

		mirrorMock.
			EXPECT().
			Check(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(true, nil).
			AnyTimes()

		ex := &ExecutorSchema{
			Log:                 log,
			Opts:                opts,
			Operator:            collector,
			Release:             collector,
			AdditionalImages:    collector,
			Mirror:              mirrorMock,
			LocalStorageService: *reg,
			LogsDir:             testFolder,
			MakeDir:             MakeDir{},
		}

		err = ex.DryRun(context.TODO(), imgs)
		assert.NoError(t, err)
		mappingPath := filepath.Join(testFolder, dryRunOutDir, "mapping.txt")
		assert.FileExists(t, mappingPath)

		mappingBytes, err := os.ReadFile(mappingPath)
		assert.NoError(t, err, "failed to read mapping file")
		mapping := string(mappingBytes)

		for _, img := range imgs {
			assert.Contains(t, mapping, img.Source+"="+img.Destination)
		}
	})

	t.Run("Testing Executor : dryrun M2D - errors finding images in cache - should generate missing.txt", func(t *testing.T) {
		testFolder := t.TempDir()

		global.WorkingDir = testFolder

		// storage cache for test
		regCfg, err := setupRegForTest(testFolder)
		assert.NoError(t, err)
		reg, err := registry.NewRegistry(context.Background(), regCfg)
		assert.NoError(t, err)

		opts := &mirror.CopyOptions{
			Global:              global,
			DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
			SrcImage:            srcOpts,
			DestImage:           destOpts,
			RetryOpts:           retryOpts,
			IsDryRun:            true,
			Mode:                mirror.MirrorToDisk,
			Dev:                 false,
			LocalStorageFQDN:    regCfg.HTTP.Addr,
		}
		cfg := v2alpha1.ImageSetConfiguration{}
		// read the ImageSetConfiguration
		res, err := config.ReadConfig(opts.Global.ConfigPath, v2alpha1.ImageSetConfigurationKind)
		if err != nil {
			log.Error("imagesetconfig %v ", err)
		} else {
			cfg = res.(v2alpha1.ImageSetConfiguration)
		}
		log.Debug("imagesetconfig : %v", cfg)
		collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: false}
		mirrorMock := mirrormock.NewMockMirrorInterface(mockCtrl)

		mirrorMock.
			EXPECT().
			Check(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(false, errors.New("force fail check")).
			AnyTimes()

		ex := &ExecutorSchema{
			Log:                 log,
			Opts:                opts,
			Operator:            collector,
			Release:             collector,
			AdditionalImages:    collector,
			Mirror:              mirrorMock,
			LocalStorageService: *reg,
			LogsDir:             "/tmp/",
			MakeDir:             MakeDir{},
		}

		err = ex.DryRun(context.TODO(), imgs)
		assert.NoError(t, err)
		mappingPath := filepath.Join(testFolder, dryRunOutDir, mappingFile)
		assert.FileExists(t, mappingPath)

		mappingBytes, err := os.ReadFile(mappingPath)
		assert.NoError(t, err)
		mapping := string(mappingBytes)

		for _, img := range imgs {
			assert.Contains(t, mapping, img.Source+"="+img.Destination)
		}

		missingImgsPath := filepath.Join(testFolder, dryRunOutDir, missingImgsFile)
		assert.FileExists(t, mappingPath)

		missingBytes, err := os.ReadFile(missingImgsPath)
		assert.NoError(t, err)
		missing := string(missingBytes)

		for _, img := range imgs {
			assert.Contains(t, missing, img.Source+"="+img.Destination)
		}
	})
}
