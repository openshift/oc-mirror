package bundle

import (
	"os"
	"testing"
)

func Test_GetAdditional(t *testing.T) {

	cfg := &BundleSpec{
		Mirror: Mirror{
			BlockedImages:    []string{"alpine"},
			AdditionalImages: []string{"docker.io/library/alpine:latest", "docker.io/library/busybox:latest"},
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
