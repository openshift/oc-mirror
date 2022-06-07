package mirror

import (
	"errors"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

// ErrInvalidSequence defines an error in imageset sequencing during
// mirroring operations.
type ErrInvalidSequence struct {
	wantSeq int
	gotSeq  int
}

func (s *ErrInvalidSequence) Error() string {
	return fmt.Sprintf("invalid mirror sequence order, want %v, got %v", s.wantSeq, s.gotSeq)
}

func (o *MirrorOptions) checkSequence(incoming, current v1alpha2.Metadata, backendErr error) error {
	switch {
	case backendErr != nil && !errors.Is(backendErr, storage.ErrMetadataNotExist):
		return backendErr
	case o.SkipMetadataCheck:
		return nil
	case backendErr != nil:
		klog.V(1).Infof("No existing metadata found. Setting up new workspace")
		// Check that this is the first imageset
		incomingRun := incoming.PastMirror
		if incomingRun.Sequence != 1 {
			return &ErrInvalidSequence{1, incomingRun.Sequence}
		}
	default:
		// Complete metadata checks
		// UUID mismatch will now be seen as a new workspace.
		klog.V(3).Info("Checking metadata sequence number")
		currRun := current.PastMirror
		incomingRun := incoming.PastMirror
		if incomingRun.Sequence != (currRun.Sequence + 1) {
			return &ErrInvalidSequence{currRun.Sequence + 1, incomingRun.Sequence}
		}
	}
	return nil
}
