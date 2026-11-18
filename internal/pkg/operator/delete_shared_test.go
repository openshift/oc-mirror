package operator

import (
	"context"
	"errors"
	"testing"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func TestPackagesWithoutVersionBounds(t *testing.T) {
	op := v2alpha1.Operator{
		Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.14",
		IncludeConfig: v2alpha1.IncludeConfig{
			Packages: []v2alpha1.IncludePackage{
				{
					Name: "quay-operator",
					IncludeBundle: v2alpha1.IncludeBundle{
						MinVersion: "3.11.5",
						MaxVersion: "3.11.5",
					},
					Channels: []v2alpha1.IncludeChannel{
						{
							Name: "stable-3.11",
							IncludeBundle: v2alpha1.IncludeBundle{
								MinVersion: "3.11.5",
								MaxVersion: "3.11.5",
							},
						},
					},
				},
			},
		},
	}
	out := packagesWithoutVersionBounds(op)
	require.Len(t, out.Packages, 1)
	assert.True(t, out.Full)
	assert.Equal(t, "quay-operator", out.Packages[0].Name)
	assert.Empty(t, out.Packages[0].MinVersion)
	assert.Empty(t, out.Packages[0].MaxVersion)
	assert.Empty(t, out.Packages[0].Channels)
	// original unchanged
	assert.Equal(t, "3.11.5", op.Packages[0].MinVersion)
	require.Len(t, op.Packages[0].Channels, 1)
}

func TestExcludeRelatedImagesSharedWithSiblingBundles(t *testing.T) {
	log := clog.New("error")
	shared := "registry.redhat.io/rhel8/redis-6@sha256:shared"
	onlyOld := "registry.redhat.io/quay/quay-rhel8@sha256:oldonly"
	bundleOld := "registry.redhat.io/quay/quay-operator-bundle@sha256:bundle3115"
	bundleNew := "registry.redhat.io/quay/quay-operator-bundle@sha256:bundle3116"

	deleteDC := &declcfg.DeclarativeConfig{
		Bundles: []declcfg.Bundle{
			{Name: "quay-operator.v3.11.5", Package: "quay-operator", Image: bundleOld},
		},
	}
	packageWideDC := &declcfg.DeclarativeConfig{
		Bundles: []declcfg.Bundle{
			{
				Name:    "quay-operator.v3.11.5",
				Package: "quay-operator",
				Image:   bundleOld,
				RelatedImages: []declcfg.RelatedImage{
					{Name: "bundle", Image: bundleOld},
					{Name: "redis", Image: shared},
					{Name: "quay", Image: onlyOld},
				},
			},
			{
				Name:    "quay-operator.v3.11.6",
				Package: "quay-operator",
				Image:   bundleNew,
				RelatedImages: []declcfg.RelatedImage{
					{Name: "bundle", Image: bundleNew},
					{Name: "redis", Image: shared},
				},
			},
		},
	}
	deleteRelated := map[string][]v2alpha1.RelatedImage{
		"quay-operator.v3.11.5": {
			{Name: "bundle", Image: bundleOld, Type: v2alpha1.TypeOperatorBundle},
			{Name: "redis", Image: shared, Type: v2alpha1.TypeOperatorRelatedImage},
			{Name: "quay", Image: onlyOld, Type: v2alpha1.TypeOperatorRelatedImage},
		},
	}

	got := excludeRelatedImagesSharedWithSiblingBundles(log, deleteRelated, deleteDC, packageWideDC, nil, nil)
	require.Contains(t, got, "quay-operator.v3.11.5")
	images := got["quay-operator.v3.11.5"]
	refs := relatedImageRefs(images)
	assert.Contains(t, refs, bundleOld)
	assert.Contains(t, refs, onlyOld)
	assert.NotContains(t, refs, shared)
}

func TestExcludeRelatedImagesSharedByDestinationIdentity(t *testing.T) {
	log := clog.New("error")
	digest := "sha256:1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58"
	fromRedhat := "registry.redhat.io/rhel8/redis-6@" + digest
	fromQuay := "quay.io/rhel8/redis-6@" + digest
	bundleOld := "registry.redhat.io/quay/quay-operator-bundle@sha256:bundle3115"
	bundleNew := "registry.redhat.io/quay/quay-operator-bundle@sha256:bundle3116"

	deleteDC := &declcfg.DeclarativeConfig{
		Bundles: []declcfg.Bundle{
			{Name: "quay-operator.v3.11.5", Package: "quay-operator", Image: bundleOld},
		},
	}
	packageWideDC := &declcfg.DeclarativeConfig{
		Bundles: []declcfg.Bundle{
			{
				Name:    "quay-operator.v3.11.5",
				Package: "quay-operator",
				Image:   bundleOld,
				RelatedImages: []declcfg.RelatedImage{
					{Name: "redis", Image: fromRedhat},
				},
			},
			{
				Name:    "quay-operator.v3.11.6",
				Package: "quay-operator",
				Image:   bundleNew,
				RelatedImages: []declcfg.RelatedImage{
					{Name: "redis", Image: fromQuay},
				},
			},
		},
	}
	deleteRelated := map[string][]v2alpha1.RelatedImage{
		"quay-operator.v3.11.5": {
			{Name: "bundle", Image: bundleOld, Type: v2alpha1.TypeOperatorBundle},
			{Name: "redis", Image: fromRedhat, Type: v2alpha1.TypeOperatorRelatedImage},
		},
	}

	got := excludeRelatedImagesSharedWithSiblingBundles(log, deleteRelated, deleteDC, packageWideDC, nil, nil)
	refs := relatedImageRefs(got["quay-operator.v3.11.5"])
	assert.Contains(t, refs, bundleOld)
	assert.NotContains(t, refs, fromRedhat)
}

func TestExcludeRelatedImagesUnionDeleteSelection(t *testing.T) {
	log := clog.New("error")
	shared := "registry.redhat.io/rhel8/redis-6@sha256:shared"
	bundleOld := "registry.redhat.io/quay/quay-operator-bundle@sha256:bundle3115"
	bundleNew := "registry.redhat.io/quay/quay-operator-bundle@sha256:bundle3116"

	deleteDC := &declcfg.DeclarativeConfig{
		Bundles: []declcfg.Bundle{
			{Name: "quay-operator.v3.11.5", Package: "quay-operator", Image: bundleOld},
		},
	}
	packageWideDC := &declcfg.DeclarativeConfig{
		Bundles: []declcfg.Bundle{
			{
				Name:    "quay-operator.v3.11.5",
				Package: "quay-operator",
				Image:   bundleOld,
				RelatedImages: []declcfg.RelatedImage{
					{Name: "redis", Image: shared},
				},
			},
			{
				Name:    "quay-operator.v3.11.6",
				Package: "quay-operator",
				Image:   bundleNew,
				RelatedImages: []declcfg.RelatedImage{
					{Name: "redis", Image: shared},
				},
			},
		},
	}
	deleteRelated := map[string][]v2alpha1.RelatedImage{
		"quay-operator.v3.11.5": {
			{Name: "bundle", Image: bundleOld, Type: v2alpha1.TypeOperatorBundle},
			{Name: "redis", Image: shared, Type: v2alpha1.TypeOperatorRelatedImage},
		},
	}
	union := map[string]struct{}{"quay-operator.v3.11.6": {}}

	got := excludeRelatedImagesSharedWithSiblingBundles(log, deleteRelated, deleteDC, packageWideDC, union, nil)
	refs := relatedImageRefs(got["quay-operator.v3.11.5"])
	assert.Contains(t, refs, bundleOld)
	assert.Contains(t, refs, shared)
}

func TestExcludeRelatedImagesSiblingMissingOnDestination(t *testing.T) {
	log := clog.New("error")
	shared := "registry.redhat.io/rhel8/redis-6@sha256:shared"
	bundleOld := "registry.redhat.io/quay/quay-operator-bundle@sha256:bundle3115"
	bundleNew := "registry.redhat.io/quay/quay-operator-bundle@sha256:bundle3116"

	deleteDC := &declcfg.DeclarativeConfig{
		Bundles: []declcfg.Bundle{
			{Name: "quay-operator.v3.11.5", Package: "quay-operator", Image: bundleOld},
		},
	}
	packageWideDC := &declcfg.DeclarativeConfig{
		Bundles: []declcfg.Bundle{
			{
				Name:    "quay-operator.v3.11.5",
				Package: "quay-operator",
				Image:   bundleOld,
				RelatedImages: []declcfg.RelatedImage{
					{Name: "redis", Image: shared},
				},
			},
			{
				Name:    "quay-operator.v3.11.6",
				Package: "quay-operator",
				Image:   bundleNew,
				RelatedImages: []declcfg.RelatedImage{
					{Name: "redis", Image: shared},
				},
			},
		},
	}
	deleteRelated := map[string][]v2alpha1.RelatedImage{
		"quay-operator.v3.11.5": {
			{Name: "bundle", Image: bundleOld, Type: v2alpha1.TypeOperatorBundle},
			{Name: "redis", Image: shared, Type: v2alpha1.TypeOperatorRelatedImage},
		},
	}
	siblingPresent := func(b declcfg.Bundle) bool {
		return b.Name != "quay-operator.v3.11.6"
	}

	got := excludeRelatedImagesSharedWithSiblingBundles(log, deleteRelated, deleteDC, packageWideDC, nil, siblingPresent)
	refs := relatedImageRefs(got["quay-operator.v3.11.5"])
	assert.Contains(t, refs, bundleOld)
	assert.Contains(t, refs, shared)
}

func TestHasVersionConstraints(t *testing.T) {
	tests := map[string]struct {
		op   v2alpha1.Operator
		want bool
	}{
		"package min version": {
			op: v2alpha1.Operator{IncludeConfig: v2alpha1.IncludeConfig{
				Packages: []v2alpha1.IncludePackage{{Name: "quay-operator", IncludeBundle: v2alpha1.IncludeBundle{MinVersion: "3.11.5"}}},
			}},
			want: true,
		},
		"channel max version": {
			op: v2alpha1.Operator{IncludeConfig: v2alpha1.IncludeConfig{
				Packages: []v2alpha1.IncludePackage{{
					Name: "quay-operator",
					Channels: []v2alpha1.IncludeChannel{{
						Name:          "stable-3.11",
						IncludeBundle: v2alpha1.IncludeBundle{MaxVersion: "3.11.5"},
					}},
				}},
			}},
			want: true,
		},
		"no bounds": {
			op: v2alpha1.Operator{IncludeConfig: v2alpha1.IncludeConfig{
				Packages: []v2alpha1.IncludePackage{{Name: "quay-operator"}},
			}},
			want: false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasVersionConstraints(tt.op))
		})
	}
}

