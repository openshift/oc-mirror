package imagebuilder

import (
	"context"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
)

type ImageBuilderInterface interface {
	BuildAndPush(ctx context.Context, targetRef string, layoutPath layout.Path, cmd []string, layers ...v1.Layer) error
	SaveImageLayoutToDir(ctx context.Context, imgRef string, layoutDir string) (layout.Path, error)
	ProcessImageIndex(ctx context.Context, idx v1.ImageIndex, v2format *bool, cmd []string, targetRef string, layers ...v1.Layer) (v1.ImageIndex, error)
}
