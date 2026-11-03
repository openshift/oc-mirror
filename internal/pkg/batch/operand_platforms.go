package batch

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// skipOperand handles the platform-mismatch exception without changing the
// image's collector-assigned type. Offline runs replay archived decisions.
func (o *ChannelConcurrentBatch) skipOperand(ctx context.Context, img v2alpha1.CopyImageSchema, schema v2alpha1.CollectorSchema, opts mirror.CopyOptions, copyErr error) (bool, error) {
	bundles := schema.CopyImageSchemaMap.BundlesByImage[img.Origin]
	if !img.Type.IsOperator() || o.OperandResolver == nil || len(bundles) == 0 || opts.IsDiskToMirror() || opts.IsDeleteMode() {
		return false, copyErr
	}
	operand, err := o.OperandResolver.IsOperand(ctx, img.Origin, bundles)
	if err != nil {
		o.Log.Warn("Unable to identify operands for %s: %v", img.Origin, err)
		return false, copyErr
	}
	requested := strings.Join(opts.InstancePlatforms, ", ")
	if !operand {
		if operators := operatorNames(schema.CopyImageSchemaMap.OperatorsByImage[img.Origin]); operators != "" {
			copyErr = fmt.Errorf("operator %s does not support %s: %w", operators, requested, copyErr)
		}
		return false, copyErr
	}
	o.Log.Warn("Skipping operand %s: %s %s", img.Origin, noMatchingPlatformReason, requested)
	return true, nil
}