// fbcCatalogHandler serves an in-memory FBC for delete shared-image filtering.
type fbcCatalogHandler struct {
	MockHandler
	dc *declcfg.DeclarativeConfig
}

func (h fbcCatalogHandler) GetDeclarativeConfig(_ context.Context, _ string) (*declcfg.DeclarativeConfig, error) {
	return h.dc, nil
}

func (h fbcCatalogHandler) ExtractOCIConfigLayers(_ image.ImageSpec, _ string) (string, error) {
	return "in-memory-fbc", nil
}

func relatedImageRefs(images []v2alpha1.RelatedImage) []string {
	refs := make([]string, 0, len(images))
	for _, img := range images {
		refs = append(refs, img.Image)
	}
	return refs
}

func declarativeBundleNames(dc *declcfg.DeclarativeConfig) []string {
	names := make([]string, 0, len(dc.Bundles))
	for _, b := range dc.Bundles {
		names = append(names, b.Name)
	}
	return names
}

func countSiblings(packageNames, deleteNames []string) int {
	deleteSet := make(map[string]struct{}, len(deleteNames))
	for _, name := range deleteNames {
		deleteSet[name] = struct{}{}
	}
	n := 0
	for _, name := range packageNames {
		if _, deleting := deleteSet[name]; !deleting {
			n++
		}
	}
	return n
}

