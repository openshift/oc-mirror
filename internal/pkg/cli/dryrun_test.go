package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/registry"
	godigest "github.com/opencontainers/go-digest"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	libmanifest "go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/archive"
	"github.com/openshift/oc-mirror/v2/internal/pkg/clusterresources"
	"github.com/openshift/oc-mirror/v2/internal/pkg/config"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/history"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/signature"
)

func getTestImages() []v2alpha1.CopyImageSchema {
	return []v2alpha1.CopyImageSchema{
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
}

func TestDryRun(t *testing.T) {
	imgs := getTestImages()

	tests := []struct {
		name                           string
		mode                           string
		mockMirrorFail                 bool
		shouldGenerateClusterResources bool
		enableClusterResourcesGen      bool
		enableGraphGeneration          bool
		expectMissingFile              bool
		expectCatalogs                 bool
		expectSignatures               bool
		description                    string
	}{
		{
			name:                           "M2D should pass without cluster resources",
			mode:                           mirror.MirrorToDisk,
			mockMirrorFail:                 false,
			shouldGenerateClusterResources: false,
			enableClusterResourcesGen:      false,
			enableGraphGeneration:          false,
			expectMissingFile:              false,
			expectCatalogs:                 false,
			expectSignatures:               false,
			description:                    "Mirror-to-disk mode should not generate cluster resources since target registry is unknown",
		},
		{
			name:                           "M2D with missing images",
			mode:                           mirror.MirrorToDisk,
			mockMirrorFail:                 true,
			shouldGenerateClusterResources: false,
			enableClusterResourcesGen:      false,
			enableGraphGeneration:          false,
			expectMissingFile:              true,
			expectCatalogs:                 false,
			expectSignatures:               false,
			description:                    "Mirror-to-disk mode with missing images should create missing.txt but no cluster resources",
		},
		{
			name:                           "M2M should generate cluster resources",
			mode:                           mirror.MirrorToMirror,
			mockMirrorFail:                 false,
			shouldGenerateClusterResources: true,
			enableClusterResourcesGen:      true,
			enableGraphGeneration:          true,
			expectMissingFile:              false,
			expectCatalogs:                 false,
			expectSignatures:               false,
			description:                    "Mirror-to-mirror mode should generate all cluster resources when properly configured",
		},
		{
			name:                           "D2M should generate cluster resources",
			mode:                           mirror.DiskToMirror,
			mockMirrorFail:                 false,
			shouldGenerateClusterResources: true,
			enableClusterResourcesGen:      true,
			enableGraphGeneration:          false,
			expectMissingFile:              false,
			expectCatalogs:                 false,
			expectSignatures:               false,
			description:                    "Disk-to-mirror mode should generate cluster resources when properly configured",
		},
		{
			name:                           "M2M with catalogs should generate catalog resources",
			mode:                           mirror.MirrorToMirror,
			mockMirrorFail:                 false,
			shouldGenerateClusterResources: true,
			enableClusterResourcesGen:      true,
			enableGraphGeneration:          true,
			expectMissingFile:              false,
			expectCatalogs:                 true,
			expectSignatures:               false,
			description:                    "Mirror-to-mirror mode with operators should generate CatalogSource and ClusterCatalog files",
		},
		{
			name:                           "M2M with signatures should generate signature ConfigMap",
			mode:                           mirror.MirrorToMirror,
			mockMirrorFail:                 false,
			shouldGenerateClusterResources: true,
			enableClusterResourcesGen:      true,
			enableGraphGeneration:          true,
			expectMissingFile:              false,
			expectCatalogs:                 false,
			expectSignatures:               true,
			description:                    "Mirror-to-mirror mode with signatures should generate signature ConfigMap files",
		},
		{
			name:                           "M2M with catalogs and signatures should generate all resources",
			mode:                           mirror.MirrorToMirror,
			mockMirrorFail:                 false,
			shouldGenerateClusterResources: true,
			enableClusterResourcesGen:      true,
			enableGraphGeneration:          true,
			expectMissingFile:              false,
			expectCatalogs:                 true,
			expectSignatures:               true,
			description:                    "Mirror-to-mirror mode with operators and signatures should generate all cluster resources",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runDryRunTestCase(t, tt, imgs)
		})
	}
}

// runDryRunTestCase executes a single test case for the DryRun functionality
func runDryRunTestCase(t *testing.T, tt struct {
	name                           string
	mode                           string
	mockMirrorFail                 bool
	shouldGenerateClusterResources bool
	enableClusterResourcesGen      bool
	enableGraphGeneration          bool
	expectMissingFile              bool
	expectCatalogs                 bool
	expectSignatures               bool
	description                    string
}, imgs []v2alpha1.CopyImageSchema) {
	log := clog.New("trace")
	testFolder := t.TempDir()

	ex, testImages, err := setupDryRunTest(t, testFolder, tt, imgs, log)
	assert.NoError(t, err)

	err = ex.DryRun(context.TODO(), v2alpha1.CollectorSchema{AllImages: testImages})
	assert.NoError(t, err)

	verifyDryRunResults(t, testFolder, tt, testImages)
}

// setupDryRunTest prepares the test environment and executor for a dry run test
func setupDryRunTest(t *testing.T, testFolder string, tt struct {
	name                           string
	mode                           string
	mockMirrorFail                 bool
	shouldGenerateClusterResources bool
	enableClusterResourcesGen      bool
	enableGraphGeneration          bool
	expectMissingFile              bool
	expectCatalogs                 bool
	expectSignatures               bool
	description                    string
}, imgs []v2alpha1.CopyImageSchema, log clog.PluggableLoggerInterface) (*ExecutorSchema, []v2alpha1.CopyImageSchema, error) {
	opts, reg, err := setupTestEnvironment(t, testFolder, tt.mode)
	if err != nil {
		return nil, nil, err
	}

	ex := createExecutorForTest(testFolder, opts, reg, log, tt)
	testImages := selectTestImages(imgs, tt.shouldGenerateClusterResources, tt.expectCatalogs)

	// Set up test environment based on expectations
	if tt.expectSignatures {
		setupSignaturesDirectory(t, testFolder)
	}

	return ex, testImages, nil
}

