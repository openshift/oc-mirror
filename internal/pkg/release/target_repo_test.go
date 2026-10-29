package release

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func TestReleaseTargetRepoMappings(t *testing.T) {
	digest := strings.Repeat("a", 64)
	const releaseTag = "4.20.8-x86_64"
	cases := []struct {
		name, defaultRepo, tag string
		image                  v2alpha1.RelatedImage
	}{
		{
			name: "payload", defaultRepo: "openshift/release-images", tag: releaseTag,
			image: v2alpha1.RelatedImage{Image: "quay.io/release/payload:" + releaseTag, Type: v2alpha1.TypeOCPRelease},
		},
		{
			name: "component", defaultRepo: "openshift/release", tag: releaseTag + "-api-server",
			image: v2alpha1.RelatedImage{Image: "quay.io/release/components@sha256:" + digest, Name: "api-server", Type: v2alpha1.TypeOCPReleaseContent},
		},
		{
			name: "kubevirt", defaultRepo: "openshift/release", tag: releaseTag + "-kube-virt-container",
			image: v2alpha1.RelatedImage{Image: "quay.io/release/components@sha256:" + digest, Name: "kube-virt-container", Type: v2alpha1.TypeOCPReleaseContent},
		},
		{
			name: "unnamed component", defaultRepo: "release/unnamed", tag: "sha256-" + digest,
			image: v2alpha1.RelatedImage{Image: "quay.io/release/unnamed@sha256:" + digest, Type: v2alpha1.TypeOCPReleaseContent},
		},
		{
			name: "graph", defaultRepo: "openshift/graph-image", tag: "latest",
			image: v2alpha1.RelatedImage{Image: "quay.io/openshift/graph-image:latest", Type: v2alpha1.TypeCincinnatiGraph},
		},
	}
	images := make([]v2alpha1.RelatedImage, len(cases))
	for i, tc := range cases {
		images[i] = tc.image
	}
	for _, repo := range []string{"", "openshift4/ocp-release"} {
		t.Run("repo="+repo, func(t *testing.T) {
			o := LocalStorageCollector{Log: clog.New("error"), LocalStorageFQDN: "localhost:55000"}
			o.Config.Mirror.Platform.TargetRepo = repo
			o.Opts = mirror.CopyOptions{Mode: mirror.MirrorToDisk, Destination: "file:///tmp/archive"}
			cached, err := o.prepareM2DCopyBatch(images, releaseTag)
			require.NoError(t, err)
			o.Opts.Mode = mirror.DiskToMirror
			o.Opts.Destination = "docker://mirror.example.com/team"
			restored, err := o.prepareD2MCopyBatch(images, releaseTag)
			require.NoError(t, err)
			o.Opts.Mode = mirror.MirrorToMirror
			direct, err := o.prepareM2DCopyBatch(images, releaseTag)
			require.NoError(t, err)
			require.Len(t, cached, len(cases))
			require.Len(t, restored, len(cases))
			require.Len(t, direct, len(cases))
			for i, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					path := tc.defaultRepo
					if repo != "" && tc.image.Type != v2alpha1.TypeCincinnatiGraph {
						path = repo
					}
					require.Equal(t, "docker://localhost:55000/"+path+":"+tc.tag, cached[i].Destination)
					require.Equal(t, cached[i].Destination, restored[i].Source, "diskToMirror must pull the cached reference")
					require.Equal(t, "docker://mirror.example.com/team/"+path+":"+tc.tag, restored[i].Destination)
					require.Equal(t, restored[i].Destination, direct[i].Destination)
					require.Equal(t, "docker://"+tc.image.Image, direct[i].Source)
					require.Equal(t, tc.image.Image, restored[i].Origin)
				})
			}
			o.Releases = []string{images[0].Image}
			releaseRef, err := o.ReleaseImage(context.Background())
			require.NoError(t, err)
			require.Equal(t, direct[0].Destination, releaseRef, "UpdateService must use the overridden repository")
		})
	}
}

func TestReleaseTargetRepoDigestPayload(t *testing.T) {
	digest := strings.Repeat("b", 64)
	o := LocalStorageCollector{Log: clog.New("error"), LocalStorageFQDN: "localhost:55000"}
	o.Config.Mirror.Platform.TargetRepo = "openshift4/ocp-release"
	o.Opts.Mode = mirror.DiskToMirror
	o.Opts.Destination = "docker://mirror.example.com"
	batches, err := o.prepareMonoReleaseBatch(v2alpha1.RelatedImage{Image: "quay.io/release/payload@sha256:" + digest, Type: v2alpha1.TypeOCPRelease})
	require.NoError(t, err)
	require.Len(t, batches, 1)
	require.Equal(t, "docker://mirror.example.com/openshift4/ocp-release:"+digest, batches[0].Destination)
}
