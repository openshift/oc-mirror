package mirror

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"

	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc-mirror/pkg/image"
)

func (o *MirrorOptions) pruneRegistry(ctx context.Context, prev, curr image.AssociationSet) error {
	toRemove, err := o.prepAssociationSets(curr, prev)
	if err != nil {
		return err
	}
	return o.pruneImages(ctx, toRemove)
}

func (o *MirrorOptions) pruneImages(ctx context.Context, as image.AssociationSet) error {
	if len(as) == 0 {
		klog.V(4).Infof("No image specified for pruning")
		return nil
	}

	klog.Info("Pruning images outside out range from registry")
	var destInsecure bool
	var terr *transport.Error
	var errs []error
	if o.DestPlainHTTP || o.DestSkipTLS {
		destInsecure = true
	}
	nameOpts := getNameOpts(destInsecure)
	remoteOpts := getRemoteOpts(ctx, destInsecure)

	for imageName := range as {

		// Need to parse twice because name reference cannot be updated
		nameRef, err := name.ParseReference(imageName, nameOpts...)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		err = remote.Delete(nameRef, remoteOpts...)
		switch {
		case err == nil:
			klog.V(4).Infof("image %q removed", imageName)
		case errors.As(err, &terr):
			klog.Warningf("registry %q: %d response code for image deletion request, ending pruning attempt", o.ToMirror, terr.StatusCode)
			return nil
		default:
			errs = append(errs, fmt.Errorf("image %q: pruning error: %v", nameRef.String(), err))
			continue
		}
	}

	return utilerrors.NewAggregate(errs)
}

// prepAssociationSets diff the previous and current set and updated the image key to
// reflect the current mirror and top-level namespace
func (o *MirrorOptions) prepAssociationSets(curr, prev image.AssociationSet) (image.AssociationSet, error) {
	var remove []string
	// TODO(jpower432): Adds Associations for images that are referenced by tag
	// but the manifests have been updated.
	for _, key := range prev.Keys() {
		if found := curr.SetContainsKey(key); !found {
			remove = append(remove, key)
		}
	}

	outputSet, err := image.Prune(prev, remove)
	if err != nil {
		return outputSet, err
	}

	for _, key := range outputSet.Keys() {
		sourceRef, err := reference.Parse(key)
		if err != nil {
			return outputSet, err
		}
		sourceRef.Registry = o.ToMirror
		sourceRef.Namespace = path.Join(o.UserNamespace, sourceRef.Namespace)
		outputSet.UpdateKey(key, sourceRef.Exact())
	}
	return outputSet, nil
}
