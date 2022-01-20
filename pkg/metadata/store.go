package metadata

import (
	"context"
	"fmt"

	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

// UpdateMetadata runs some reconciliation functions on Metadata to ensure its state is consistent
// then uses the Backend to update the metadata storage medium.
func UpdateMetadata(ctx context.Context, backend storage.Backend, meta *v1alpha1.Metadata, skipTLSVerify, plainHTTP bool) error {

	var operatorErrs []error
	for mi, mirror := range meta.PastMirrors {
		for _, operator := range mirror.Mirror.Operators {
			operatorMeta, err := resolveOperatorMetadata(ctx, operator, backend, skipTLSVerify, plainHTTP)
			if err != nil {
				operatorErrs = append(operatorErrs, err)
				continue
			}

			meta.PastMirrors[mi].Operators = append(meta.PastMirrors[mi].Operators, operatorMeta)
		}
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

func resolveOperatorMetadata(ctx context.Context, operator v1alpha1.Operator, backend storage.Backend, skipTLSVerify, plainHTTP bool) (operatorMeta v1alpha1.OperatorMetadata, err error) {
	operatorMeta.Catalog = operator.Catalog

	resolver, err := containerdregistry.NewResolver("", skipTLSVerify, plainHTTP, nil)
	if err != nil {
		return v1alpha1.OperatorMetadata{}, fmt.Errorf("error creating image resolver: %v", err)
	}
	operatorMeta.ImagePin, err = image.ResolveToPin(ctx, resolver, operator.Catalog)
	if err != nil {
		return v1alpha1.OperatorMetadata{}, fmt.Errorf("error resolving catalog image %q: %v", operator.Catalog, err)
	}

	return operatorMeta, nil
}
