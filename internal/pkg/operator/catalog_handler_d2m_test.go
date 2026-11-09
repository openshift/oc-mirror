package operator

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

const (
	// The registry hosting the catalog is never reachable in these tests: any
	// attempt to copy from it goes through a recordingMirror that fails.
	archivedCatalogRef  = "registry.example.invalid/ns/full-catalog@sha256:1111111111111111111111111111111111111111111111111111111111111111"
	archivedBundleRef   = "registry.example.invalid/ns/foo-bundle@sha256:2222222222222222222222222222222222222222222222222222222222222222"
	archivedOperatorRef = "registry.example.invalid/ns/foo-operator@sha256:3333333333333333333333333333333333333333333333333333333333333333"
	archivedCatalogFBC  = `{"schema":"olm.package","name":"foo","defaultChannel":"stable"}
{"schema":"olm.channel","name":"stable","package":"foo","entries":[{"name":"foo.v0.1.0"}]}
{"schema":"olm.bundle","name":"foo.v0.1.0","package":"foo","image":"` + archivedBundleRef + `","properties":[{"type":"olm.package","value":{"packageName":"foo","version":"0.1.0"}}],"relatedImages":[{"name":"","image":"` + archivedBundleRef + `"},{"name":"operator","image":"` + archivedOperatorRef + `"}]}
`
	errSourceRegistryContacted = "the source registry must not be contacted"
)

// recordingMirror records every copy request and returns runErr for all of them.
type recordingMirror struct {
	runErr error
	srcs   []string
	dests  []string
}

func (m *recordingMirror) Run(_ context.Context, src, dest string, _ mirror.Mode, _ *mirror.CopyOptions) error {
	m.srcs = append(m.srcs, src)
	m.dests = append(m.dests, dest)
	return m.runErr
}

func (m *recordingMirror) Check(_ context.Context, _ string, _ *mirror.CopyOptions, _ bool) (bool, error) {
	return m.runErr == nil, m.runErr
}

// newSourceRegistryTrap returns a mirror simulating a source registry that cannot
// be reached: every copy request is recorded and fails.
func newSourceRegistryTrap() *recordingMirror {
	return &recordingMirror{runErr: errors.New(errSourceRegistryContacted)}
}

// writeCatalogOCILayout writes, at dir, an OCI layout containing a catalog image
// whose configs layer holds the given file-based catalog, the same shape as the
// catalog-image directory that mirrorToDisk stores in the working-dir.
func writeCatalogOCILayout(t *testing.T, dir, fbc string) {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	content := []byte(fbc)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "configs/", Typeflag: tar.TypeDir, Mode: 0o755}))
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "configs/foo/", Typeflag: tar.TypeDir, Mode: 0o755}))
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "configs/foo/catalog.json", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content))}))
	_, err := tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	layerContent := buf.Bytes()
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(layerContent)), nil
	})
	require.NoError(t, err)
	img, err := mutate.AppendLayers(empty.Image, layer)
	require.NoError(t, err)
	img, err = mutate.Config(img, gcrv1.Config{Labels: map[string]string{operatorsConfigsV1Label: "/configs"}})
	require.NoError(t, err)

	p, err := layout.Write(dir, empty.Index)
	require.NoError(t, err)
	require.NoError(t, p.AppendImage(img))
}

// newD2MFullCatalogCollector returns a diskToMirror collector for a full catalog
// (full: true, no packages) that uses the real catalog handler and manifest
// implementations, plus the catalog-image directory where mirrorToDisk would have
// stored the catalog in the working-dir.
func newD2MFullCatalogCollector(t *testing.T, mirrorImpl mirror.MirrorInterface) (*FilterCollector, string) {
	t.Helper()
	log := clog.New("trace")
	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{{Catalog: archivedCatalogRef, Full: true}},
			},
		},
	}

	collector := setupFilterCollector_DiskToMirror(t.TempDir(), log).withConfig(cfg)
	collector.Opts.Function = string(mirror.CopyMode)
	mf := manifest.New(log)
	collector.Manifest = mf
	collector.ctlgHandler = CatalogHandler{Log: log, Manifest: mf, Mirror: mirrorImpl}

	imgSpec, err := image.ParseRef(archivedCatalogRef)
	require.NoError(t, err)
	catalogImageDir := filepath.Join(collector.Opts.Global.WorkingDir, operatorCatalogsDir, imgSpec.ComponentName(), imgSpec.Digest, operatorCatalogImageDir)

	return collector, catalogImageDir
}

