package operator

import (
	"testing"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
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
	assert.Equal(t, "quay-operator", out.Packages[0].Name)
	assert.Empty(t, out.Packages[0].MinVersion)
	assert.Empty(t, out.Packages[0].MaxVersion)
	require.Len(t, out.Packages[0].Channels, 1)
	assert.Empty(t, out.Packages[0].Channels[0].MinVersion)
	assert.Empty(t, out.Packages[0].Channels[0].MaxVersion)
	// original unchanged
	assert.Equal(t, "3.11.5", op.Packages[0].MinVersion)
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

	got := excludeRelatedImagesSharedWithSiblingBundles(log, deleteRelated, deleteDC, packageWideDC)
	require.Contains(t, got, "quay-operator.v3.11.5")
	images := got["quay-operator.v3.11.5"]
	var refs []string
	for _, img := range images {
		refs = append(refs, img.Image)
	}
	assert.Contains(t, refs, bundleOld)
	assert.Contains(t, refs, onlyOld)
	assert.NotContains(t, refs, shared)
}
