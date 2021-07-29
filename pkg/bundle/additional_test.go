package bundle

import (
	"os"
	"testing"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

func Test_GetAdditional(t *testing.T) {

	cfg := v1alpha1.ImageSetConfiguration{}
	cfg.Mirror = v1alpha1.Mirror{
		BlockedImages: []v1alpha1.BlockedImages{
			{Name: "alpine"},
		},
		AdditionalImages: []v1alpha1.AdditionalImages{
			{Name: "docker.io/library/alpine:latest"},
			{Name: "docker.io/library/busybox:latest"},
		},
	}

	tmpdir, err := os.MkdirTemp("", "test")

	if err != nil {
		t.Error(err)
	}

	defer os.RemoveAll(tmpdir)

	if err := GetAdditional(cfg, tmpdir); err != nil {
		t.Error(err)
	}
}