// OCPBUGS-136389: a full catalog must be read from the mirror archive during
// diskToMirror, without contacting the registry the catalog originally came from.
func TestFilterCollectorD2MFullCatalogUsesArchivedCatalog(t *testing.T) {
	trap := newSourceRegistryTrap()
	collector, catalogImageDir := newD2MFullCatalogCollector(t, trap)
	writeCatalogOCILayout(t, catalogImageDir, archivedCatalogFBC)

	res, err := collector.OperatorImageCollector(context.Background())
	require.NoError(t, err)
	assert.Empty(t, trap.srcs, "diskToMirror must not copy the catalog from its source registry")

	expected := []v2alpha1.CopyImageSchema{
		{
			Source:      "docker://localhost:9999/ns/foo-bundle:sha256-2222222222222222222222222222222222222222222222222222222222222222",
			Destination: "docker://localhost:5000/test/ns/foo-bundle:sha256-2222222222222222222222222222222222222222222222222222222222222222",
			Origin:      consts.DockerProtocol + archivedBundleRef,
			Type:        v2alpha1.TypeOperatorBundle,
		},
		{
			Source:      "docker://localhost:9999/ns/foo-operator:sha256-3333333333333333333333333333333333333333333333333333333333333333",
			Destination: "docker://localhost:5000/test/ns/foo-operator:sha256-3333333333333333333333333333333333333333333333333333333333333333",
			Origin:      consts.DockerProtocol + archivedOperatorRef,
			Type:        v2alpha1.TypeOperatorRelatedImage,
		},
		{
			// full catalogs are mirrored as is: no rebuilt tag
			Source:      "docker://localhost:9999/ns/full-catalog:sha256-1111111111111111111111111111111111111111111111111111111111111111",
			Destination: "docker://localhost:5000/test/ns/full-catalog:sha256-1111111111111111111111111111111111111111111111111111111111111111",
			Origin:      consts.DockerProtocol + archivedCatalogRef,
			Type:        v2alpha1.TypeOperatorCatalog,
		},
	}
	assert.ElementsMatch(t, expected, res.AllImages)

	result, ok := res.CatalogToFBCMap[consts.DockerProtocol+archivedCatalogRef]
	require.True(t, ok, "the catalog should be part of the collected catalogs")
	assert.False(t, result.ToRebuild, "a full catalog must not be rebuilt")
	assert.Empty(t, result.FilteredConfigPath, "a full catalog has no filtered config")
	require.NotNil(t, result.DeclConfig)
	require.Len(t, result.DeclConfig.Bundles, 1)
	assert.Equal(t, "foo.v0.1.0", result.DeclConfig.Bundles[0].Name)
}

// The archived catalog copy is the only source of the catalog in diskToMirror:
// missing or invalid content must fail locally, without falling back to the
// source registry.
func TestFilterCollectorD2MFullCatalogInvalidArchive(t *testing.T) {
	testCases := []struct {
		caseName      string
		prepare       func(t *testing.T, catalogImageDir string)
		errorContains string
	}{
		{
			caseName:      "catalog missing from the working-dir",
			prepare:       func(t *testing.T, catalogImageDir string) {},
			errorContains: "catalog not found in the mirror archive",
		},
		{
			caseName: "invalid index.json",
			prepare: func(t *testing.T, catalogImageDir string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(catalogImageDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(catalogImageDir, "index.json"), []byte("{not json"), 0o600))
			},
			errorContains: "oci image index",
		},
		{
			caseName: "image manifest missing from the layout",
			prepare: func(t *testing.T, catalogImageDir string) {
				t.Helper()
				writeCatalogOCILayout(t, catalogImageDir, archivedCatalogFBC)
				p, err := layout.FromPath(catalogImageDir)
				require.NoError(t, err)
				idx, err := p.ImageIndex()
				require.NoError(t, err)
				idxManifest, err := idx.IndexManifest()
				require.NoError(t, err)
				require.Len(t, idxManifest.Manifests, 1)
				require.NoError(t, p.RemoveBlob(idxManifest.Manifests[0].Digest))
			},
			errorContains: "failed to get catalog oci image",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			trap := newSourceRegistryTrap()
			collector, catalogImageDir := newD2MFullCatalogCollector(t, trap)
			testCase.prepare(t, catalogImageDir)

			res, err := collector.OperatorImageCollector(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), archivedCatalogRef)
			assert.Contains(t, err.Error(), testCase.errorContains)
			assert.NotContains(t, err.Error(), errSourceRegistryContacted)
			assert.Empty(t, trap.srcs, "diskToMirror must not fall back to the source registry")
			assert.Empty(t, res.AllImages)
		})
	}
}

