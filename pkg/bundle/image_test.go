package bundle

import (
	"testing"

	"github.com/openshift/oc-mirror/pkg/config/v1alpha2"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
)

func TestImageBlocking(t *testing.T) {
	type fields struct {
		blockedImages []v1alpha2.BlockedImages
	}
	tests := []struct {
		name   string
		fields fields
		ref    string
		want   bool
	}{
		{
			name: "testing want to block",
			fields: fields{
				blockedImages: []v1alpha2.BlockedImages{
					{Image: v1alpha2.Image{Name: "alpine"}},
				},
			},
			ref:  "docker.io/library/alpine:latest",
			want: true,
		},
		{
			name: "testing do not want to block",
			fields: fields{
				blockedImages: []v1alpha2.BlockedImages{
					{Image: v1alpha2.Image{Name: "alpine"}},
				},
			},
			ref:  "registry.redhat.io/ubi8/ubi:latest",
			want: false,
		},
		{
			name: "testing do not want to block, contains keyword",
			fields: fields{
				blockedImages: []v1alpha2.BlockedImages{
					{Image: v1alpha2.Image{Name: "alpine"}},
				},
			},
			ref:  "docker.io/library/notalpine:latest",
			want: false,
		},
		{
			name: "testing with image not tag",
			fields: fields{
				blockedImages: []v1alpha2.BlockedImages{
					{Image: v1alpha2.Image{Name: "openshift-migration-velero-restic-restore-helper-rhel8"}},
				},
			},
			ref:  "registry.redhat.io/rhmtc/openshift-migration-velero-restic-restore-helper-rhel8",
			want: true,
		},
	}
	for _, tt := range tests {
		cfg := v1alpha2.ImageSetConfiguration{}
		cfg.Mirror = v1alpha2.Mirror{
			BlockedImages: tt.fields.blockedImages,
		}

		img, err := imagesource.ParseReference(tt.ref)

		if err != nil {
			t.Fatal(err)
		}

		actual := IsBlocked(cfg, img.Ref)

		if actual != tt.want {
			t.Errorf("Test %s: Expected '%v', got '%v'", tt.name, tt.want, actual)
		}

	}
}
