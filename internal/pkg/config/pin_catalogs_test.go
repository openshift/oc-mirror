package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

const (
	// Valid SHA256 digests for testing (64 hex characters)
	testDigest1 = "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	testDigest2 = "fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321"

	// Catalog references
	redhatIndexTag          = "registry.redhat.io/redhat/redhat-operator-index:v4.12"
	redhatIndexTagDocker    = "docker://registry.redhat.io/redhat/redhat-operator-index:v4.12"
	certifiedIndexTag       = "registry.redhat.io/redhat/certified-operator-index:v4.12"
	certifiedIndexTagDocker = "docker://registry.redhat.io/redhat/certified-operator-index:v4.12"
	communityIndexBase      = "registry.redhat.io/redhat/community-operator-index"
	redhatIndexBase         = "registry.redhat.io/redhat/redhat-operator-index"

	// Test digests
	testDigestShort1 = "abc123def456789"
	testDigestShort2 = "digest1"
	testDigestShort3 = "digest2"
	testDigestShort4 = "abc123"

	// Kind constants
	kindISC  = "ImageSetConfiguration"
	kindDISC = "DeleteImageSetConfiguration"

	// API version
	apiVersion = "mirror.openshift.io/v2alpha1"

	// Filename patterns
	filenamePrefixISC  = "isc_pinned_"
	filenamePrefixDISC = "disc_pinned_"

	// Error messages
	errMsgParseCatalog   = "failed to parse catalog"
	errMsgWriteISC       = "failed to write pinned ISC"
	errMsgWriteDISC      = "failed to write pinned DISC"
	nonexistentDirectory = "/nonexistent/directory/path"
)

func TestPinCatalogDigests_Success(t *testing.T) {
	logger := clog.New("trace")

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: redhatIndexTag,
					},
				},
			},
		},
	}

	catalogMap := map[string]v2alpha1.CatalogFilterResult{
		redhatIndexTagDocker: {
			Digest: testDigestShort1,
		},
	}

	pinnedCfg, err := pinCatalogDigests(cfg, catalogMap, logger)
	require.NoError(t, err)
	assert.Equal(t,
		redhatIndexBase+"@sha256:"+testDigestShort1,
		pinnedCfg.Mirror.Operators[0].Catalog,
	)

	// Verify original config is unchanged
	assert.Equal(t,
		redhatIndexTag,
		cfg.Mirror.Operators[0].Catalog,
	)
}

func TestPinCatalogDigests_AlreadyPinned(t *testing.T) {
	logger := clog.New("trace")

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: redhatIndexBase + "@sha256:" + testDigest1,
					},
				},
			},
		},
	}

	catalogMap := map[string]v2alpha1.CatalogFilterResult{
		"docker://" + redhatIndexBase + "@sha256:" + testDigest1: {
			Digest: "newdigest123",
		},
	}

	pinnedCfg, err := pinCatalogDigests(cfg, catalogMap, logger)
	require.NoError(t, err)

	// Should remain unchanged since it's already pinned
	assert.Equal(t,
		redhatIndexBase+"@sha256:"+testDigest1,
		pinnedCfg.Mirror.Operators[0].Catalog,
	)
}

func TestPinCatalogDigests_CatalogNotInMap(t *testing.T) {
	logger := clog.New("trace")

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: redhatIndexTag,
					},
				},
			},
		},
	}

	// Empty catalog map
	catalogMap := map[string]v2alpha1.CatalogFilterResult{}

	pinnedCfg, err := pinCatalogDigests(cfg, catalogMap, logger)
	require.NoError(t, err)

	// Should remain unchanged when not found in map
	assert.Equal(t,
		redhatIndexTag,
		pinnedCfg.Mirror.Operators[0].Catalog,
	)
}

func TestPinCatalogDigests_EmptyDigest(t *testing.T) {
	logger := clog.New("trace")

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: redhatIndexTag,
					},
				},
			},
		},
	}

	catalogMap := map[string]v2alpha1.CatalogFilterResult{
		redhatIndexTagDocker: {
			Digest: "", // Empty digest
		},
	}

	pinnedCfg, err := pinCatalogDigests(cfg, catalogMap, logger)
	require.NoError(t, err)

	// Should remain unchanged when digest is empty
	assert.Equal(t,
		redhatIndexTag,
		pinnedCfg.Mirror.Operators[0].Catalog,
	)
}

func TestPinCatalogDigests_InvalidCatalog(t *testing.T) {
	logger := clog.New("trace")

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						// Empty catalog reference should fail parsing
						Catalog: "",
					},
				},
			},
		},
	}

	catalogMap := map[string]v2alpha1.CatalogFilterResult{}

	_, err := pinCatalogDigests(cfg, catalogMap, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), errMsgParseCatalog)
}

func TestPinCatalogDigests_NoOperators(t *testing.T) {
	logger := clog.New("trace")

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{},
			},
		},
	}

	catalogMap := map[string]v2alpha1.CatalogFilterResult{}

	pinnedCfg, err := pinCatalogDigests(cfg, catalogMap, logger)
	require.NoError(t, err)
	assert.Empty(t, pinnedCfg.Mirror.Operators)
}