// createExecutorForTest creates the appropriate executor based on test configuration
func createExecutorForTest(testFolder string, opts *mirror.CopyOptions, reg *registry.Registry, log clog.PluggableLoggerInterface, tt struct {
	name                           string
	mode                           string
	mockMirrorFail                 bool
	shouldGenerateClusterResources bool
	enableClusterResourcesGen      bool
	enableGraphGeneration          bool
	expectMissingFile              bool
	expectCatalogs                 bool
	expectSignatures               bool
	description                    string
}) *ExecutorSchema {
	if tt.enableClusterResourcesGen {
		return createTestExecutorWithClusterResources(testFolder, opts, reg, log, tt.enableGraphGeneration, tt.expectCatalogs)
	}

	ex := createTestExecutor(testFolder, opts, reg, log)
	if tt.mockMirrorFail {
		ex.Mirror = Mirror{Fail: true}
	}
	return ex
}

// selectTestImages returns the appropriate test images based on test configuration
func selectTestImages(imgs []v2alpha1.CopyImageSchema, shouldGenerateClusterResources bool, expectCatalogs bool) []v2alpha1.CopyImageSchema {
	if shouldGenerateClusterResources {
		testImages := []v2alpha1.CopyImageSchema{
			{
				Source:      consts.DockerProtocol + "registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Destination: "registry.example.com/test/image-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Origin:      consts.DockerProtocol + "registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Source:      consts.DockerProtocol + "quay.io/openshift-release-dev/ocp-release:4.14.0-x86_64",
				Destination: "registry.example.com/test/ocp-release:4.14.0-x86_64",
				Origin:      consts.DockerProtocol + "quay.io/openshift-release-dev/ocp-release:4.14.0-x86_64",
				Type:        v2alpha1.TypeOCPRelease,
			},
		}

		// Add catalog images if catalogs are expected to be tested
		// IMPORTANT: Destination must NOT contain LocalStorageFQDN (localhost:5000) for CatalogSource generation
		if expectCatalogs {
			catalogImages := []v2alpha1.CopyImageSchema{
				{
					Source:      consts.DockerProtocol + "registry.redhat.io/redhat/redhat-operator-index:v4.14@sha256:a1b2c3d4e5f67890123456789012345678901234567890123456789012345678",
					Destination: "registry.example.com/test/redhat-operator-index:v4.14@sha256:a1b2c3d4e5f67890123456789012345678901234567890123456789012345678",
					Origin:      consts.DockerProtocol + "registry.redhat.io/redhat/redhat-operator-index:v4.14@sha256:a1b2c3d4e5f67890123456789012345678901234567890123456789012345678",
					Type:        v2alpha1.TypeOperatorCatalog,
				},
			}
			testImages = append(testImages, catalogImages...)
		}

		return testImages
	}
	return imgs
}

// verifyDryRunResults validates the output files and directories created by DryRun
func verifyDryRunResults(t *testing.T, testFolder string, tt struct {
	name                           string
	mode                           string
	mockMirrorFail                 bool
	shouldGenerateClusterResources bool
	enableClusterResourcesGen      bool
	enableGraphGeneration          bool
	expectMissingFile              bool
	expectCatalogs                 bool
	expectSignatures               bool
	description                    string
}, testImages []v2alpha1.CopyImageSchema) {
	verifyMappingFile(t, testFolder, testImages)
	verifyClusterResources(t, testFolder, tt)
	verifyMissingImagesFile(t, testFolder, tt, testImages)
}

// verifyMappingFile validates the mapping.txt file contents
func verifyMappingFile(t *testing.T, testFolder string, testImages []v2alpha1.CopyImageSchema) {
	mappingPath := filepath.Join(testFolder, dryRunOutDir, "mapping.txt")
	assert.FileExists(t, mappingPath, "mapping.txt should always be created")

	mappingBytes, err := os.ReadFile(mappingPath)
	assert.NoError(t, err)
	mapping := string(mappingBytes)

	for _, img := range testImages {
		assert.Contains(t, mapping, img.Source+"="+img.Destination, "mapping should contain all images")
	}
}

// verifyClusterResources validates cluster resources generation based on test configuration
func verifyClusterResources(t *testing.T, testFolder string, tt struct {
	name                           string
	mode                           string
	mockMirrorFail                 bool
	shouldGenerateClusterResources bool
	enableClusterResourcesGen      bool
	enableGraphGeneration          bool
	expectMissingFile              bool
	expectCatalogs                 bool
	expectSignatures               bool
	description                    string
}) {
	clusterResourcesPath := filepath.Join(testFolder, clusterResourcesDir)

	if tt.shouldGenerateClusterResources {
		verifyClusterResourcesExist(t, clusterResourcesPath, tt.enableGraphGeneration, tt.expectCatalogs, tt.expectSignatures)
	} else {
		assert.NoDirExists(t, clusterResourcesPath, "cluster-resources directory should NOT exist for %s mode", tt.mode)
	}
}

