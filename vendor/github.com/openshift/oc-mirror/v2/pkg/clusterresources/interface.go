package clusterresources

import (
	"context"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

type GeneratorInterface interface {
	IDMSGenerator(ctx context.Context, allRelatedImages []v1alpha3.CopyImageSchema, opts mirror.CopyOptions) error
}