func TestPinCatalogDigests_MultipleOperators(t *testing.T) {
	logger := clog.New("trace")

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: redhatIndexTag,
					},
					{
						Catalog: certifiedIndexTag,
					},
					{
						Catalog: communityIndexBase + "@sha256:" + testDigest2,
					},
				},
			},
		},
	}

	catalogMap := map[string]v2alpha1.CatalogFilterResult{
		redhatIndexTagDocker: {
			Digest: testDigestShort2,
		},
		certifiedIndexTagDocker: {
			Digest: testDigestShort3,
		},
	}

	pinnedCfg, err := pinCatalogDigests(cfg, catalogMap, logger)
	require.NoError(t, err)

	// First operator should be pinned
	assert.Equal(t,
		redhatIndexBase+"@sha256:"+testDigestShort2,
		pinnedCfg.Mirror.Operators[0].Catalog,
	)

	// Second operator should be pinned
	assert.Equal(t,
		"registry.redhat.io/redhat/certified-operator-index@sha256:"+testDigestShort3,
		pinnedCfg.Mirror.Operators[1].Catalog,
	)

	// Third operator already pinned, should remain unchanged
	assert.Equal(t,
		communityIndexBase+"@sha256:"+testDigest2,
		pinnedCfg.Mirror.Operators[2].Catalog,
	)
}

func TestPinCatalogDigests_OCITransport(t *testing.T) {
	logger := clog.New("trace")

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: "oci:///path/to/catalog",
					},
				},
			},
		},
	}

	catalogMap := map[string]v2alpha1.CatalogFilterResult{
		"oci:///path/to/catalog": {
			Digest: "ocidigests123",
		},
	}

	pinnedCfg, err := pinCatalogDigests(cfg, catalogMap, logger)
	require.NoError(t, err)

	// OCI transport should be preserved
	assert.Equal(t,
		"oci:///path/to/catalog@sha256:ocidigests123",
		pinnedCfg.Mirror.Operators[0].Catalog,
	)
}

func TestWritePinnedISC_Success(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: redhatIndexBase + "@sha256:" + testDigest1,
					},
				},
			},
		},
	}

	filePath, err := writePinnedISC(cfg, tmpDir)
	require.NoError(t, err)

	// Verify file was created
	assert.FileExists(t, filePath)

	// Verify file is in the correct directory
	assert.Equal(t, tmpDir, filepath.Dir(filePath))

	// Verify filename format
	filename := filepath.Base(filePath)
	assert.True(t, strings.HasPrefix(filename, filenamePrefixISC))
	assert.True(t, strings.HasSuffix(filename, ".yaml"))

	// Verify file contents can be read back
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)

	var loadedCfg v2alpha1.ImageSetConfiguration
	err = yaml.Unmarshal(data, &loadedCfg)
	require.NoError(t, err)

	// Verify TypeMeta is set
	assert.Equal(t, kindISC, loadedCfg.Kind)
	assert.Equal(t, apiVersion, loadedCfg.APIVersion)

	// Verify operator catalog is preserved
	assert.Equal(t,
		redhatIndexBase+"@sha256:"+testDigest1,
		loadedCfg.Mirror.Operators[0].Catalog,
	)
}

func TestWritePinnedISC_InvalidDirectory(t *testing.T) {
	cfg := v2alpha1.ImageSetConfiguration{}

	// Use a non-existent directory
	_, err := writePinnedISC(cfg, nonexistentDirectory)
	require.Error(t, err)
	assert.Contains(t, err.Error(), errMsgWriteISC)
}

func TestPinAndWriteConfigs(t *testing.T) {
	tmpDir := t.TempDir()
	logger := clog.New("trace")

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: redhatIndexTag,
					},
				},
			},
		},
	}

	catalogMap := map[string]v2alpha1.CatalogFilterResult{
		redhatIndexTagDocker: {
			Digest: testDigestShort1,
		},
	}

	iscPath, discPath, err := PinAndWriteISCAndDSC(cfg, catalogMap, tmpDir, logger)
	require.NoError(t, err)
	assert.FileExists(t, iscPath)
	assert.FileExists(t, discPath)

	// Verify ISC file
	iscData, err := os.ReadFile(iscPath)
	require.NoError(t, err)
	var loadedISC v2alpha1.ImageSetConfiguration
	err = yaml.Unmarshal(iscData, &loadedISC)
	require.NoError(t, err)
	assert.Equal(t, kindISC, loadedISC.Kind)
	assert.Equal(t,
		redhatIndexBase+"@sha256:"+testDigestShort1,
		loadedISC.Mirror.Operators[0].Catalog,
	)

	// Verify DISC file
	discData, err := os.ReadFile(discPath)
	require.NoError(t, err)
	var loadedDISC v2alpha1.DeleteImageSetConfiguration
	err = yaml.Unmarshal(discData, &loadedDISC)
	require.NoError(t, err)
	assert.Equal(t, kindDISC, loadedDISC.Kind)
	assert.Equal(t,
		redhatIndexBase+"@sha256:"+testDigestShort1,
		loadedDISC.Delete.Operators[0].Catalog,
	)
}