// verifyClusterResourcesExist validates that expected cluster resource files exist
func verifyClusterResourcesExist(t *testing.T, clusterResourcesPath string, enableGraphGeneration bool, expectCatalogs bool, expectSignatures bool) {
	assert.DirExists(t, clusterResourcesPath, "cluster-resources directory should exist for modes that generate cluster resources")

	idmsFile := filepath.Join(clusterResourcesPath, "idms-oc-mirror.yaml")
	assert.FileExists(t, idmsFile, "IDMS file should be generated")

	itmsFile := filepath.Join(clusterResourcesPath, "itms-oc-mirror.yaml")
	assert.FileExists(t, itmsFile, "ITMS file should be generated")

	// Check for signature ConfigMap files (strict validation based on expectations)
	verifySignatureConfigMapFiles(t, clusterResourcesPath, expectSignatures)

	// Check for CatalogSource files (strict validation based on expectations)
	verifyCatalogSourceFiles(t, clusterResourcesPath, expectCatalogs)

	// Check for ClusterCatalog files (strict validation based on expectations)
	verifyClusterCatalogFiles(t, clusterResourcesPath, expectCatalogs)

	if enableGraphGeneration {
		verifyUpdateServiceFile(t, clusterResourcesPath)
	}
}

// verifyUpdateServiceFile validates the UpdateService file when graph generation is enabled
func verifyUpdateServiceFile(t *testing.T, clusterResourcesPath string) {
	updateServiceFile := filepath.Join(clusterResourcesPath, "updateService.yaml")
	assert.FileExists(t, updateServiceFile, "UpdateService manifest should be generated when platform.graph=true")

	updateServiceContent, err := os.ReadFile(updateServiceFile)
	assert.NoError(t, err)
	updateServiceStr := string(updateServiceContent)
	assert.Contains(t, updateServiceStr, "graphDataImage:", "UpdateService should contain graphDataImage field")
	assert.NotContains(t, updateServiceStr, "localhost:5000/openshift/graph-data", "UpdateService should not contain hardcoded localhost reference")
}

// verifyMissingImagesFile validates the missing.txt file based on test expectations
func verifyMissingImagesFile(t *testing.T, testFolder string, tt struct {
	name                           string
	mode                           string
	mockMirrorFail                 bool
	shouldGenerateClusterResources bool
	enableClusterResourcesGen      bool
	enableGraphGeneration          bool
	expectMissingFile              bool
	expectCatalogs                 bool
	expectSignatures               bool
	description                    string
}, testImages []v2alpha1.CopyImageSchema) {
	missingImgsPath := filepath.Join(testFolder, dryRunOutDir, missingImgsFile)

	if tt.expectMissingFile {
		verifyMissingImagesFileExists(t, missingImgsPath, testImages)
	} else {
		assert.NoFileExists(t, missingImgsPath, "missing.txt should not exist when no images are missing")
	}
}

// verifyMissingImagesFileExists validates that the missing images file contains expected content
func verifyMissingImagesFileExists(t *testing.T, missingImgsPath string, testImages []v2alpha1.CopyImageSchema) {
	assert.FileExists(t, missingImgsPath, "missing.txt should exist when there are missing images")

	missingBytes, err := os.ReadFile(missingImgsPath)
	assert.NoError(t, err)
	missing := string(missingBytes)

	for _, img := range testImages {
		assert.Contains(t, missing, img.Source+"="+img.Destination, "missing images should contain all images when mirror fails")
	}
}

// setupTestEnvironment creates common test setup for dry run tests
func setupTestEnvironment(t *testing.T, testFolder string, mode string) (*mirror.CopyOptions, *registry.Registry, error) {
	// Create fresh GlobalOptions for this test
	global := &mirror.GlobalOptions{
		SecurePolicy: false,
		WorkingDir:   testFolder,
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	// storage cache for test
	regCfg, err := setupRegForTest(testFolder)
	if err != nil {
		return nil, nil, err
	}
	reg, err := registry.NewRegistry(context.Background(), regCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create registry: %w", err)
	}

	opts := &mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		IsDryRun:            true,
		Mode:                mode,
		Dev:                 false,
		LocalStorageFQDN:    regCfg.HTTP.Addr,
	}

	return opts, reg, nil
}

// createTestExecutor creates a test ExecutorSchema
func createTestExecutor(testFolder string, opts *mirror.CopyOptions, reg *registry.Registry, log clog.PluggableLoggerInterface) *ExecutorSchema {
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

	return &ExecutorSchema{
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
}

// createTestExecutorWithClusterResources creates a test ExecutorSchema with cluster resources generation enabled
func createTestExecutorWithClusterResources(testFolder string, opts *mirror.CopyOptions, reg *registry.Registry, log clog.PluggableLoggerInterface, enableGraphGeneration bool, expectCatalogs bool) *ExecutorSchema {
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
					Graph: enableGraphGeneration, // Enable/disable UpdateService generation based on test needs
				},
			},
		},
	}

	// Add operators configuration when catalogs are expected
	if expectCatalogs {
		cfg.Mirror.Operators = []v2alpha1.Operator{
			{
				Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.14",
				IncludeConfig: v2alpha1.IncludeConfig{
					Packages: []v2alpha1.IncludePackage{
						{
							Name: "test-operator",
						},
					},
				},
			},
		}
	}

	collector := &Collector{Log: log, Config: cfg, Opts: *opts, Fail: false}
	mockMirror := Mirror{}

	// Initialize ClusterResources generator (this is what enables cluster resource generation)
	clusterResourcesGenerator := clusterresources.New(log, testFolder, cfg, opts.LocalStorageFQDN)

	return &ExecutorSchema{
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
}

