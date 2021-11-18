package mirror

import (
	"testing"

	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddOPMImage(t *testing.T) {

	var cfg v1alpha1.ImageSetConfiguration
	var meta v1alpha1.Metadata

	// No past OPMImage.
	cfg = v1alpha1.ImageSetConfiguration{}
	meta = v1alpha1.Metadata{}
	meta.MetadataSpec.PastMirrors = []v1alpha1.PastMirror{
		{
			Mirror: v1alpha1.Mirror{
				AdditionalImages: []v1alpha1.AdditionalImages{
					{Image: v1alpha1.Image{Name: "reg.com/ns/other:latest"}},
				},
			},
		},
	}

	addOPMImage(&cfg, meta)
	if assert.Len(t, cfg.Mirror.AdditionalImages, 1) {
		require.Equal(t, cfg.Mirror.AdditionalImages[0].Image.Name, OPMImage)
	}

	// Has past OPMImage.
	cfg = v1alpha1.ImageSetConfiguration{}
	meta = v1alpha1.Metadata{}
	meta.MetadataSpec.PastMirrors = []v1alpha1.PastMirror{
		{
			Mirror: v1alpha1.Mirror{
				AdditionalImages: []v1alpha1.AdditionalImages{
					{Image: v1alpha1.Image{Name: OPMImage}},
					{Image: v1alpha1.Image{Name: "reg.com/ns/other:latest"}},
				},
			},
		},
	}

	addOPMImage(&cfg, meta)
	require.Len(t, cfg.Mirror.AdditionalImages, 0)
}
