package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/distribution/distribution/v3/registry"
	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/clusterresources"
	"github.com/openshift/oc-mirror/v2/internal/pkg/config"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// TestExecutorRunPrepare
func TestDryRun(t *testing.T) {
	imgs := []v2alpha1.CopyImageSchema{
		{Source: consts.DockerProtocol + "registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: consts.DockerProtocol + "registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: consts.DockerProtocol + "registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: consts.DockerProtocol + "registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: consts.DockerProtocol + "registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: consts.DockerProtocol + "registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: consts.DockerProtocol + "registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: consts.DockerProtocol + "registry/name/namespace/sometestimage-h@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		{Source: consts.DockerProtocol + "registry/name/namespace/sometestimage-i@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
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

	t.Run("Testing Executor : dryrun M2D should pass", func(t *testing.T) {
		testFolder := t.TempDir()

		global.WorkingDir = testFolder

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
			IsDryRun:            true,
			Mode:                mirror.MirrorToDisk,
			Dev:                 false,
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
		}
		collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: false}
		mockMirror := Mirror{}

		ex := &ExecutorSchema{
			Log:                 log,
			Opts:                opts,
			Operator:            collector,
			Release:             collector,
			AdditionalImages:    collector,
			Mirror:              mockMirror,
			LocalStorageService: *reg,
			LogsDir:             testFolder,
			MakeDir:             MakeDir{},
		}

		err = ex.DryRun(context.TODO(), imgs)
		if err != nil {
			t.Fatalf("should not fail")
		}
		mappingPath := filepath.Join(testFolder, dryRunOutDir, "mapping.txt")
		assert.FileExists(t, mappingPath)

		mappingBytes, err := os.ReadFile(mappingPath)
		if err != nil {
			t.Fatalf("failed to read mapping file: %v", err)
		}
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
			IsDryRun:            true,
			Mode:                mirror.MirrorToDisk,
			Dev:                 false,
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
		collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: false}
		mockMirror := Mirror{Fail: true}

		ex := &ExecutorSchema{
			Log:                 log,
			Opts:                opts,
			Operator:            collector,
			Release:             collector,
			AdditionalImages:    collector,
			Mirror:              mockMirror,
			LocalStorageService: *reg,
			LogsDir:             "/tmp/",
			MakeDir:             MakeDir{},
		}

		err = ex.DryRun(context.TODO(), imgs)
		if err != nil {
			t.Fatalf("should not fail")
		}
		mappingPath := filepath.Join(testFolder, dryRunOutDir, mappingFile)
		assert.FileExists(t, mappingPath)

		mappingBytes, err := os.ReadFile(mappingPath)
		if err != nil {
			t.Fatalf("failed to read mapping file: %v", err)
		}
		mapping := string(mappingBytes)

		for _, img := range imgs {
			assert.Contains(t, mapping, img.Source+"="+img.Destination)
		}

		missingImgsPath := filepath.Join(testFolder, dryRunOutDir, missingImgsFile)
		assert.FileExists(t, mappingPath)

		missingBytes, err := os.ReadFile(missingImgsPath)
		if err != nil {
			t.Fatalf("failed to read mapping file: %v", err)
		}
		missing := string(missingBytes)

		for _, img := range imgs {
			assert.Contains(t, missing, img.Source+"="+img.Destination)
		}
	})

	t.Run("Testing Executor : dryrun should generate cluster resources when ClusterResources is initialized", func(t *testing.T) {
		testFolder := t.TempDir()

		global.WorkingDir = testFolder

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
			IsDryRun:            true,
			Mode:                mirror.MirrorToDisk,
			Dev:                 false,
			LocalStorageFQDN:    regCfg.HTTP.Addr,
		}

		// Create a minimal ImageSetConfiguration for testing
		cfg := v2alpha1.ImageSetConfiguration{
			ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
				Mirror: v2alpha1.Mirror{
					Platform: v2alpha1.Platform{
						Channels: []v2alpha1.ReleaseChannel{
							{
								Name: "stable-4.14",
							},
						},
					},
				},
			},
		}

		collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: false}
		mockMirror := Mirror{}

		// Initialize ClusterResources generator (this is what enables cluster resource generation)
		clusterResourcesGenerator := clusterresources.New(log, testFolder, cfg, opts.LocalStorageFQDN)

		ex := &ExecutorSchema{
			Log:                 log,
			Opts:                opts,
			Config:              cfg,
			Operator:            collector,
			Release:             collector,
			AdditionalImages:    collector,
			Mirror:              mockMirror,
			LocalStorageService: *reg,
			LogsDir:             testFolder,
			MakeDir:             MakeDir{},
			ClusterResources:    clusterResourcesGenerator, // This is the key difference from other tests
		}

		// Add some images with different types to test different cluster resource generators
		imgsWithTypes := []v2alpha1.CopyImageSchema{
			{
				Source:      consts.DockerProtocol + "registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Destination: "localhost:5000/test/image-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Origin:      consts.DockerProtocol + "registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Source:      consts.DockerProtocol + "quay.io/openshift-release-dev/ocp-release:4.14.0-x86_64",
				Destination: "localhost:5000/test/ocp-release:4.14.0-x86_64",
				Origin:      consts.DockerProtocol + "quay.io/openshift-release-dev/ocp-release:4.14.0-x86_64",
				Type:        v2alpha1.TypeOCPRelease,
			},
		}

		err = ex.DryRun(context.TODO(), imgsWithTypes)
		if err != nil {
			t.Fatalf("DryRun should not fail: %v", err)
		}

		// Verify that the mapping file is still created (existing functionality)
		mappingPath := filepath.Join(testFolder, dryRunOutDir, "mapping.txt")
		assert.FileExists(t, mappingPath)

		// Verify that cluster-resources directory is created
		clusterResourcesPath := filepath.Join(testFolder, clusterResourcesDir)
		assert.DirExists(t, clusterResourcesPath)
	})
}