func keptImageSet(got map[string][]v2alpha1.RelatedImage) map[string]struct{} {
	kept := make(map[string]struct{})
	for _, images := range got {
		for _, img := range images {
			kept[img.Image] = struct{}{}
		}
	}
	return kept
}

func assertSharedRelatedDropped(t *testing.T, got, deleteRelated map[string][]v2alpha1.RelatedImage, deleteDC, packageDC *declcfg.DeclarativeConfig) {
	t.Helper()
	_, deleteBundleImages := deleteBundleIdentity(deleteDC)
	siblingImages := siblingKeepImages(packageDC, deleteBundleIdentityNames(deleteDC), nil)
	require.NotEmpty(t, siblingImages)

	kept := keptImageSet(got)
	droppedShared := 0
	for bundleName, images := range deleteRelated {
		require.Contains(t, got, bundleName, "deleted bundle should remain in the delete set")
		for _, img := range images {
			if mustKeepInDeleteSet(img, deleteBundleImages) {
				assert.Contains(t, kept, img.Image, "bundle image for the deleted version must stay in the delete set")
				continue
			}
			if _, shared := siblingImages[destIdentity(img.Image)]; shared {
				droppedShared++
				assert.NotContains(t, kept, img.Image, "related image still referenced by a sibling version must not be deleted: %s", img.Image)
			}
		}
	}
	require.Greater(t, droppedShared, 0, "expected at least one related image to be shared with a sibling version")
}

