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

		err = ex.DryRun(context.TODO(), imgs, nil)
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

		err = ex.DryRun(context.TODO(), imgs, nil)
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

	return consts.OciProtocolTrimmed + ociSourcePath, subDigests
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
				Destination: consts.DockerProtocol + "registry.example.com/namespace/multiarch-image:latest",
			},
			{
				Source:      consts.DockerProtocol + "registry.example.com/namespace/simple-image@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
				Destination: consts.OciProtocolTrimmed + "simple-image",
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

		err = ex.DryRun(context.TODO(), imgs, nil)
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
			// Sub-digest destinations are digest-pinned for docker:// destinations
			expectedMapping += ociSource + "@" + d.String() + "=" + consts.DockerProtocol + "registry.example.com/namespace/multiarch-image@" + d.String() + "\n"
		}
		expectedMapping += imgs[1].Source + "=" + imgs[1].Destination + "\n"

		assert.Equal(t, expectedMapping, mapping)
	})
}

// TestDryRunUnreachableImagesWarnButDontFail verifies that DryRun handles
// unreachable images gracefully: manifest inspection warns but doesn't fail,
// and base entries are still written to mapping.txt.
func TestDryRunUnreachableImagesWarnButDontFail(t *testing.T) {
	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		SecurePolicy: false,
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	testFolder := t.TempDir()
	global.WorkingDir = testFolder

	// These images point to a non-existent registry — manifest inspection will fail
	var imgs = []v2alpha1.CopyImageSchema{
		{Source: consts.DockerProtocol + "fake-registry.invalid/namespace/image1@sha256:aaa1111111111111111111111111111111111111111111111111111111111111", Destination: "oci:test1", Type: v2alpha1.TypeGeneric},
		{Source: consts.DockerProtocol + "fake-registry.invalid/namespace/image2@sha256:bbb2222222222222222222222222222222222222222222222222222222222222", Destination: "oci:test2", Type: v2alpha1.TypeGeneric},
	}

	regCfg, err := setupRegForTest(testFolder)
	if err != nil {
		t.Fatalf("storage cache error: %v", err)
	}
	reg, err := registry.NewRegistry(context.Background(), regCfg)
	if err != nil {
		t.Fatalf("storage cache error: %v", err)
	}

	opts := &mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		IsDryRun:            true,
		Mode:                mirror.DiskToMirror,
		Dev:                 false,
		LocalStorageFQDN:    regCfg.HTTP.Addr,
		ParallelImages:      4,
	}

	cfg := v2alpha1.ImageSetConfiguration{}
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

	// Should not fail even though manifest inspection will warn for unreachable images
	err = ex.DryRun(context.TODO(), imgs, nil)
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

	// Base entries should still be written despite inspection failures
	for _, img := range imgs {
		assert.Contains(t, mapping, img.Source+"="+img.Destination)
	}
}

func TestSubDigestDestination(t *testing.T) {
	tests := []struct {
		name     string
		dest     string
		digest   string
		expected string
	}{
		{
			name:     "docker destination with tag",
			dest:     "docker://registry.example.com/namespace/image:latest",
			digest:   "sha256:aaa1111111111111111111111111111111111111111111111111111111111111",
			expected: "docker://registry.example.com/namespace/image@sha256:aaa1111111111111111111111111111111111111111111111111111111111111",
		},
		{
			name:     "docker destination with port and tag",
			dest:     "docker://localhost:9999/namespace/image:v1.0",
			digest:   "sha256:bbb2222222222222222222222222222222222222222222222222222222222222",
			expected: "docker://localhost:9999/namespace/image@sha256:bbb2222222222222222222222222222222222222222222222222222222222222",
		},
		{
			name:     "docker destination with digest-as-tag",
			dest:     "docker://localhost:55000/openshift4/ose-kube-rbac-proxy:sha256-ac54cb8ff880a935ea3b4b1efc96d35bbf973342c450400d6417d06e59050027",
			digest:   "sha256:61d446b8b81cc1545ee805dbd46f921aecb1517c3478bdff654ab9a2a637845a",
			expected: "docker://localhost:55000/openshift4/ose-kube-rbac-proxy@sha256:61d446b8b81cc1545ee805dbd46f921aecb1517c3478bdff654ab9a2a637845a",
		},
		{
			name:     "oci destination kept as-is",
			dest:     "oci:test-image",
			digest:   "sha256:aaa1111111111111111111111111111111111111111111111111111111111111",
			expected: "oci:test-image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := subDigestDestination(tt.dest, tt.digest)
			assert.Equal(t, tt.expected, result)
		})
	}
}
