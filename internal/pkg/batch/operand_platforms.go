package batch

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// retryOperandCopy is the only exception to strict platform filtering. Bundles,
// controllers and unclassified images keep the original copy failure. Offline
// runs replay the archived decisions instead of reading source bundles.
func (o *ChannelConcurrentBatch) retryOperandCopy(ctx context.Context, img v2alpha1.CopyImageSchema, schema v2alpha1.CollectorSchema, opts mirror.CopyOptions, copyErr error) ([]v2alpha1.InstancePlatformFilter, bool, error) {
	bundles := schema.CopyImageSchemaMap.BundlesByImage[img.Origin]
	if img.Type != v2alpha1.TypeOperatorRelatedImage || o.OperandResolver == nil || len(bundles) == 0 || opts.IsDiskToMirror() || opts.IsDeleteMode() {
		return nil, false, copyErr
	}
	operand, err := o.OperandResolver.IsOperand(ctx, img.Origin, bundles)
	if err != nil {
		o.Log.Warn("Unable to identify operands for %s: %v", img.Origin, err)
		return nil, false, copyErr
	}
	if !operand {
		if operators := operatorNames(schema.CopyImageSchemaMap.OperatorsByImage[img.Origin]); operators != "" {
			copyErr = fmt.Errorf("operator %s does not support %s: %w", operators, strings.Join(opts.InstancePlatforms, ", "), copyErr)
		}
		return nil, false, copyErr
	}
	available, missing, err := o.copyAvailableOperandPlatforms(ctx, img, opts, copyErr)
	if err != nil {
		return nil, false, err
	}
	o.Log.Warn("Skipping platform(s) %s for operand %s: %s", strings.Join(missing, ", "), img.Origin, noMatchingPlatformReason)
	return available, len(available) == 0, nil
}

// copyAvailableOperandPlatforms handles a failed copy of a proven operand.
// The copy library identifies one missing platform before copying any instances.
// Remove only that requested platform and retry, at most once per platform.
// Never retry with an empty filter: that would select unrequested architectures.
// Other errors, including errors whose platform cannot be identified, stay fatal.
func (o *ChannelConcurrentBatch) copyAvailableOperandPlatforms(ctx context.Context, img v2alpha1.CopyImageSchema, opts mirror.CopyOptions, copyErr error) ([]v2alpha1.InstancePlatformFilter, []string, error) {
	var missing []string
	for copyErr != nil {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		_, after, found := strings.Cut(copyErr.Error(), "no instances found for platform ")
		fields := strings.Fields(after)
		if !found || len(fields) == 0 {
			return nil, nil, copyErr
		}

		remaining := slices.DeleteFunc(slices.Clone(opts.InstancePlatforms), func(platform string) bool {
			return platform == fields[0]
		})
		if len(remaining) == len(opts.InstancePlatforms) {
			return nil, nil, copyErr
		}
		missing = append(missing, fields[0])
		opts.InstancePlatforms = remaining
		if len(remaining) == 0 {
			break
		}
		retryOpts := opts
		copyErr = o.Mirror.Run(ctx, img.Source, img.Destination, mirror.Mode(opts.Function), &retryOpts)
	}

	var available []v2alpha1.InstancePlatformFilter
	for _, platform := range opts.InstancePlatforms {
		filter, err := v2alpha1.ParseInstancePlatformFilter(platform)
		if err != nil {
			return nil, nil, err
		}
		available = append(available, filter)
	}
	return available, missing, nil
}