func deleteBundleIdentityNames(dc *declcfg.DeclarativeConfig) map[string]struct{} {
	names, _ := deleteBundleIdentity(dc)
	return names
}

func TestExcludeSharedRelatedForDelete_Integration(t *testing.T) {
	log := clog.New("error")
	handler := &CatalogHandler{Log: log}
	loadFBC := func() *declcfg.DeclarativeConfig {
		t.Helper()
		dc, err := handler.GetDeclarativeConfig(t.Context(), consts.TestFolder+"configs")
		require.NoError(t, err)
		return dc
	}

	deleteFilter := v2alpha1.Operator{
		Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.14",
		IncludeConfig: v2alpha1.IncludeConfig{
			Packages: []v2alpha1.IncludePackage{{
				Name:           "3scale-operator",
				DefaultChannel: "threescale-2.11",
				Channels: []v2alpha1.IncludeChannel{{
					Name: "threescale-2.11",
					IncludeBundle: v2alpha1.IncludeBundle{
						MinVersion: "0.8.3",
						MaxVersion: "0.8.3",
					},
				}},
			}},
		},
	}
	deleteDC, err := filterCatalog(t.Context(), *loadFBC(), deleteFilter)
	require.NoError(t, err)
	require.NotEmpty(t, deleteDC.Bundles)

	copyMap := &v2alpha1.CopyImageSchemaMap{
		OperatorsByImage: make(map[string]map[string]struct{}),
		BundlesByImage:   make(map[string]map[string]string),
	}
	deleteRelated, err := handler.getRelatedImagesFromCatalog(deleteDC, copyMap)
	require.NoError(t, err)
	require.NotEmpty(t, deleteRelated)

	imgSpec, err := image.ParseRef(deleteFilter.Catalog)
	require.NoError(t, err)

	collector := &FilterCollector{
		OperatorCollector: OperatorCollector{
			Log:         log,
			ctlgHandler: fbcCatalogHandler{dc: loadFBC()},
			Opts: mirror.CopyOptions{
				Function: string(mirror.DeleteMode),
				Global:   &mirror.GlobalOptions{WorkingDir: t.TempDir()},
			},
		},
	}
	got, err := collector.excludeSharedRelatedForDelete(
		t.Context(), deleteFilter, imgSpec, "testdigest", deleteDC, deleteRelated, nil,
	)
	require.NoError(t, err)

	packageDC, err := filterCatalog(t.Context(), *loadFBC(), packagesWithoutVersionBounds(deleteFilter))
	require.NoError(t, err)
	deleteNames := declarativeBundleNames(deleteDC)
	packageNames := declarativeBundleNames(packageDC)
	t.Logf("delete bundles (%d): %v", len(deleteNames), deleteNames)
	t.Logf("package-wide bundles (%d): %v", len(packageNames), packageNames)
	require.Greater(t, countSiblings(packageNames, deleteNames), 0, "package-wide catalog should include sibling versions")
	assertSharedRelatedDropped(t, got, deleteRelated, deleteDC, packageDC)
}

type failingExtractHandler struct {
	fbcCatalogHandler
}

func (h failingExtractHandler) ExtractOCIConfigLayers(_ image.ImageSpec, _ string) (string, error) {
	return "", errors.New("extract failed")
}

func TestExcludeSharedRelatedForDelete_ExtractError(t *testing.T) {
	log := clog.New("error")
	op := v2alpha1.Operator{Catalog: "registry.redhat.io/redhat/redhat-operator-index:v4.14"}
	imgSpec, err := image.ParseRef(op.Catalog)
	require.NoError(t, err)
	collector := &FilterCollector{
		OperatorCollector: OperatorCollector{
			Log:         log,
			ctlgHandler: failingExtractHandler{},
			Opts: mirror.CopyOptions{
				Function: string(mirror.DeleteMode),
				Global:   &mirror.GlobalOptions{WorkingDir: t.TempDir()},
			},
		},
	}
	got, err := collector.excludeSharedRelatedForDelete(
		t.Context(), op, imgSpec, "testdigest", &declcfg.DeclarativeConfig{}, map[string][]v2alpha1.RelatedImage{"x": {}}, nil,
	)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "extract failed")
}