func TestEnsureCatalogInOCIFormatSourceByMode(t *testing.T) {
	const catalog = "registry.example.invalid/ns/catalog:v1.0"

	testCases := []struct {
		caseName        string
		mode            string
		function        string
		archived        bool
		expectMirrorRun bool
		expectError     string
	}{
		{
			caseName:        "mirrorToDisk copies the catalog from its source registry",
			mode:            mirror.MirrorToDisk,
			function:        string(mirror.CopyMode),
			expectMirrorRun: true,
		},
		{
			caseName:        "mirrorToMirror copies the catalog from its source registry",
			mode:            mirror.MirrorToMirror,
			function:        string(mirror.CopyMode),
			expectMirrorRun: true,
		},
		{
			caseName:        "mirrorToDisk refreshes an existing working-dir copy from the source registry",
			mode:            mirror.MirrorToDisk,
			function:        string(mirror.CopyMode),
			archived:        true,
			expectMirrorRun: true,
		},
		{
			caseName:        "delete keeps copying the catalog from its source registry",
			mode:            mirror.DiskToMirror,
			function:        string(mirror.DeleteMode),
			expectMirrorRun: true,
		},
		{
			caseName: "diskToMirror uses the catalog from the archive",
			mode:     mirror.DiskToMirror,
			function: string(mirror.CopyMode),
			archived: true,
		},
		{
			caseName:    "diskToMirror fails locally when the archived catalog is missing",
			mode:        mirror.DiskToMirror,
			function:    string(mirror.CopyMode),
			expectError: "catalog not found in the mirror archive",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			opts := setupFilterCollector_DiskToMirror(t.TempDir(), clog.New("trace")).Opts
			opts.Mode = testCase.mode
			opts.Function = testCase.function

			imageIndexDir := filepath.Join(opts.Global.WorkingDir, operatorCatalogsDir, "catalog", "abc123")
			catalogImageDir := filepath.Join(imageIndexDir, operatorCatalogImageDir)
			if testCase.archived {
				writeCatalogOCILayout(t, catalogImageDir, archivedCatalogFBC)
			}

			rec := &recordingMirror{}
			handler := CatalogHandler{Log: clog.New("trace"), Mirror: rec}

			imgSpec, err := image.ParseRef(catalog)
			require.NoError(t, err)

			err = handler.EnsureCatalogInOCIFormat(context.Background(), imgSpec, catalog, imageIndexDir, opts)
			if testCase.expectError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), testCase.expectError)
				assert.Contains(t, err.Error(), catalogImageDir)
				assert.NoDirExists(t, catalogImageDir, "no catalog-image directory should be created")
			} else {
				require.NoError(t, err)
			}

			if testCase.expectMirrorRun {
				assert.Equal(t, []string{consts.DockerProtocol + catalog}, rec.srcs)
				assert.Equal(t, []string{consts.OciProtocolTrimmed + catalogImageDir}, rec.dests)
			} else {
				assert.Empty(t, rec.srcs, "the source registry must not be contacted")
			}
		})
	}
}

// Catalogs referenced with oci:// are copied from the local layout in every mode,
// including diskToMirror.
func TestEnsureCatalogInOCIFormatOCICatalogInDiskToMirror(t *testing.T) {
	tempDir := t.TempDir()
	srcLayout := filepath.Join(tempDir, "src-catalog")
	writeCatalogOCILayout(t, srcLayout, archivedCatalogFBC)

	opts := setupFilterCollector_DiskToMirror(tempDir, clog.New("trace")).Opts
	opts.Function = string(mirror.CopyMode)
	imageIndexDir := filepath.Join(opts.Global.WorkingDir, operatorCatalogsDir, "src-catalog", "abc123")

	rec := &recordingMirror{}
	handler := CatalogHandler{Log: clog.New("trace"), Mirror: rec}

	catalog := consts.OciProtocol + srcLayout
	imgSpec, err := image.ParseRef(catalog)
	require.NoError(t, err)

	require.NoError(t, handler.EnsureCatalogInOCIFormat(context.Background(), imgSpec, catalog, imageIndexDir, opts))
	assert.Empty(t, rec.srcs)

	copied, err := os.ReadFile(filepath.Join(imageIndexDir, operatorCatalogImageDir, "index.json"))
	require.NoError(t, err)
	original, err := os.ReadFile(filepath.Join(srcLayout, "index.json"))
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(string(original)), strings.TrimSpace(string(copied)))
}