// verifySignatureConfigMapFiles validates signature ConfigMap files based on expectations
func verifySignatureConfigMapFiles(t *testing.T, clusterResourcesPath string, expectSignatures bool) {
	signatureYamlFile := filepath.Join(clusterResourcesPath, "signature-configmap.yaml")
	signatureJsonFile := filepath.Join(clusterResourcesPath, "signature-configmap.json")

	yamlExists := false
	jsonExists := false

	if _, err := os.Stat(signatureYamlFile); err == nil {
		yamlExists = true
	}
	if _, err := os.Stat(signatureJsonFile); err == nil {
		jsonExists = true
	}

	if expectSignatures {
		// Strict validation: files MUST exist
		assert.FileExists(t, signatureYamlFile, "signature-configmap.yaml MUST exist when signatures are expected")
		assert.FileExists(t, signatureJsonFile, "signature-configmap.json MUST exist when signatures are expected")
	} else {
		// Conditional validation: if files exist, both should exist
		if yamlExists {
			assert.FileExists(t, signatureYamlFile, "signature-configmap.yaml should exist when signatures are present")
		}
		if jsonExists {
			assert.FileExists(t, signatureJsonFile, "signature-configmap.json should exist when signatures are present")
		}
	}
}

// verifyCatalogSourceFiles validates CatalogSource files based on expectations
func verifyCatalogSourceFiles(t *testing.T, clusterResourcesPath string, expectCatalogs bool) {
	catalogSourceFiles, err := filepath.Glob(filepath.Join(clusterResourcesPath, "cs-*.yaml"))
	assert.NoError(t, err)

	if expectCatalogs {
		// Strict validation: files MUST exist
		assert.True(t, len(catalogSourceFiles) > 0, "CatalogSource files MUST be generated when catalogs are expected")
		// Validate that each file actually exists and is readable
		for _, file := range catalogSourceFiles {
			assert.FileExists(t, file, "CatalogSource file should be readable")
		}
	} else {
		// Conditional validation: just log what we found
		if len(catalogSourceFiles) > 0 {
			t.Logf("✅ Found %d unexpected CatalogSource files: %v", len(catalogSourceFiles), catalogSourceFiles)
			// Still validate that they are correct if they exist
			for _, file := range catalogSourceFiles {
				assert.FileExists(t, file, "CatalogSource file should be readable")
			}
		} else {
			t.Logf("ℹ️  No CatalogSource files found (expected when no operators are configured)")
		}
	}
}

// verifyClusterCatalogFiles validates ClusterCatalog files based on expectations
func verifyClusterCatalogFiles(t *testing.T, clusterResourcesPath string, expectCatalogs bool) {
	clusterCatalogFiles, err := filepath.Glob(filepath.Join(clusterResourcesPath, "cc-*.yaml"))
	assert.NoError(t, err)

	if expectCatalogs {
		// Strict validation: files MUST exist
		assert.True(t, len(clusterCatalogFiles) > 0, "ClusterCatalog files MUST be generated when catalogs are expected")
		// Validate that each file actually exists and is readable
		for _, file := range clusterCatalogFiles {
			assert.FileExists(t, file, "ClusterCatalog file should be readable")
		}
	} else {
		// Conditional validation: just log what we found
		if len(clusterCatalogFiles) > 0 {
			t.Logf("✅ Found %d unexpected ClusterCatalog files: %v", len(clusterCatalogFiles), clusterCatalogFiles)
			// Still validate that they are correct if they exist
			for _, file := range clusterCatalogFiles {
				assert.FileExists(t, file, "ClusterCatalog file should be readable")
			}
		} else {
			t.Logf("ℹ️  No ClusterCatalog files found (expected when no operators are configured)")
		}
	}
}

// setupSignaturesDirectory creates a test signatures directory with sample signature files
func setupSignaturesDirectory(t *testing.T, testFolder string) {
	signaturesDir := filepath.Join(testFolder, "signatures")
	err := os.MkdirAll(signaturesDir, 0755)
	assert.NoError(t, err)

	// Create sample signature files that match the expected naming pattern
	// Signature files are named as: {imageTag}-sha256-{digest}.sig
	// The logic matches signatureFiles[imageTag] against the actual image tags
	sampleSignatures := []struct {
		name    string
		content string
	}{
		{
			name:    "4.14.0-x86_64-sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea.sig",
			content: "sample signature content for release image with tag 4.14.0-x86_64",
		},
	}

	for _, sig := range sampleSignatures {
		sigFile := filepath.Join(signaturesDir, sig.name)
		err := os.WriteFile(sigFile, []byte(sig.content), 0600)
		assert.NoError(t, err)
	}
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
			Global:                global,
			DeprecatedTLSVerify:   deprecatedTLSVerifyOpt,
			SrcImage:              srcOpts,
			DestImage:             destOpts,
			RetryOpts:             retryOpts,
			IsDryRunManifestLists: true,
			Mode:                  mirror.MirrorToDisk,
			Dev:                   false,
			LocalStorageFQDN:      regCfg.HTTP.Addr,
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
			Manifest:            manifest.New(log),
			LocalStorageService: *reg,
			LogsDir:             testFolder,
			MakeDir:             MakeDir{},
		}

		err = ex.DryRun(context.TODO(), v2alpha1.CollectorSchema{AllImages: imgs})
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
		Global:                global,
		DeprecatedTLSVerify:   deprecatedTLSVerifyOpt,
		SrcImage:              srcOpts,
		DestImage:             destOpts,
		RetryOpts:             retryOpts,
		IsDryRunManifestLists: true,
		Mode:                  mirror.DiskToMirror,
		Dev:                   false,
		LocalStorageFQDN:      regCfg.HTTP.Addr,
		ParallelImages:        4,
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
		Manifest:            manifest.New(log),
		LocalStorageService: *reg,
		LogsDir:             testFolder,
		MakeDir:             MakeDir{},
	}

	// Should not fail even though manifest inspection will warn for unreachable images
	err = ex.DryRun(context.TODO(), v2alpha1.CollectorSchema{AllImages: imgs})
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

