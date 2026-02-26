package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/distribution/distribution/v3/registry"
	godigest "github.com/opencontainers/go-digest"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/config"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// TestExecutorRunPrepare
func TestDryRun(t *testing.T) {
	var imgs = []v2alpha1.CopyImageSchema{
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

	t.Run("Testing Executor : dryrun M2D should pass", func(t *testing.T) {
		testFolder := t.TempDir()
		defer os.RemoveAll(testFolder)

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
		defer os.RemoveAll(testFolder)

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
}

// createTestOCILayout creates a minimal OCI image layout on disk with a manifest list
// referencing three sub-manifests. It returns the OCI source string and the sub-digests.
func createTestOCILayout(t *testing.T, testFolder string) (string, []godigest.Digest) {
	t.Helper()

	ociSourcePath := filepath.Join(testFolder, "test-oci-source")
	err := os.MkdirAll(filepath.Join(ociSourcePath, specv1.ImageBlobsDir, "sha256"), 0o755)
	if err != nil {
		t.Fatalf("failed to create OCI directory structure: %v", err)
	}

	ociLayout, err := json.Marshal(specv1.ImageLayout{Version: specv1.ImageLayoutVersion})
	if err != nil {
		t.Fatalf("failed to marshal oci-layout: %v", err)
	}
	err = os.WriteFile(filepath.Join(ociSourcePath, specv1.ImageLayoutFile), ociLayout, 0o600)
	if err != nil {
		t.Fatalf("failed to write oci-layout: %v", err)
	}

	subDigests := []godigest.Digest{
		godigest.Digest("sha256:aaa1111111111111111111111111111111111111111111111111111111111111"),
		godigest.Digest("sha256:bbb2222222222222222222222222222222222222222222222222222222222222"),
		godigest.Digest("sha256:ccc3333333333333333333333333333333333333333333333333333333333333"),
	}

	manifestList := specv1.Index{
		MediaType: specv1.MediaTypeImageIndex,
		Manifests: []specv1.Descriptor{
			{MediaType: specv1.MediaTypeImageManifest, Digest: subDigests[0], Size: 1234, Platform: &specv1.Platform{Architecture: "amd64", OS: "linux"}},
			{MediaType: specv1.MediaTypeImageManifest, Digest: subDigests[1], Size: 5678, Platform: &specv1.Platform{Architecture: "arm64", OS: "linux"}},
			{MediaType: specv1.MediaTypeImageManifest, Digest: subDigests[2], Size: 9012, Platform: &specv1.Platform{Architecture: "ppc64le", OS: "linux"}},
		},
	}
	manifestList.SchemaVersion = 2

	manifestListBytes, err := json.Marshal(manifestList)
	if err != nil {
		t.Fatalf("failed to marshal manifest list: %v", err)
	}

	manifestListDigest := godigest.FromBytes(manifestListBytes)

	err = os.WriteFile(
		filepath.Join(ociSourcePath, specv1.ImageBlobsDir, "sha256", manifestListDigest.Encoded()),
		manifestListBytes, 0o600)
	if err != nil {
		t.Fatalf("failed to write manifest list blob: %v", err)
	}

	indexJSON := specv1.Index{
		MediaType: specv1.MediaTypeImageIndex,
		Manifests: []specv1.Descriptor{
			{
				MediaType: specv1.MediaTypeImageIndex,
				Digest:    manifestListDigest,
				Size:      int64(len(manifestListBytes)),
			},
		},
	}
	indexJSON.SchemaVersion = 2

	indexData, err := json.Marshal(indexJSON)
	if err != nil {
		t.Fatalf("failed to marshal index.json: %v", err)
	}
	err = os.WriteFile(filepath.Join(ociSourcePath, specv1.ImageIndexFile), indexData, 0o600)
	if err != nil {
		t.Fatalf("failed to write index.json: %v", err)
	}

	return "oci:" + ociSourcePath, subDigests
}

// TestDryRunWithManifestList tests that manifest list sub-digests are included in mapping.txt.
// It creates a proper OCI layout on disk as the source, with a manifest list blob
// referencing sub-manifests, and verifies the dry-run output includes all sub-digests.
func TestDryRunWithManifestList(t *testing.T) {
	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		SecurePolicy: false,
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	t.Run("Testing Executor : dryrun with manifest list should include sub-digests", func(t *testing.T) {
		testFolder := t.TempDir()

		global.WorkingDir = testFolder

		ociSource, subDigests := createTestOCILayout(t, testFolder)

		var imgs = []v2alpha1.CopyImageSchema{
			{
				Source:      ociSource,
				Destination: "docker://registry.example.com/namespace/multiarch-image:latest",
			},
			{
				Source:      "docker://registry.example.com/namespace/simple-image@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
				Destination: "oci:simple-image",
			},
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
			t.Fatalf("should not fail: %v", err)
		}

		mappingPath := filepath.Join(testFolder, dryRunOutDir, mappingFile)
		assert.FileExists(t, mappingPath)

		mappingBytes, err := os.ReadFile(mappingPath)
		if err != nil {
			t.Fatalf("failed to read mapping file: %v", err)
		}
		mapping := string(mappingBytes)

		expectedMapping := ociSource + "=" + imgs[0].Destination + "\n"
		for _, d := range subDigests {
			expectedMapping += ociSource + "@" + d.String() + "=" + imgs[0].Destination + "\n"
		}
		expectedMapping += imgs[1].Source + "=" + imgs[1].Destination + "\n"

		assert.Equal(t, expectedMapping, mapping)
	})
}
