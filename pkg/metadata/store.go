package metadata

import (
	"context"
	"fmt"

	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

// SyncMetadata copies Metadata from one Backend to another
func SyncMetadata(ctx context.Context, first storage.Backend, second storage.Backend) error {
	var meta v1alpha2.Metadata
	if err := first.ReadMetadata(ctx, &meta, config.MetadataBasePath); err != nil {
		return fmt.Errorf("error reading metadata: %v", err)
	}
	// Add mirror as a new PastMirror
	if err := second.WriteMetadata(ctx, &meta, config.MetadataBasePath); err != nil {
		return fmt.Errorf("error writing metadata: %v", err)
	}
	return nil
}

// UpdateMetadata runs some reconciliation functions on Metadata to ensure its state is consistent
// then uses the Backend to update the metadata storage medium.
func UpdateMetadata(ctx context.Context, backend storage.Backend, meta *v1alpha2.Metadata, skipTLSVerify, plainHTTP bool) error {

	// If using heads only setting, aggregate all of the associations.
	// Replace if using ranges to prune any old image information
	// to support pruning.
	if keepPastAssociations(meta.PastMirror.Mirror) {
		meta.PastAssociations = append(meta.PastAssociations, meta.PastMirror.Associations...)
	} else {
		meta.PastAssociations = meta.PastMirror.Associations
	}

	var operatorErrs []error

	mirror := meta.PastMirror
	for _, operator := range mirror.Mirror.Operators {
		operatorMeta, err := resolveOperatorMetadata(ctx, operator, skipTLSVerify, plainHTTP)
		if err != nil {
			operatorErrs = append(operatorErrs, err)
			continue
		}

		meta.PastMirror.Operators = append(meta.PastMirror.Operators, operatorMeta)
	}
	if len(operatorErrs) != 0 {
		return utilerrors.NewAggregate(operatorErrs)
	}

	// Add mirror as a new PastMirror
	if err := backend.WriteMetadata(ctx, meta, config.MetadataBasePath); err != nil {
		return fmt.Errorf("error writing metadata: %v", err)
	}

	return nil
}

// TODO:(jpower432): Remove and use a
// configuration key to determine whether pruning
// is enabled
func keepPastAssociations(mirror v1alpha2.Mirror) bool {
	for _, release := range mirror.OCP.Channels {
		if release.IsHeadsOnly() {
			return true
		}
	}
	for _, ctlg := range mirror.Operators {
		// Keep for heads only and full catalog workflows
		if ctlg.IsHeadsOnly() || len(ctlg.Packages) == 0 {
			return true
		}
	}

	return false
}

func resolveOperatorMetadata(ctx context.Context, operator v1alpha2.Operator, skipTLSVerify, plainHTTP bool) (operatorMeta v1alpha2.OperatorMetadata, err error) {
	operatorMeta.Catalog = operator.Catalog

	resolver, err := containerdregistry.NewResolver("", skipTLSVerify, plainHTTP, nil)
	if err != nil {
		return v1alpha2.OperatorMetadata{}, fmt.Errorf("error creating image resolver: %v", err)
	}
	operatorMeta.ImagePin, err = image.ResolveToPin(ctx, resolver, operator.Catalog)
	if err != nil {
		return v1alpha2.OperatorMetadata{}, fmt.Errorf("error resolving catalog image %q: %v", operator.Catalog, err)
	}

	return operatorMeta, nil
}