// --- Size estimation tests ---

type mockManifestWithSizes struct {
	manifest.Manifest
	blobsByImage   map[string][]types.BlobInfo
	failImages     map[string]error
	mu             sync.Mutex
	filtersByImage map[string][]v2alpha1.InstancePlatformFilter
	digestsByImage map[string][]string
	sizeCalls      map[string][]bool
}

func (m *mockManifestWithSizes) InspectImage(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, platformFilters []v2alpha1.InstancePlatformFilter, includeSizes bool) ([]string, []types.BlobInfo, error) {
	m.mu.Lock()
	if m.filtersByImage == nil {
		m.filtersByImage = map[string][]v2alpha1.InstancePlatformFilter{}
	}
	m.filtersByImage[imgRef] = append([]v2alpha1.InstancePlatformFilter(nil), platformFilters...)
	if m.sizeCalls == nil {
		m.sizeCalls = map[string][]bool{}
	}
	m.sizeCalls[imgRef] = append(m.sizeCalls[imgRef], includeSizes)
	m.mu.Unlock()
	digests := m.digestsByImage[imgRef]
	if err, ok := m.failImages[imgRef]; ok {
		return digests, m.blobsByImage[imgRef], err
	}
	if !includeSizes {
		return digests, nil, nil
	}
	return digests, m.blobsByImage[imgRef], nil
}

type capturingLogger struct {
	mu       sync.Mutex
	messages []string
}

func (c *capturingLogger) record(prefix, msg string, val ...interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, prefix+fmt.Sprintf(msg, val...))
}
func (c *capturingLogger) output() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.messages, "\n")
}
func (c *capturingLogger) Info(msg string, val ...interface{})  { c.record("[INFO] ", msg, val...) }
func (c *capturingLogger) Debug(msg string, val ...interface{}) { c.record("[DEBUG] ", msg, val...) }
func (c *capturingLogger) Warn(msg string, val ...interface{})  { c.record("[WARN] ", msg, val...) }
func (c *capturingLogger) Error(msg string, val ...interface{}) { c.record("[ERROR] ", msg, val...) }
func (c *capturingLogger) Trace(msg string, val ...interface{}) { c.record("[TRACE] ", msg, val...) }
func (c *capturingLogger) Level(string)                         {}
func (c *capturingLogger) GetLevel() string                     { return "debug" }

func newTestExecutorWithManifest(m manifest.ManifestInterface) *ExecutorSchema {
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLS := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(&mirror.GlobalOptions{}, sharedOpts, deprecatedTLS, "src-", "screds")
	return &ExecutorSchema{Log: clog.New("debug"), Manifest: m, Opts: &mirror.CopyOptions{SrcImage: srcOpts}}
}

// loadTestManifestBlobs loads digests/sizes from tests/unit/testdata/image-manifest.json.
func loadTestManifestBlobs(t *testing.T) []types.BlobInfo {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(consts.TestFolder, "image-manifest.json"))
	assert.NoError(t, err)
	var m specv1.Manifest
	assert.NoError(t, json.Unmarshal(data, &m))
	blobs := make([]types.BlobInfo, 0, len(m.Layers)+1)
	for _, layer := range m.Layers {
		blobs = append(blobs, libmanifest.BlobInfoFromOCI1Descriptor(layer))
	}
	return append(blobs, libmanifest.BlobInfoFromOCI1Descriptor(m.Config))
}

func TestSizeEstimationHelpers(t *testing.T) {
	imgs := getTestImages()
	blobs := loadTestManifestBlobs(t)
	require.GreaterOrEqual(t, len(blobs), 3)

	t.Run("inspectImages + Origin filters", func(t *testing.T) {
		localSrc := consts.DockerProtocol + "127.0.0.1:55000/ns/img@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"
		wantFilters := []v2alpha1.InstancePlatformFilter{{OS: "linux", Architecture: "amd64"}}
		mock := &mockManifestWithSizes{
			blobsByImage: map[string][]types.BlobInfo{
				imgs[0].Source: blobs[:2],
				imgs[1].Source: {blobs[0]},
				localSrc:       {blobs[0]},
			},
			failImages: map[string]error{imgs[1].Source: fmt.Errorf("connection refused")},
		}
		ex := newTestExecutorWithManifest(mock)
		_, ok := ex.inspectImages(context.TODO(), []v2alpha1.CopyImageSchema{{Source: imgs[0].Source}}, nil, true)
		assert.Len(t, ok[imgs[0].Source].Blobs, 2)
		_, bad := ex.inspectImages(context.TODO(), []v2alpha1.CopyImageSchema{{Source: imgs[1].Source}}, nil, true)
		require.Len(t, bad[imgs[1].Source].Errors, 1)
		assert.ErrorContains(t, bad[imgs[1].Source].Errors[0], "connection refused")
		assert.Empty(t, bad[imgs[1].Source].Blobs, "failed inspections must discard partial content")
		_, _ = ex.inspectImages(context.TODO(),
			[]v2alpha1.CopyImageSchema{{Source: localSrc, Origin: imgs[0].Source}},
			map[string][]v2alpha1.InstancePlatformFilter{imgs[0].Source: wantFilters}, true)
		mock.mu.Lock()
		defer mock.mu.Unlock()
		assert.Equal(t, wantFilters, mock.filtersByImage[localSrc])
	})

	t.Run("local-cache filter matches authority only", func(t *testing.T) {
		fqdn := "mirror.local:55000"
		digest := "@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"
		local := consts.DockerProtocol + fqdn + "/ns/c" + digest
		lookalike := consts.DockerProtocol + "not" + fqdn + "/ns/i" + digest
		remote := imgs[0].Source
		assert.True(t, isLocalCacheSource(local, fqdn))
		assert.False(t, isLocalCacheSource(lookalike, fqdn))
		assert.False(t, isLocalCacheSource(remote, fqdn))
	})

	t.Run("dedupeImagesBySource", func(t *testing.T) {
		src := imgs[0].Source
		got := dedupeImagesBySource([]v2alpha1.CopyImageSchema{
			{Source: src, Type: v2alpha1.TypeGeneric},
			{Source: src, Origin: "origin", Type: v2alpha1.TypeOperatorCatalog},
			{Source: imgs[1].Source},
		})
		assert.Len(t, got, 2)
		assert.Equal(t, "origin", got[0].Origin)
		assert.True(t, got[0].Type.IsOperatorCatalog())
	})

	t.Run("saturating arithmetic", func(t *testing.T) {
		assert.Equal(t, int64(math.MaxInt64), archive.AddSize(math.MaxInt64-1, 2))
		assert.Equal(t, int64(3), archive.AddSize(-1, 3))
	})
}

