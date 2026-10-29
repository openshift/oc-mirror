package clusterresources

import (
	"strings"
	"testing"

	confv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

func TestReleaseCustomRepositoryMirrors(t *testing.T) {
	target := "mirror.example.com/team/openshift4/ocp-release"
	images := []v2alpha1.CopyImageSchema{
		{Origin: "quay.io/openshift-release-dev/ocp-release:4.20.8-x86_64", Destination: "docker://" + target + ":4.20.8-x86_64", Type: v2alpha1.TypeOCPRelease},
		{Origin: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:" + strings.Repeat("a", 64), Destination: "docker://" + target + ":4.20.8-x86_64-api-server", Type: v2alpha1.TypeOCPReleaseContent},
	}
	for _, tc := range []struct {
		name      string
		forceRepo bool
	}{
		{"default scope", false},
		{"forced repository scope", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cr := ClusterResourcesGenerator{}
			mirrors, err := cr.generateImageMirrors(images, DigestsOnlyMode, tc.forceRepo)
			require.NoError(t, err)
			idms, err := cr.generateIDMS(mirrors)
			require.NoError(t, err)
			require.Len(t, idms, 1)
			require.ElementsMatch(t, []confv1.ImageDigestMirrors{
				{Source: "quay.io/openshift-release-dev/ocp-release", Mirrors: []confv1.ImageMirror{confv1.ImageMirror(target)}},
				{Source: "quay.io/openshift-release-dev/ocp-v4.0-art-dev", Mirrors: []confv1.ImageMirror{confv1.ImageMirror(target)}},
			}, idms[0].Spec.ImageDigestMirrors)
			mirrors, err = cr.generateImageMirrors(images, TagsOnlyMode, tc.forceRepo)
			require.NoError(t, err)
			itms, err := cr.generateITMS(mirrors)
			require.NoError(t, err)
			require.Len(t, itms, 1)
			require.Equal(t, []confv1.ImageTagMirrors{
				{Source: "quay.io/openshift-release-dev/ocp-release", Mirrors: []confv1.ImageMirror{confv1.ImageMirror(target)}},
			}, itms[0].Spec.ImageTagMirrors)
		})
	}
}

func TestReleaseCustomRepositorySuffixMirrors(t *testing.T) {
	for _, sourcePath := range []string{"release", "team/release"} {
		t.Run(sourcePath, func(t *testing.T) {
			source := "source.example.com/" + sourcePath
			target := "mirror.example.com/enterprise-" + sourcePath
			images := []v2alpha1.CopyImageSchema{
				{Origin: source + ":v1", Destination: "docker://" + target + ":v1", Type: v2alpha1.TypeOCPRelease},
			}
			cr := ClusterResourcesGenerator{}
			mirrors, err := cr.generateImageMirrors(images, DigestsOnlyMode, false)
			require.NoError(t, err)
			idms, err := cr.generateIDMS(mirrors)
			require.NoError(t, err)
			require.Len(t, idms, 1)
			require.Equal(t, []confv1.ImageDigestMirrors{
				{Source: source, Mirrors: []confv1.ImageMirror{confv1.ImageMirror(target)}},
			}, idms[0].Spec.ImageDigestMirrors)
			mirrors, err = cr.generateImageMirrors(images, TagsOnlyMode, false)
			require.NoError(t, err)
			itms, err := cr.generateITMS(mirrors)
			require.NoError(t, err)
			require.Len(t, itms, 1)
			require.Equal(t, []confv1.ImageTagMirrors{
				{Source: source, Mirrors: []confv1.ImageMirror{confv1.ImageMirror(target)}},
			}, itms[0].Spec.ImageTagMirrors)
		})
	}
}
