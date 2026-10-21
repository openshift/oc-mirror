package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

func TestBuildPlatform(t *testing.T) {
	tags := []string{
		"4.16.1-x86_64", "4.16.3-x86_64", "4.16.2-aarch64",
		"4.17.0-rc.1-x86_64", "4.17.0-x86_64",
		"4.16.3-x86_64.sig", "sha256-abcd.sig", "latest",
	}
	p, warns := buildPlatform(tags)

	assert.Equal(t, []string{"amd64", "arm64"}, p.Architectures) //nolint:staticcheck // deprecated field is what buildPlatform fills
	assert.Equal(t, []v2alpha1.ReleaseChannel{
		{Name: "stable-4.16", Type: v2alpha1.TypeOCP, MinVersion: "4.16.1", MaxVersion: "4.16.3"},
		{Name: "stable-4.17", Type: v2alpha1.TypeOCP, MinVersion: "4.17.0-rc.1", MaxVersion: "4.17.0"},
	}, p.Channels)
	assert.NotEmpty(t, warns, "derived channels must be reported as inferred")
}

func TestBuildPlatformNoReleaseTags(t *testing.T) {
	p, warns := buildPlatform([]string{"latest", "foo.sig"})
	assert.Empty(t, p.Channels)
	assert.Empty(t, warns)
}

func TestIsRepo(t *testing.T) {
	assert.True(t, isRepo("openshift/release", releaseContentRepo))
	assert.True(t, isRepo("mirror/openshift/release", releaseContentRepo))
	assert.False(t, isRepo("openshift/release-images", releaseContentRepo))
	assert.False(t, isRepo("xopenshift/release", releaseContentRepo))
}
