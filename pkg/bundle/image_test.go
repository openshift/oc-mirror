package bundle

import (
	"testing"

	"github.com/openshift/library-go/pkg/image/reference"
)

func Test_ImageBlocking(t *testing.T) {
	type fields struct {
		blockedImages []string
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
				blockedImages: []string{"alpine"},
			},
			ref: reference.DockerImageReference{
				Name: "alpine",
			},
			want: true,
		},
		{
			name: "testing do not want to block",
			fields: fields{
				blockedImages: []string{"alpine"},
			},
			ref: reference.DockerImageReference{
				Name: "ubi8",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		cfg := &BundleSpec{
			Mirror: Mirror{
				BlockedImages: tt.fields.blockedImages,
			},
		}

		actual := IsBlocked(cfg, tt.ref)

		if actual != tt.want {
			t.Errorf("Test %s: Expected '%v', got '%v'", tt.name, tt.want, actual)
		}

	}
}
