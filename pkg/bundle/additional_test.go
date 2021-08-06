package bundle

import (
	"testing"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

func Test_GetAdditional(t *testing.T) {

	mirror := v1alpha1.PastMirror{}
	cfg := v1alpha1.ImageSetConfiguration{}
	cfg.Mirror = v1alpha1.Mirror{
		BlockedImages: []v1alpha1.BlockedImages{
			{Image: v1alpha1.Image{Name: "alpine"}},
		},
		AdditionalImages: []v1alpha1.AdditionalImages{
			{Image: v1alpha1.Image{Name: "docker.io/library/busybox:latest"}},
		},
	}

	tmpdir := t.TempDir()

	// Use dry run to avoid hitting docker limits.
	dryRun := true
	if err := GetAdditional(mirror, cfg, tmpdir, dryRun, false); err != nil {
		t.Error(err)
	}
}
