package config

import (
	"context"
	"testing"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// MockManifest provides a test double for ManifestInterface
// Based on the pattern from filtered_collector_test.go
type MockManifest struct {
	digestMap map[string]string // maps image reference to digest
}

func (m *MockManifest) ImageDigest(ctx context.Context, srcCtx *types.SystemContext, ref string) (string, error) {
	if m.digestMap != nil {
		if digest, ok := m.digestMap[ref]; ok {
			return digest, nil
		}
	}
	// Return empty string if not in map (catalog won't be pinned)
	return "", nil
}

// Stub implementations for unused ManifestInterface methods
func (m *MockManifest) GetOCIImageIndex(file string) (*v2alpha1.OCISchema, error) {
	return nil, nil
}
func (m *MockManifest) GetOCIImageManifest(file string) (*v2alpha1.OCISchema, error) {
	return nil, nil
}
func (m *MockManifest) GetOCIImageFromIndex(dir string) (gcrv1.Image, error) { //nolint:ireturn // as expected by go-containerregistry
	return nil, nil
}
func (m *MockManifest) ExtractOCILayers(img gcrv1.Image, toPath, label string) error {
	return nil
}
func (m *MockManifest) ConvertOCIIndexToSingleManifest(dir string, oci *v2alpha1.OCISchema) error {
	return nil
}
func (m *MockManifest) GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error) {
	return nil, nil
}
func (m *MockManifest) GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error) {
	return nil, nil
}
func (m *MockManifest) ImageManifest(ctx context.Context, srcCtx *types.SystemContext, ref string, instanceDigest *digest.Digest) ([]byte, string, error) {
	return nil, "", nil
}

const (
	// Valid SHA256 digests for testing (64 hex characters)
	testDigest1 = "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	testDigest2 = "fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321"

	// Catalog references
	redhatIndexTag          = "registry.redhat.io/redhat/redhat-operator-index:v4.12"
	redhatIndexTagDocker    = consts.DockerProtocol + "registry.redhat.io/redhat/redhat-operator-index:v4.12"
	certifiedIndexTag       = "registry.redhat.io/redhat/certified-operator-index:v4.12"
	certifiedIndexTagDocker = consts.DockerProtocol + "registry.redhat.io/redhat/certified-operator-index:v4.12"
	communityIndexBase      = "registry.redhat.io/redhat/community-operator-index"
	redhatIndexBase         = "registry.redhat.io/redhat/redhat-operator-index"

	// Test digests
	testDigestShort1 = "abc123def456789"
	testDigestShort2 = "digest1"
	testDigestShort3 = "digest2"

	// Error messages
	errMsgParseCatalog = "failed to parse catalog"
)

// createM2DTestOpts creates a CopyOptions configured for MirrorToDisk mode
// for testing catalog pinning (resolves digests via network calls via mock)
func createM2DTestOpts(t *testing.T) *mirror.CopyOptions {
	global := &mirror.GlobalOptions{
		WorkingDir: t.TempDir(),
	}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")

	return &mirror.CopyOptions{
		Mode:                mirror.MirrorToDisk,
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
	}
}

func TestPinCatalogDigests_CopyBehavior(t *testing.T) {
	logger := clog.New("trace")
	ctx := context.Background()
	opts := createM2DTestOpts(t)

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

	manifest := &MockManifest{
		digestMap: map[string]string{
			redhatIndexTagDocker: testDigestShort1,
		},
	}

	pinnedCfg := PinCatalogDigests(ctx, cfg, manifest, opts, logger)
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
	ctx := context.Background()
	opts := createM2DTestOpts(t)

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{},
			},
		},
	}

	manifest := &MockManifest{}

	pinnedCfg := PinCatalogDigests(ctx, cfg, manifest, opts, logger)
	assert.Empty(t, pinnedCfg.Mirror.Operators)
}

