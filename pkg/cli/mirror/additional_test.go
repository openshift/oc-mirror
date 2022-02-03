package mirror

import (
	"testing"

	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlan_Additional(t *testing.T) {
	opts := &AdditionalOptions{}

	tests := []struct {
		name    string
		cfg     v1alpha1.ImageSetConfiguration
		want    error
		wantErr bool
		imgPin  bool
	}{
		{
			name: "Valid/WithTag",
			cfg: v1alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha1.ImageSetConfigurationSpec{
					Mirror: v1alpha1.Mirror{
						BlockedImages: []v1alpha1.BlockedImages{
							{Image: v1alpha1.Image{Name: "pull-tester-blocked"}},
						},
						AdditionalImages: []v1alpha1.AdditionalImages{
							{Image: v1alpha1.Image{Name: "quay.io/estroz/pull-tester-additional:latest"}},
						},
					},
				},
			},
			imgPin: true,
		},
		{
			name: "Valid/NoTag",
			cfg: v1alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha1.ImageSetConfigurationSpec{
					Mirror: v1alpha1.Mirror{
						BlockedImages: []v1alpha1.BlockedImages{
							{Image: v1alpha1.Image{Name: "pull-tester-blocked"}},
						},
						AdditionalImages: []v1alpha1.AdditionalImages{
							{Image: v1alpha1.Image{Name: "quay.io/estroz/pull-tester-additional"}},
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			mappings, err := opts.Plan(test.cfg.Mirror.AdditionalImages)
			if test.wantErr {
				testErr := test.want
				require.ErrorAs(t, err, &testErr)
			} else {
				require.NoError(t, err)
			}

			testerRef, err := image.ParseTypedImage(test.cfg.Mirror.AdditionalImages[0].Name, image.TypeGeneric)
			require.NoError(t, err)
			if testerRef.Ref.Tag == "" {
				testerRef.Ref.Tag = "latest"
			}
			if assert.Len(t, mappings, 1) {
				require.Contains(t, mappings, testerRef)
			}
		})
	}
}
