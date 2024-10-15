package imagebuilder

import (
	"context"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type ImageBuilderInterface interface {
	BuildAndPush(ctx context.Context, targetRef string, layoutPath layout.Path, cmd []string, layers ...v1.Layer) error
	SaveImageLayoutToDir(ctx context.Context, imgRef string, layoutDir string) (layout.Path, error)
	ProcessImageIndex(ctx context.Context, idx v1.ImageIndex, v2format *bool, cmd []string, targetRef string, layers ...v1.Layer) (v1.ImageIndex, error)
	RebuildCatalogs(ctx context.Context, collectorSchema v2alpha1.CollectorSchema) ([]v2alpha1.CopyImageSchema, []v2alpha1.Image, error)
}
