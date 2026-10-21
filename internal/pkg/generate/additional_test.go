package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

func TestBuildAdditional(t *testing.T) {
	imgs, skipped := buildAdditional("reg.example.com", "ubi9/ubi", []string{
		"latest", "9.4",
		"0123456789ab",               // digest-derived tag
		"sha256-" + hex64() + ".sig", // signature
		"sha256-" + hex64(),          // digest-derived tag
	})
	assert.Equal(t, []v2alpha1.AdditionalImage{
		{Name: "reg.example.com/ubi9/ubi:9.4"},
		{Name: "reg.example.com/ubi9/ubi:latest"},
	}, imgs)
	assert.Equal(t, 2, skipped, "digest-derived tags counted, signatures not")
}

func hex64() string {
	s := ""
	for len(s) < 64 {
		s += "ab"
	}
	return s[:64]
}
