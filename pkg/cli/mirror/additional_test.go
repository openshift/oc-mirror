package mirror

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"

	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc-mirror/pkg/image"
)

// TODO(jpower432): replace images being used under estroz org
func TestPlan_Additional(t *testing.T) {
	tmpdir := t.TempDir()

	tests := []struct {
		name      string
		cfg       v1alpha1.ImageSetConfiguration
		want      error
		wantImage image.TypedImage
		wantErr   bool
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
			wantImage: image.TypedImage{
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Name:      "pull-tester-additional",
						ID:        "sha256:5e642429d9e8d03267879160121f3001a300cc31cd93455bc27edea309ea9a88",
						Tag:       "latest",
						Namespace: "estroz",
						Registry:  "quay.io",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: image.TypeGeneric,
			},
		},
		{
			name: "Valid/NoTag",
			cfg: v1alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha1.ImageSetConfigurationSpec{
					Mirror: v1alpha1.Mirror{
						BlockedImages: []v1alpha1.BlockedImages{
							{Image: v1alpha1.Image{Name: "pull-tester-blocked:test"}},
						},
						AdditionalImages: []v1alpha1.AdditionalImages{
							{Image: v1alpha1.Image{Name: "quay.io/estroz/pull-tester-additional"}},
						},
					},
				},
			},
			wantImage: image.TypedImage{
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "quay.io",
						Name:      "pull-tester-additional",
						ID:        "sha256:5e642429d9e8d03267879160121f3001a300cc31cd93455bc27edea309ea9a88",
						Tag:       "latest",
						Namespace: "estroz",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: image.TypeGeneric,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mo := MirrorOptions{
				RootOptions: &cli.RootOptions{
					Dir: tmpdir,
					IOStreams: genericclioptions.IOStreams{
						In:     os.Stdin,
						Out:    os.Stdout,
						ErrOut: os.Stderr,
					},
				},
			}
			opts := NewAdditionalOptions(&mo)

			mappings, err := opts.Plan(context.TODO(), test.cfg.Mirror.AdditionalImages)
			if test.wantErr {
				testErr := test.want
				require.ErrorAs(t, err, &testErr)
			} else {
				require.NoError(t, err)
			}

			if assert.Len(t, mappings, 1) {
				require.Contains(t, mappings, test.wantImage)
			}
		})
	}
}
