package mirror

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/archive"
	"github.com/openshift/oc-mirror/pkg/bundle"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

var (
	// ErrNoUpdatesExist should be returned by Create() when no updates are found
	ErrNoUpdatesExist = errors.New("no updates detected, process stopping")
)

const (
	// defaultSegSize is the default maximum archive size.
	defaultSegSize int64 = 500
	// segMultiplier is the multiplier used to
	// convert segSize to GiB
	segMultiplier int64 = 1024 * 1024 * 1024
)

// Pack will pack the imageset and return a temporary backend storing metadata for final push
// The metadata has been updated by the plan stage at this point but not pushed to the backend
func (o *MirrorOptions) Pack(ctx context.Context, prevAssocs, currAssocs image.AssociationSet, meta *v1alpha2.Metadata, archiveSize int64) (storage.Backend, error) {
	tmpdir, _, err := o.mktempDir()
	if err != nil {
		return nil, err
	}
	tmpBackend, err := storage.NewLocalBackend(tmpdir)
	if err != nil {
		return nil, err
	}

	// Update metadata files and get newly created filepaths.
	diskPath := filepath.Join(o.Dir, config.SourceDir, config.V2Dir)
	// Define a map that associates locations
	// on disk to location in archive
	paths := map[string]string{diskPath: config.V2Dir}
	reconcileAssociation := image.AssociationSet{}
	if !o.IgnoreHistory {
		reconcileAssociation = prevAssocs
	}
	manifests, blobs, err := bundle.ReconcileV2Dir(reconcileAssociation, paths)
	if err != nil {
		return tmpBackend, fmt.Errorf("error reconciling v2 files: %v", err)
	}

	// Stop the process if no new blobs
	if len(blobs) == 0 {
		return tmpBackend, ErrNoUpdatesExist
	}

	// Update Association in PastMirror to the current value and update
	meta.PastMirror.Associations, err = image.ConvertFromAssociationSet(currAssocs)
	if err != nil {
		return tmpBackend, err
	}
	prevAssocs.Merge(currAssocs)
	meta.PastAssociations, err = image.ConvertFromAssociationSet(prevAssocs)
	if err != nil {
		return tmpBackend, err
	}
	if err := metadata.UpdateMetadata(ctx, tmpBackend, meta, filepath.Join(o.Dir, config.SourceDir), o.SourceSkipTLS, o.SourcePlainHTTP); err != nil {
		return tmpBackend, err
	}

	if err := o.prepareArchive(ctx, tmpBackend, archiveSize, meta.PastMirror.Sequence, manifests, blobs); err != nil {
		return tmpBackend, err
	}

	/* Commenting out temporarily because no concrete types implement this
	if committer, isCommitter := backend.(storage.Committer); isCommitter {
		if err := committer.Commit(ctx); err != nil {
			return err
		}
	}*/

	return tmpBackend, nil
}

func (o *MirrorOptions) prepareArchive(ctx context.Context, backend storage.Backend, archiveSize int64, seq int, manifests, blobs []string) error {

	segSize := defaultSegSize
	if archiveSize != 0 {
		segSize = archiveSize
		klog.V(4).Infof("Using user provided archive size %d GiB", segSize)
	}
	segSize *= segMultiplier

	// Set get absolute path to output dir
	// to avoid issue with directory change
	output, err := filepath.Abs(o.OutputDir)
	if err != nil {
		return err
	}

	// Change directory before archiving to
	// avoid broken symlink paths
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(filepath.Join(o.Dir, config.SourceDir)); err != nil {
		return err
	}
	defer os.Chdir(cwd)

	packager := archive.NewPackager(manifests, blobs)
	prefix := fmt.Sprintf("mirror_seq%d", seq)
	if err := packager.CreateSplitArchive(ctx, backend, segSize, output, ".", prefix, o.SkipCleanup); err != nil {
		return fmt.Errorf("failed to create archive: %v", err)
	}
	return nil
}

func (o *MirrorOptions) mktempDir() (string, func(), error) {
	// Placing this under the source directory, so it will be cleaned up
	// at the end of operators if cleanup func is not used
	dir := filepath.Join(o.Dir, config.SourceDir, fmt.Sprintf("tmpbackend.%d", time.Now().Unix()))
	return dir, func() {
		if err := os.RemoveAll(dir); err != nil {
			klog.Error(err)
		}
	}, os.MkdirAll(dir, os.ModePerm)
}