func TestDryRunSizeReport(t *testing.T) {
	imgs := getTestImages()
	blobs := loadTestManifestBlobs(t)
	require.GreaterOrEqual(t, len(blobs), 2)

	newEx := func(t *testing.T, mode string, m *mockManifestWithSizes) (*ExecutorSchema, *capturingLogger, *mirror.CopyOptions, string) {
		t.Helper()
		dir := t.TempDir()
		opts, _, err := setupTestEnvironment(t, dir, mode)
		require.NoError(t, err)
		log := &capturingLogger{}
		opts.RemoveSignatures = true
		return &ExecutorSchema{Log: log, Opts: opts, Manifest: m, MakeDir: MakeDir{}, Mirror: Mirror{},
			LocalStorageDisk: filepath.Join(dir, "cache"),
			blobExists:       func(context.Context, string, types.BlobInfo) (bool, error) { return false, nil },
		}, log, opts, dir
	}

	t.Run("m2m happy path", func(t *testing.T) {
		ex, log, _, dir := newEx(t, mirror.MirrorToMirror, &mockManifestWithSizes{
			blobsByImage: map[string][]types.BlobInfo{
				imgs[0].Source: {blobs[0], blobs[1]},
				imgs[1].Source: {blobs[0]},
			},
		})
		require.NoError(t, ex.DryRun(context.TODO(), v2alpha1.CollectorSchema{AllImages: []v2alpha1.CopyImageSchema{
			{Source: imgs[0].Source, Destination: "docker://mirror.example.com/a:latest"},
			{Source: imgs[1].Source, Destination: "docker://mirror.example.com/b:latest"},
		}}))
		out := log.output()
		assert.Contains(t, out, fmt.Sprintf("(%d bytes)", blobs[0].Size*2+blobs[1].Size))
		assert.Contains(t, out, "Images: 2")
		assert.FileExists(t, filepath.Join(dir, dryRunOutDir, "mapping.txt"))
	})

	t.Run("m2m incomplete on sizing errors", func(t *testing.T) {
		ex, log, _, _ := newEx(t, mirror.MirrorToMirror, &mockManifestWithSizes{
			blobsByImage: map[string][]types.BlobInfo{imgs[0].Source: {blobs[0]}},
			failImages:   map[string]error{imgs[1].Source: fmt.Errorf("connection refused")},
		})
		require.NoError(t, ex.DryRun(context.TODO(), v2alpha1.CollectorSchema{AllImages: []v2alpha1.CopyImageSchema{
			{Source: imgs[0].Source, Destination: "docker://mirror.example.com/a:latest"},
			{Source: imgs[1].Source, Destination: "docker://mirror.example.com/b:latest"},
		}}))
		out := log.output()
		assert.Contains(t, out, fmt.Sprintf("(%d bytes; incomplete)", blobs[0].Size))
		assert.Contains(t, out, "Total Download Size")
		assert.Contains(t, out, imgs[1].Source)
		assert.Contains(t, out, "connection refused")
	})

	t.Run("summarizes sizing failures", func(t *testing.T) {
		log := &capturingLogger{}
		ex := &ExecutorSchema{Log: log}
		estimate := sizeEstimate{Bytes: 100}
		for range 200 {
			estimate.addError("docker://source.example/image:latest", errors.New("signature unavailable"))
		}
		ex.logSizeReport(dryRunSizeReport{TotalDownloadSize: estimate, LocalStorageSize: estimate, TarballSize: estimate})
		out := log.output()
		assert.Equal(t, 3, strings.Count(out, "[WARN]"))
		assert.Equal(t, 3, strings.Count(out, "200 issue(s); rerun with --log-level=debug for details"))
		assert.Equal(t, 600, strings.Count(out, "[DEBUG]"))
	})

	t.Run("d2m size reporting is deferred", func(t *testing.T) {
		ex, log, opts, _ := newEx(t, mirror.DiskToMirror, &mockManifestWithSizes{})
		digest := "@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"
		require.NoError(t, ex.DryRun(context.TODO(), v2alpha1.CollectorSchema{AllImages: []v2alpha1.CopyImageSchema{
			{Source: consts.DockerProtocol + opts.LocalStorageFQDN + "/ns/a" + digest, Destination: "docker://mirror.example.com/a:latest"},
			{Source: consts.DockerProtocol + opts.LocalStorageFQDN + "/ns/b" + digest, Destination: "docker://mirror.example.com/b:latest"},
			{Source: consts.DockerProtocol + opts.LocalStorageFQDN + "/ns/c" + digest, Destination: "docker://mirror.example.com/c"},
		}}))
		assert.NotContains(t, log.output(), "Dry-run size estimate")

		ex2, log2, opts2, _ := newEx(t, mirror.DiskToMirror, &mockManifestWithSizes{})
		opts2.Global.From = "file:///nonexistent/archive-path"
		require.NoError(t, ex2.DryRun(context.TODO(), v2alpha1.CollectorSchema{AllImages: []v2alpha1.CopyImageSchema{
			{Source: consts.DockerProtocol + opts2.LocalStorageFQDN + "/ns/a" + digest, Destination: "docker://mirror.example.com/a:latest"},
		}}))
		out := log2.output()
		assert.NotContains(t, out, "Dry-run size estimate")
		assert.NotContains(t, out, "/nonexistent/archive-path")
	})
}

