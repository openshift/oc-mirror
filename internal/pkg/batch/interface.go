package batch

import (
	"context"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type BatchInterface interface {
	Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.CollectorSchema, error)
}

// OperandResolver reports whether an image is an operand the operator deploys
// rather than one it runs itself. Only a proven operand may be skipped.
//
// Consulted only after a copy has already failed on a platform mismatch, so
// answering may be expensive. bundles is keyed as BundlesByImage records it.
type OperandResolver interface {
	IsOperand(ctx context.Context, imageOrigin string, bundles map[string]string) (bool, error)
}