func TestCopyISC(t *testing.T) {
	original := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: redhatIndexTag,
					},
				},
			},
		},
	}

	copied := copyISC(original)

	// Modify the copied version
	copied.Mirror.Operators[0].Catalog = "modified-catalog"

	// Verify original is unchanged
	assert.Equal(t,
		redhatIndexTag,
		original.Mirror.Operators[0].Catalog,
	)

	// Verify copied has the modification
	assert.Equal(t,
		"modified-catalog",
		copied.Mirror.Operators[0].Catalog,
	)
}

func TestCreateDISCFromISC_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a pinned ISC (catalogs already have digests)
	pinnedISC := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: redhatIndexBase + "@sha256:" + testDigestShort1,
					},
				},
			},
		},
	}

	filePath, err := createDISCFromISC(pinnedISC, tmpDir)
	require.NoError(t, err)
	assert.FileExists(t, filePath)

	// Verify filename format
	filename := filepath.Base(filePath)
	assert.True(t, strings.HasPrefix(filename, filenamePrefixDISC))
	assert.True(t, strings.HasSuffix(filename, ".yaml"))

	// Read back and verify
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)

	var loadedDISC v2alpha1.DeleteImageSetConfiguration
	err = yaml.Unmarshal(data, &loadedDISC)
	require.NoError(t, err)

	// Verify TypeMeta
	assert.Equal(t, kindDISC, loadedDISC.Kind)
	assert.Equal(t, apiVersion, loadedDISC.APIVersion)

	// Verify operator catalog is pinned (copied from pinnedISC)
	assert.Equal(t,
		redhatIndexBase+"@sha256:"+testDigestShort1,
		loadedDISC.Delete.Operators[0].Catalog,
	)
}

func TestCreateDISCFromISC_MultipleOperators(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a pinned ISC (all catalogs already have digests)
	pinnedISC := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: redhatIndexBase + "@sha256:" + testDigestShort2,
					},
					{
						Catalog: "registry.redhat.io/redhat/certified-operator-index@sha256:" + testDigestShort3,
					},
					{
						Catalog: communityIndexBase + "@sha256:" + testDigest2,
					},
				},
			},
		},
	}

	filePath, err := createDISCFromISC(pinnedISC, tmpDir)
	require.NoError(t, err)

	// Read back and verify
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)

	var loadedDISC v2alpha1.DeleteImageSetConfiguration
	err = yaml.Unmarshal(data, &loadedDISC)
	require.NoError(t, err)

	// All operators should be pinned (copied from pinnedISC)
	assert.Equal(t,
		redhatIndexBase+"@sha256:"+testDigestShort2,
		loadedDISC.Delete.Operators[0].Catalog,
	)
	assert.Equal(t,
		"registry.redhat.io/redhat/certified-operator-index@sha256:"+testDigestShort3,
		loadedDISC.Delete.Operators[1].Catalog,
	)
	assert.Equal(t,
		communityIndexBase+"@sha256:"+testDigest2,
		loadedDISC.Delete.Operators[2].Catalog,
	)
}

func TestCreateDISCFromISC_AllSections(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a pinned ISC with all sections
	pinnedISC := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Platform: v2alpha1.Platform{
					Channels: []v2alpha1.ReleaseChannel{
						{
							Name: "stable-4.12",
						},
					},
				},
				Operators: []v2alpha1.Operator{
					{
						Catalog: redhatIndexBase + "@sha256:" + testDigestShort4,
					},
				},
				AdditionalImages: []v2alpha1.Image{
					{
						Name: "quay.io/example/image:latest",
					},
				},
				Helm: v2alpha1.Helm{
					Repositories: []v2alpha1.Repository{
						{
							Name: "my-repo",
							URL:  "https://example.com/charts",
						},
					},
				},
			},
		},
	}

	filePath, err := createDISCFromISC(pinnedISC, tmpDir)
	require.NoError(t, err)

	// Read back and verify
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)

	var loadedDISC v2alpha1.DeleteImageSetConfiguration
	err = yaml.Unmarshal(data, &loadedDISC)
	require.NoError(t, err)

	// Verify all sections are copied
	assert.Equal(t, "stable-4.12", loadedDISC.Delete.Platform.Channels[0].Name)
	assert.Equal(t, redhatIndexBase+"@sha256:"+testDigestShort4, loadedDISC.Delete.Operators[0].Catalog)
	assert.Equal(t, "quay.io/example/image:latest", loadedDISC.Delete.AdditionalImages[0].Name)
	assert.Equal(t, "my-repo", loadedDISC.Delete.Helm.Repositories[0].Name)
}

func TestCreateDISCFromISC_InvalidDirectory(t *testing.T) {
	pinnedISC := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: redhatIndexBase + "@sha256:" + testDigestShort4,
					},
				},
			},
		},
	}

	// Use a non-existent directory
	_, err := createDISCFromISC(pinnedISC, nonexistentDirectory)
	require.Error(t, err)
	assert.Contains(t, err.Error(), errMsgWriteDISC)
}