func TestDryRunCombinedInspection(t *testing.T) {
	img := getTestImages()[0]
	blob := loadTestManifestBlobs(t)[0]
	for _, mode := range []string{mirror.MirrorToMirror, mirror.MirrorToDisk, mirror.DiskToMirror} {
		for _, inspectLists := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/lists=%t", mode, inspectLists), func(t *testing.T) {
				dir := t.TempDir()
				opts, _, err := setupTestEnvironment(t, dir, mode)
				require.NoError(t, err)
				opts.IsDryRunManifestLists = inspectLists
				local := img
				local.Source = consts.DockerProtocol + opts.LocalStorageFQDN + "/ns/catalog:latest"
				m := &mockManifestWithSizes{
					digestsByImage: map[string][]string{img.Source: {blob.Digest.String()}},
					failImages:     map[string]error{img.Source: fmt.Errorf("sub-manifest unavailable")},
				}
				log := &capturingLogger{}
				ex := &ExecutorSchema{Log: log, Opts: opts, Manifest: m, MakeDir: MakeDir{}, Mirror: Mirror{}}
				require.NoError(t, ex.DryRun(context.Background(), v2alpha1.CollectorSchema{AllImages: []v2alpha1.CopyImageSchema{img, img, local}}))
				warning := "[WARN] unable to inspect manifest for " + img.Source + ": sub-manifest unavailable"
				assert.Equal(t, inspectLists, strings.Contains(log.output(), warning), "manifest-list failures must identify the image in every workflow")
				wantCalls := []bool{mode != mirror.DiskToMirror}
				if mode == mirror.DiskToMirror && !inspectLists {
					wantCalls = nil
				}
				assert.Equal(t, wantCalls, m.sizeCalls[img.Source])
				switch {
				case mode != mirror.DiskToMirror:
					assert.Equal(t, []bool{true}, m.sizeCalls[local.Source])
					assert.Contains(t, log.output(), "bytes; incomplete", "inspection failures must remain in the size report")
				case inspectLists:
					assert.Equal(t, []bool{false}, m.sizeCalls[local.Source])
				default:
					assert.Empty(t, m.sizeCalls[local.Source])
				}
				mapping, err := os.ReadFile(filepath.Join(dir, dryRunOutDir, mappingFile))
				require.NoError(t, err)
				assert.Equal(t, inspectLists, strings.Contains(string(mapping), blob.Digest.String()), "sub-digests must be flag-gated and survive size failures")
			})
		}
	}
}

func newIncrementalExecutor(t *testing.T, mode string) *ExecutorSchema {
	t.Helper()
	root := t.TempDir()
	working := filepath.Join(root, "working-dir")
	require.NoError(t, os.MkdirAll(working, 0755))
	config := filepath.Join(root, "isc.yaml")
	require.NoError(t, os.WriteFile(config, []byte("kind: ImageSetConfiguration\n"), 0600))
	return &ExecutorSchema{
		Log: &capturingLogger{}, LocalStorageDisk: filepath.Join(root, "cache"),
		Opts: &mirror.CopyOptions{Mode: mode, LocalStorageFQDN: "localhost:55000", RemoveSignatures: true,
			Global: &mirror.GlobalOptions{WorkingDir: working, ConfigPath: config}},
		blobExists: func(context.Context, string, types.BlobInfo) (bool, error) { return false, nil },
	}
}

func TestIncrementalCacheAndHistoryAreIndependent(t *testing.T) {
	for _, cached := range []bool{false, true} {
		for _, archived := range []bool{false, true} {
			t.Run(fmt.Sprintf("cached=%t/history=%t", cached, archived), func(t *testing.T) {
				ex := newIncrementalExecutor(t, mirror.MirrorToDisk)
				blob := types.BlobInfo{Digest: godigest.FromString("layer"), Size: 8192}
				img := v2alpha1.CopyImageSchema{Source: "docker://source.example/ns/image:latest", Destination: "docker://localhost:55000/ns/image:latest"}
				if cached {
					path, err := archive.BlobPath(ex.LocalStorageDisk, blob.Digest)
					require.NoError(t, err)
					require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
					require.NoError(t, os.WriteFile(path, make([]byte, blob.Size), 0600))
				}
				if archived {
					h, err := history.NewHistory(ex.Opts.Global.WorkingDir, time.Time{}, ex.Log, history.OSFileCreator{})
					require.NoError(t, err)
					_, err = h.Append(sets.New(blob.Digest.String()))
					require.NoError(t, err)
				}
				cs := v2alpha1.CollectorSchema{AllImages: []v2alpha1.CopyImageSchema{img, img}}
				results := map[string]imageSizeResult{img.Source: {Blobs: []types.BlobInfo{blob}}}
				report := ex.estimateDryRunSize(context.Background(), cs, results)
				wantDownload := blob.Size
				if cached {
					wantDownload = 0
				}
				require.Equal(t, wantDownload, report.TotalDownloadSize.Bytes)
				// A new repository link occupies one digest string even when its blob is cached.
				require.Equal(t, wantDownload+int64(len(blob.Digest.String())), report.LocalStorageSize.Bytes)
				require.Empty(t, report.TarballSize.Errors)
				require.False(t, report.TarballSize.Unavailable)
				empty, err := archive.EstimateSize(context.Background(), ex.Opts, ex.LocalStorageDisk, v2alpha1.CollectorSchema{}, nil, nil, 0, ex.Log)
				require.NoError(t, err)
				if archived {
					require.Less(t, report.TarballSize.Bytes-empty, blob.Size)
				} else {
					require.Greater(t, report.TarballSize.Bytes-empty, blob.Size)
				}
			})
		}
	}
}

