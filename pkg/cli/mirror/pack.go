package mirror

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/openshift/oc-mirror/pkg/archive"
	"github.com/openshift/oc-mirror/pkg/bundle"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

var (
	// NoUpdatesExist should be returned by Create() when no updates are found
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
func (o *MirrorOptions) Pack(ctx context.Context, assocs image.AssociationSet, meta v1alpha1.Metadata, archiveSize int64) (storage.Backend, error) {
	tmpdir, _, err := o.mktempDir()
	if err != nil {
		return nil, err
	}
	cfg := v1alpha1.StorageConfig{
		Local: &v1alpha1.LocalConfig{Path: tmpdir},
	}
	tmpBackend, err := storage.ByConfig(tmpdir, cfg)
	if err != nil {
		return nil, err
	}

	if err := o.writeAssociations(assocs); err != nil {
		return tmpBackend, fmt.Errorf("error writing association file: %v", err)
	}

	currRun := meta.PastMirrors[len(meta.PastMirrors)-1]
	// Update metadata files and get newly created filepaths.
	manifests, blobs, err := o.getFiles(meta)
	if err != nil {
		return tmpBackend, err
	}

	// Stop the process if no new blobs
	if len(blobs) == 0 {
		return tmpBackend, ErrNoUpdatesExist
	}

	// Add only the new manifests and blobs created to the current run.
	currRun.Manifests = append(currRun.Manifests, manifests...)
	currRun.Blobs = append(currRun.Blobs, blobs...)
	// Add this run and metadata to top level metadata.
	meta.PastMirrors[len(meta.PastMirrors)-1] = currRun
	meta.PastBlobs = append(meta.PastBlobs, blobs...)

	// Update the metadata.
	if err := metadata.UpdateMetadata(ctx, tmpBackend, &meta, o.SourceSkipTLS, o.SourcePlainHTTP); err != nil {
		return tmpBackend, err
	}

	// If any errors occur after the metadata is written
	// initiate metadata rollback
	if err := o.prepareArchive(ctx, tmpBackend, archiveSize, currRun.Sequence, manifests, blobs); err != nil {
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

func (o *MirrorOptions) prepareArchive(ctx context.Context, backend storage.Backend, archiveSize int64, seq int, manifests []v1alpha1.Manifest, blobs []v1alpha1.Blob) error {

	segSize := defaultSegSize
	if archiveSize != 0 {
		segSize = archiveSize
		logrus.Debugf("Using user provided archive size %d GiB", segSize)
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

func (o *MirrorOptions) getFiles(meta v1alpha1.Metadata) ([]v1alpha1.Manifest, []v1alpha1.Blob, error) {
	diskPath := filepath.Join(o.Dir, config.SourceDir, "v2")
	// Define a map that associates locations
	// on disk to location in archive
	paths := map[string]string{diskPath: "v2"}
	return bundle.ReconcileV2Dir(meta, paths)
}

func (o *MirrorOptions) writeAssociations(assocs image.AssociationSet) error {
	assocPath := filepath.Join(o.Dir, config.SourceDir, config.AssociationsBasePath)
	if err := os.MkdirAll(filepath.Dir(assocPath), 0755); err != nil {
		return fmt.Errorf("mkdir image associations file: %v", err)
	}
	f, err := os.OpenFile(assocPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		return fmt.Errorf("open image associations file: %v", err)
	}
	defer f.Close()
	return assocs.Encode(f)
}

func (o *MirrorOptions) mktempDir() (string, func(), error) {
	// Placing this under the source directory, so it will be cleaned up
	// at the end of operators if cleanup func is not used
	dir := filepath.Join(o.Dir, config.SourceDir, fmt.Sprintf("tmpbackend.%d", time.Now().Unix()))
	return dir, func() {
		if err := os.RemoveAll(dir); err != nil {
			logrus.Error(err)
		}
	}, os.MkdirAll(dir, os.ModePerm)
}
