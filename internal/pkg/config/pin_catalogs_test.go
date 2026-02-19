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
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
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

func TestPinCatalogDigests_CopyBehavior(t *testing.T) {
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
		image.WithDigest(redhatIndexBase, testDigestShort1),
		pinnedCfg.Mirror.Operators[0].Catalog,
	)

	// Verify original config is unchanged (tests copy behavior)
	assert.Equal(t,
		redhatIndexTag,
		cfg.Mirror.Operators[0].Catalog,
	)
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
						Catalog: image.WithDigest(communityIndexBase, testDigest2),
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
		image.WithDigest(redhatIndexBase, testDigestShort2),
		pinnedCfg.Mirror.Operators[0].Catalog,
	)

	// Second operator should be pinned
	assert.Equal(t,
		image.WithDigest("registry.redhat.io/redhat/certified-operator-index", testDigestShort3),
		pinnedCfg.Mirror.Operators[1].Catalog,
	)

	// Third operator already pinned, should remain unchanged
	assert.Equal(t,
		image.WithDigest(communityIndexBase, testDigest2),
		pinnedCfg.Mirror.Operators[2].Catalog,
	)
}

func TestWritePinnedISC_Success(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: image.WithDigest(redhatIndexBase, testDigest1),
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
		image.WithDigest(redhatIndexBase, testDigest1),
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
		image.WithDigest(redhatIndexBase, testDigestShort1),
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
		image.WithDigest(redhatIndexBase, testDigestShort1),
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
						Catalog: image.WithDigest(redhatIndexBase, testDigestShort1),
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
		image.WithDigest(redhatIndexBase, testDigestShort1),
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
						Catalog: image.WithDigest(redhatIndexBase, testDigestShort2),
					},
					{
						Catalog: image.WithDigest("registry.redhat.io/redhat/certified-operator-index", testDigestShort3),
					},
					{
						Catalog: image.WithDigest(communityIndexBase, testDigest2),
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
		image.WithDigest(redhatIndexBase, testDigestShort2),
		loadedDISC.Delete.Operators[0].Catalog,
	)
	assert.Equal(t,
		image.WithDigest("registry.redhat.io/redhat/certified-operator-index", testDigestShort3),
		loadedDISC.Delete.Operators[1].Catalog,
	)
	assert.Equal(t,
		image.WithDigest(communityIndexBase, testDigest2),
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
						Catalog: image.WithDigest(redhatIndexBase, testDigestShort4),
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
	assert.Equal(t, image.WithDigest(redhatIndexBase, testDigestShort4), loadedDISC.Delete.Operators[0].Catalog)
	assert.Equal(t, "quay.io/example/image:latest", loadedDISC.Delete.AdditionalImages[0].Name)
	assert.Equal(t, "my-repo", loadedDISC.Delete.Helm.Repositories[0].Name)
}

func TestCreateDISCFromISC_InvalidDirectory(t *testing.T) {
	pinnedISC := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: image.WithDigest(redhatIndexBase, testDigestShort4),
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

// Tests for pinSingleCatalogDigest function

func TestPinSingleCatalog_Success(t *testing.T) {
	logger := clog.New("trace")

	catalogMap := map[string]v2alpha1.CatalogFilterResult{
		redhatIndexTagDocker: {
			Digest: testDigestShort1,
		},
	}

	pinnedRef, err := pinSingleCatalogDigest(redhatIndexTag, catalogMap, logger)
	require.NoError(t, err)
	assert.Equal(t, image.WithDigest(redhatIndexBase, testDigestShort1), pinnedRef)
}

func TestPinSingleCatalog_AlreadyPinned(t *testing.T) {
	logger := clog.New("trace")

	alreadyPinnedCatalog := image.WithDigest(redhatIndexBase, testDigest1)

	catalogMap := map[string]v2alpha1.CatalogFilterResult{
		"docker://" + alreadyPinnedCatalog: {
			Digest: "newdigest123",
		},
	}

	pinnedRef, err := pinSingleCatalogDigest(alreadyPinnedCatalog, catalogMap, logger)
	require.NoError(t, err)
	assert.Empty(t, pinnedRef, "Should return empty string for already pinned catalog")
}

func TestPinSingleCatalog_NotInMap(t *testing.T) {
	logger := clog.New("trace")

	// Empty catalog map
	catalogMap := map[string]v2alpha1.CatalogFilterResult{}

	pinnedRef, err := pinSingleCatalogDigest(redhatIndexTag, catalogMap, logger)
	require.NoError(t, err)
	assert.Empty(t, pinnedRef, "Should return empty string when catalog not found in map")
}

func TestPinSingleCatalog_EmptyDigest(t *testing.T) {
	logger := clog.New("trace")

	catalogMap := map[string]v2alpha1.CatalogFilterResult{
		redhatIndexTagDocker: {
			Digest: "", // Empty digest
		},
	}

	pinnedRef, err := pinSingleCatalogDigest(redhatIndexTag, catalogMap, logger)
	require.NoError(t, err)
	assert.Empty(t, pinnedRef, "Should return empty string for empty digest")
}

func TestPinSingleCatalog_InvalidCatalog(t *testing.T) {
	logger := clog.New("trace")

	catalogMap := map[string]v2alpha1.CatalogFilterResult{}

	// Empty catalog reference should fail parsing
	pinnedRef, err := pinSingleCatalogDigest("", catalogMap, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), errMsgParseCatalog)
	assert.Empty(t, pinnedRef)
}