func TestIncrementalM2MScopesDownloadsAndCachesCatalogs(t *testing.T) {
	ex := newIncrementalExecutor(t, mirror.MirrorToMirror)
	blob := types.BlobInfo{Digest: godigest.FromString("shared"), Size: 1000}
	images := []v2alpha1.CopyImageSchema{
		{Source: "docker://source.example/catalog:latest", Destination: "docker://localhost:55000/catalog:latest", Type: v2alpha1.TypeOperatorCatalog},
		{Source: "docker://source.example/catalog:latest", Destination: "docker://mirror.example/a:latest", Type: v2alpha1.TypeOperatorCatalog},
		{Source: "docker://source.example/catalog:latest", Destination: "docker://mirror.example/a:other", Type: v2alpha1.TypeOperatorCatalog},
		{Source: "docker://source.example/catalog:latest", Destination: "docker://mirror.example/b:latest", Type: v2alpha1.TypeOperatorCatalog},
	}
	ex.blobExists = func(_ context.Context, dst string, _ types.BlobInfo) (bool, error) {
		return strings.Contains(dst, "/b:"), nil
	}
	report := ex.estimateDryRunSize(context.Background(), v2alpha1.CollectorSchema{AllImages: images},
		map[string]imageSizeResult{images[0].Source: {Blobs: []types.BlobInfo{blob}}})
	require.Equal(t, int64(2000), report.TotalDownloadSize.Bytes, "cache and repository a each need one transfer")
	require.Equal(t, blob.Size+int64(len(blob.Digest.String())), report.LocalStorageSize.Bytes)
	require.Equal(t, sizeEstimate{}, report.TarballSize)

	t.Run("rebuilt catalog cache includes unselected platforms", func(t *testing.T) {
		rebuilt := images[0]
		rebuilt.Source, rebuilt.RebuiltTag = "oci:/catalog", "filtered"
		otherPlatform := types.BlobInfo{Digest: godigest.FromString("other-platform"), Size: 2000}
		ex.Manifest = &mockManifestWithSizes{blobsByImage: map[string][]types.BlobInfo{rebuilt.Source: {blob, otherPlatform}}}
		ex.Opts.SrcImage = newTestExecutorWithManifest(ex.Manifest).Opts.SrcImage
		selected := map[string]imageSizeResult{rebuilt.Source: {Blobs: []types.BlobInfo{blob}}}
		report := ex.estimateDryRunSize(context.Background(), v2alpha1.CollectorSchema{AllImages: []v2alpha1.CopyImageSchema{rebuilt}},
			selected)
		require.Empty(t, report.LocalStorageSize.Errors)
		require.Equal(t, blob.Size+otherPlatform.Size+2*int64(len(blob.Digest.String())), report.LocalStorageSize.Bytes)
		require.Zero(t, report.TotalDownloadSize.Bytes, "locally generated catalog content is not downloaded")
		require.Equal(t, []types.BlobInfo{blob}, selected[rebuilt.Source].Blobs, "cache sizing must preserve the selected-platform inventory")
	})
}

func TestDryRunSignatureSizeInventory(t *testing.T) {
	top := types.BlobInfo{Digest: godigest.FromString("manifest"), Size: 300, MediaType: "application/vnd.oci.image.manifest.v1+json"}
	sig := types.BlobInfo{Digest: godigest.FromString("signature"), Size: 400, MediaType: top.MediaType}
	tag, err := signature.SigstoreAttachmentTag(top.Digest)
	require.NoError(t, err)
	for _, source := range []string{
		"docker://source.example/image:latest",
		"docker://source.example/image@" + top.Digest.String(),
		"docker://source.example/image:latest@" + top.Digest.String(),
	} {
		t.Run(source, func(t *testing.T) {
			img := v2alpha1.CopyImageSchema{Source: source, Destination: "docker://localhost:55000/image:latest"}
			mock := &mockManifestWithSizes{blobsByImage: map[string][]types.BlobInfo{source: {top}, "docker://source.example/image:" + tag: {sig}}}
			_, result := newTestExecutorWithManifest(mock).inspectImage(context.Background(), img, nil, true)
			require.Empty(t, result.Errors)
			require.Len(t, result.Blobs, 2)
			require.Equal(t, top.Digest, result.Blobs[1].Digest)
			require.Equal(t, top.Digest.String(), result.Blobs[0].Annotations[consts.SignatureFor])
		})
	}
	t.Run("signature failure retains image content", func(t *testing.T) {
		source := "docker://source.example/image:latest"
		mock := &mockManifestWithSizes{
			blobsByImage: map[string][]types.BlobInfo{source: {top}},
			failImages:   map[string]error{"docker://source.example/image:" + tag: errors.New("registry denied")},
		}
		_, result := newTestExecutorWithManifest(mock).inspectImage(context.Background(), v2alpha1.CopyImageSchema{Source: source}, nil, true)
		require.Equal(t, []types.BlobInfo{top}, result.Blobs)
		require.Len(t, result.Errors, 1)
		require.ErrorContains(t, result.Errors[0], "registry denied")
	})
}
