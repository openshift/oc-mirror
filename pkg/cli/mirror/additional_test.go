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

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/image"
)

func TestPlan_Additional(t *testing.T) {
	tmpdir := t.TempDir()

	tests := []struct {
		name      string
		cfg       v1alpha2.ImageSetConfiguration
		want      error
		wantImage image.TypedImage
		wantErr   bool
	}{
		{
			name: "Valid/WithTag",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						AdditionalImages: []v1alpha2.Image{
							{Name: "quay.io/redhatgov/oc-mirror-dev:latest"},
						},
					},
				},
			},
			wantImage: image.TypedImage{
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Name:      "oc-mirror-dev",
						ID:        "sha256:ee09cc8be7dd2b7a163e37f3e4dcdb7dbf474e15bbae557249cf648da0c7559f",
						Tag:       "latest",
						Namespace: "redhatgov",
						Registry:  "quay.io",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
			},
		},
		{
			name: "Valid/NoTag",
			cfg: v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						AdditionalImages: []v1alpha2.Image{
							{Name: "quay.io/redhatgov/oc-mirror-dev"},
						},
					},
				},
			},
			wantImage: image.TypedImage{
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "quay.io",
						Name:      "oc-mirror-dev",
						ID:        "sha256:ee09cc8be7dd2b7a163e37f3e4dcdb7dbf474e15bbae557249cf648da0c7559f",
						Tag:       "latest",
						Namespace: "redhatgov",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
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

			mappings, err := opts.Plan(context.TODO(), test.cfg.Mirror.AdditionalImages, MirrorToDiskScenario)
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
