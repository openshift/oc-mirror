package mirror

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/oc-mirror/pkg/bundle"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
)

// TODO: use some oc lib to mock image mirroring, or mirror from files.

func TestGetAdditional(t *testing.T) {
	tmpdir := t.TempDir()
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

	tests := []struct {
		name    string
		cfg     v1alpha1.ImageSetConfiguration
		want    error
		wantErr bool
		imgPin  bool
	}{
		{
			name: "testing with no block",
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
			name: "testing with no tag",
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
		{
			name: "testing with block",
			cfg: v1alpha1.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha1.ImageSetConfigurationSpec{
					Mirror: v1alpha1.Mirror{
						BlockedImages: []v1alpha1.BlockedImages{
							{Image: v1alpha1.Image{Name: "pull-tester-blocked"}},
						},
						AdditionalImages: []v1alpha1.AdditionalImages{
							{Image: v1alpha1.Image{Name: "quay.io/estroz/pull-tester-blocked"}},
						},
					},
				},
			},
			want:    ErrBlocked{},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			assocs, err := opts.GetAdditional(test.cfg, test.cfg.Mirror.AdditionalImages)
			if test.wantErr {
				testErr := test.want
				require.ErrorAs(t, err, &testErr)
			} else {
				require.NoError(t, err)
			}

			if test.imgPin {
				testerImg, err := bundle.PinImages(context.TODO(), test.cfg.Mirror.AdditionalImages[0].Name, "", false)
				require.NoError(t, err)
				if assert.Len(t, assocs, 1) {
					require.Contains(t, assocs, testerImg)
				}
			}
		})
	}
}