func TestPinSingleCatalog_OCITransport(t *testing.T) {
	logger := clog.New("trace")

	ociCatalog := "oci:///path/to/catalog"

	catalogMap := map[string]v2alpha1.CatalogFilterResult{
		ociCatalog: {
			Digest: "ocidigests123",
		},
	}

	pinnedRef, err := pinSingleCatalogDigest(ociCatalog, catalogMap, logger)
	require.NoError(t, err)
	assert.Equal(t, "oci://"+image.WithDigest("/path/to/catalog", "ocidigests123"), pinnedRef,
		"Should preserve OCI transport prefix")
}

func TestPinSingleCatalog_DockerTransport(t *testing.T) {
	logger := clog.New("trace")

	dockerCatalog := "docker://registry.redhat.io/redhat/redhat-operator-index:v4.12"

	catalogMap := map[string]v2alpha1.CatalogFilterResult{
		dockerCatalog: {
			Digest: testDigestShort1,
		},
	}

	pinnedRef, err := pinSingleCatalogDigest(dockerCatalog, catalogMap, logger)
	require.NoError(t, err)
	assert.Equal(t, image.WithDigest(redhatIndexBase, testDigestShort1), pinnedRef,
		"Should omit docker:// transport prefix")
}

func TestPinSingleCatalog_NoTransport(t *testing.T) {
	logger := clog.New("trace")

	// Catalog without explicit transport (docker:// is implied)
	catalogMap := map[string]v2alpha1.CatalogFilterResult{
		redhatIndexTagDocker: {
			Digest: testDigestShort1,
		},
	}

	pinnedRef, err := pinSingleCatalogDigest(redhatIndexTag, catalogMap, logger)
	require.NoError(t, err)
	assert.Equal(t, image.WithDigest(redhatIndexBase, testDigestShort1), pinnedRef,
		"Should not add transport prefix when none specified")
	assert.NotContains(t, pinnedRef, "docker://",
		"Should not include docker:// prefix in pinned reference")
}

func TestPinSingleCatalog_MultipleCatalogs(t *testing.T) {
	logger := clog.New("trace")

	catalogMap := map[string]v2alpha1.CatalogFilterResult{
		redhatIndexTagDocker: {
			Digest: testDigestShort1,
		},
		certifiedIndexTagDocker: {
			Digest: testDigestShort2,
		},
	}

	// Pin first catalog
	pinnedRef1, err := pinSingleCatalogDigest(redhatIndexTag, catalogMap, logger)
	require.NoError(t, err)
	assert.Equal(t, image.WithDigest(redhatIndexBase, testDigestShort1), pinnedRef1)

	// Pin second catalog
	pinnedRef2, err := pinSingleCatalogDigest(certifiedIndexTag, catalogMap, logger)
	require.NoError(t, err)
	assert.Equal(t, image.WithDigest("registry.redhat.io/redhat/certified-operator-index", testDigestShort2), pinnedRef2)
}
