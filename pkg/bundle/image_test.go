package bundle

import (
	"testing"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/openshift/library-go/pkg/image/reference"
)

func Test_ImageBlocking(t *testing.T) {
	type fields struct {
		blockedImages []v1alpha1.BlockedImages
	}
	tests := []struct {
		name   string
		fields fields
		ref    reference.DockerImageReference
		want   bool
	}{
		{
			name: "testing want to block",
			fields: fields{
				blockedImages: []v1alpha1.BlockedImages{
					{Name: "alpine"},
				},
			},
			ref: reference.DockerImageReference{
				Name: "alpine",
			},
			want: true,
		},
		{
			name: "testing do not want to block",
			fields: fields{
				blockedImages: []v1alpha1.BlockedImages{
					{Name: "alpine"},
				},
			},
			ref: reference.DockerImageReference{
				Name: "ubi8",
			},
			want: false,
		},
		{
			name: "testing do not want to block, contains keyword",
			fields: fields{
				blockedImages: []v1alpha1.BlockedImages{
					{Name: "alpine"},
				},
			},
			ref: reference.DockerImageReference{
				Name: "notalpine",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		cfg := v1alpha1.ImageSetConfiguration{}
		cfg.Mirror = v1alpha1.Mirror{
			BlockedImages: tt.fields.blockedImages,
		}

		actual := IsBlocked(cfg, tt.ref)

		if actual != tt.want {
			t.Errorf("Test %s: Expected '%v', got '%v'", tt.name, tt.want, actual)
		}

	}
}
