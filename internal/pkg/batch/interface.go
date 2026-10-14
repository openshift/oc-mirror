package batch

import (
	"context"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type BatchInterface interface {
	Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.CollectorSchema, error)
}

// OperandResolver reports whether an image listed by an operator bundle is an
// operand -- workload the operator deploys -- rather than an image the operator
// runs itself. Only a proven operand may be skipped when no instance matches
// the requested platforms.
//
// The worker consults it only after a copy has already failed on a platform
// mismatch, so answering is allowed to be expensive. bundles maps a bundle
// image reference to its name, as CopyImageSchemaMap.BundlesByImage records it.
//
// Declared here rather than in the implementing package so the worker stays
// independent of operator-catalog concepts; the operator package provides an
// implementation.
type OperandResolver interface {
	IsOperand(ctx context.Context, imageOrigin string, bundles map[string]string) (bool, error)
}