func TestPinCatalogDigests_MultipleOperators(t *testing.T) {
	logger := clog.New("trace")
	ctx := context.Background()
	opts := createM2DTestOpts(t)

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

	manifest := &MockManifest{
		digestMap: map[string]string{
			redhatIndexTagDocker:    testDigestShort2,
			certifiedIndexTagDocker: testDigestShort3,
		},
	}

	pinnedCfg := PinCatalogDigests(ctx, cfg, manifest, opts, logger)

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

func TestPinSingleCatalog_Success(t *testing.T) {
	logger := clog.New("trace")
	ctx := context.Background()
	opts := createM2DTestOpts(t)

	op := &v2alpha1.Operator{
		Catalog: redhatIndexTag,
	}

	manifest := &MockManifest{
		digestMap: map[string]string{
			redhatIndexTagDocker: testDigestShort1,
		},
	}

	err := pinSingleCatalogDigest(ctx, op, manifest, opts, logger)
	require.NoError(t, err)
	assert.Equal(t, image.WithDigest(redhatIndexBase, testDigestShort1), op.Catalog)
}

func TestPinSingleCatalog_AlreadyPinned(t *testing.T) {
	logger := clog.New("trace")
	ctx := context.Background()
	opts := createM2DTestOpts(t)

	alreadyPinnedCatalog := image.WithDigest(redhatIndexBase, testDigest1)

	op := &v2alpha1.Operator{
		Catalog: alreadyPinnedCatalog,
	}

	manifest := &MockManifest{
		digestMap: map[string]string{
			consts.DockerProtocol + alreadyPinnedCatalog: "newdigest123",
		},
	}

	err := pinSingleCatalogDigest(ctx, op, manifest, opts, logger)
	require.NoError(t, err)
	assert.Equal(t, alreadyPinnedCatalog, op.Catalog, "Should not modify already pinned catalog")
}

func TestPinSingleCatalog_NotInMap(t *testing.T) {
	logger := clog.New("trace")
	ctx := context.Background()
	opts := createM2DTestOpts(t)

	op := &v2alpha1.Operator{
		Catalog: redhatIndexTag,
	}

	// Empty digest map - will use default digest
	manifest := &MockManifest{
		digestMap: map[string]string{},
	}

	err := pinSingleCatalogDigest(ctx, op, manifest, opts, logger)
	require.NoError(t, err)
	assert.Equal(t, redhatIndexTag, op.Catalog, "Should not modify catalog when not found in map")
}

func TestPinSingleCatalog_EmptyDigest(t *testing.T) {
	logger := clog.New("trace")
	ctx := context.Background()
	opts := createM2DTestOpts(t)

	op := &v2alpha1.Operator{
		Catalog: redhatIndexTag,
	}

	manifest := &MockManifest{
		digestMap: map[string]string{
			redhatIndexTagDocker: "", // Empty digest
		},
	}

	err := pinSingleCatalogDigest(ctx, op, manifest, opts, logger)
	require.NoError(t, err)
	assert.Equal(t, redhatIndexTag, op.Catalog, "Should not modify catalog for empty digest")
}

func TestPinSingleCatalog_InvalidCatalog(t *testing.T) {
	logger := clog.New("trace")
	ctx := context.Background()
	opts := createM2DTestOpts(t)

	op := &v2alpha1.Operator{
		Catalog: "", // Empty catalog reference should fail parsing
	}

	manifest := &MockManifest{
		digestMap: map[string]string{},
	}

	err := pinSingleCatalogDigest(ctx, op, manifest, opts, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), errMsgParseCatalog)
}

func TestPinSingleCatalog_OCITransport(t *testing.T) {
	logger := clog.New("trace")
	ctx := context.Background()
	opts := createM2DTestOpts(t)

	ociCatalog := "oci:///path/to/catalog"

	op := &v2alpha1.Operator{
		Catalog: ociCatalog,
	}

	manifest := &MockManifest{
		digestMap: map[string]string{
			ociCatalog: "ocidigests123",
		},
	}

	err := pinSingleCatalogDigest(ctx, op, manifest, opts, logger)
	require.NoError(t, err)
	assert.Equal(t, ociCatalog, op.Catalog, "Should skip OCI catalog (already local)")
}

func TestPinSingleCatalog_DockerTransport(t *testing.T) {
	logger := clog.New("trace")
	ctx := context.Background()
	opts := createM2DTestOpts(t)

	dockerCatalog := consts.DockerProtocol + "registry.redhat.io/redhat/redhat-operator-index:v4.12"

	op := &v2alpha1.Operator{
		Catalog: dockerCatalog,
	}

	manifest := &MockManifest{
		digestMap: map[string]string{
			dockerCatalog: testDigestShort1,
		},
	}

	err := pinSingleCatalogDigest(ctx, op, manifest, opts, logger)
	require.NoError(t, err)
	assert.Equal(t, image.WithDigest(redhatIndexBase, testDigestShort1), op.Catalog,
		"Should omit docker:// transport prefix")
}

func TestPinSingleCatalog_NoTransport(t *testing.T) {
	logger := clog.New("trace")
	ctx := context.Background()
	opts := createM2DTestOpts(t)

	op := &v2alpha1.Operator{
		Catalog: redhatIndexTag,
	}

	// Catalog without explicit transport (docker:// is implied)
	manifest := &MockManifest{
		digestMap: map[string]string{
			redhatIndexTagDocker: testDigestShort1,
		},
	}

	err := pinSingleCatalogDigest(ctx, op, manifest, opts, logger)
	require.NoError(t, err)
	assert.Equal(t, image.WithDigest(redhatIndexBase, testDigestShort1), op.Catalog,
		"Should not add transport prefix when none specified")
	assert.NotContains(t, op.Catalog, consts.DockerProtocol,
		"Should not include docker:// prefix in pinned reference")
}

func TestPinSingleCatalog_MultipleCatalogs(t *testing.T) {
	logger := clog.New("trace")
	ctx := context.Background()
	opts := createM2DTestOpts(t)

	manifest := &MockManifest{
		digestMap: map[string]string{
			redhatIndexTagDocker:    testDigestShort1,
			certifiedIndexTagDocker: testDigestShort2,
		},
	}

	// Pin first catalog
	op1 := &v2alpha1.Operator{
		Catalog: redhatIndexTag,
	}
	err := pinSingleCatalogDigest(ctx, op1, manifest, opts, logger)
	require.NoError(t, err)
	assert.Equal(t, image.WithDigest(redhatIndexBase, testDigestShort1), op1.Catalog)

	// Pin second catalog
	op2 := &v2alpha1.Operator{
		Catalog: certifiedIndexTag,
	}
	err = pinSingleCatalogDigest(ctx, op2, manifest, opts, logger)
	require.NoError(t, err)
	assert.Equal(t, image.WithDigest("registry.redhat.io/redhat/certified-operator-index", testDigestShort2), op2.Catalog)
}
