package mirror

import (
	"context"
	"os"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/match"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/operator-framework/operator-registry/pkg/containertools"
	"github.com/sirupsen/logrus"
)

type catalogBuilder struct {
	nameOpts   []name.Option
	remoteOpts []remote.Option
}

func (b *catalogBuilder) Run(ctx context.Context, targetRef string, layoutPath layout.Path, layers ...v1.Layer) error {

	var v2format bool
	tag, err := name.NewTag(targetRef, b.nameOpts...)
	if err != nil {
		return err
	}

	idx, err := layoutPath.ImageIndex()
	if err != nil {
		return err
	}
	idxManifest, err := idx.IndexManifest()
	if err != nil {
		return err
	}

	for _, manifest := range idxManifest.Manifests {
		if manifest.MediaType == types.DockerManifestSchema2 {
			v2format = true
		}

		img, err := layoutPath.Image(manifest.Digest)
		if err != nil {
			return err
		}
		// Add new layers to image
		img, err = mutate.AppendLayers(img, layers...)
		if err != nil {
			return err
		}

		// Update image config
		cfg, err := img.ConfigFile()
		if err != nil {
			return err
		}
		labels := map[string]string{
			containertools.ConfigsLocationLabel: "/configs",
		}
		cfg.Config.Labels = labels
		cfg.Config.Cmd = []string{"serve", "/configs"}
		cfg.Config.Entrypoint = []string{"/bin/opm"}
		img, err = mutate.Config(img, cfg.Config)
		if err != nil {
			return err
		}

		layoutOpts := []layout.Option{}
		if manifest.Platform != nil {
			layoutOpts = append(layoutOpts, layout.WithPlatform(*manifest.Platform))
		}
		if err := layoutPath.ReplaceImage(img, match.Digests(manifest.Digest), layoutOpts...); err != nil {
			return err
		}
	}

	// Pull updated index
	idx, err = layoutPath.ImageIndex()
	if err != nil {
		return err
	}

	// Ensure the index media type is a docker manifest list
	// if child manifests are docker V2 schema
	if v2format {
		idx = mutate.IndexMediaType(idx, types.DockerManifestList)
	}
	return remote.WriteIndex(tag, idx, b.remoteOpts...)
}

func (s *catalogBuilder) CreateLayout(srcRef, dir string) (layout.Path, error) {
	if srcRef == "" {
		logrus.Debugf("Using existing OCI layout to %s", dir)
		return layout.FromPath(dir)
	}

	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return "", err
	}
	// Pull source reference image
	ref, err := name.ParseReference(srcRef, s.nameOpts...)
	if err != nil {
		return "", err
	}
	idx, err := remote.Index(ref, s.remoteOpts...)
	if err != nil {
		return "", err
	}
	logrus.Debugf("Writing OCI layout to %s", dir)
	return layout.Write(dir, idx)
}
