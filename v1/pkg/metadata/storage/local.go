package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/config"
)

var _ Backend = &localDirBackend{}

type localDirBackend struct {
	fs  afero.Fs
	dir string
}

func NewLocalBackend(dir string) (Backend, error) {

	// Get absolute path for provided dir
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	b := localDirBackend{
		dir: absDir,
	}
	return &b, b.init()
}

func (b *localDirBackend) init() error {
	if b.fs == nil {
		b.fs = afero.NewOsFs()
	}

	if err := b.fs.MkdirAll(b.dir, 0750); err != nil {
		return err
	}

	// Use a basepath FS to obviate joining paths later.
	// Do this after creating the dir using the underlying fs
	// so b.dir is not created under the base (itself).
	b.fs = afero.NewBasePathFs(b.fs, b.dir)

	return nil
}

// ReadMetadata reads the provided metadata from disk.
func (b *localDirBackend) ReadMetadata(_ context.Context, meta *v1alpha2.Metadata, path string) error {

	klog.V(2).Infof("looking for metadata file at %q", path)

	data, err := afero.ReadFile(b.fs, path)
	if err != nil {
		// Non-existent metadata is allowed.
		if errors.Is(err, os.ErrNotExist) {
			return ErrMetadataNotExist
		}
		return err
	}

	typeMeta, err := getTypeMeta(data)
	if err != nil {
		return err
	}

	switch typeMeta.GroupVersionKind() {
	case v1alpha2.GroupVersion.WithKind(v1alpha2.MetadataKind):
		*meta, err = config.LoadMetadata(data)
	default:
		return fmt.Errorf("config GVK not recognized: %s", typeMeta.GroupVersionKind())
	}
	if err != nil {
		return err
	}

	return nil
}

// WriteMetadata writes the provided metadata to disk.
func (b *localDirBackend) WriteMetadata(ctx context.Context, meta *v1alpha2.Metadata, path string) error {
	return b.WriteObject(ctx, path, meta)
}

// ReadObject reads the provided object from disk.
// In this implementation, key is a file path.
func (b *localDirBackend) ReadObject(_ context.Context, fpath string, obj interface{}) error {

	data, err := afero.ReadFile(b.fs, fpath)
	if err != nil {
		return err
	}

	switch v := obj.(type) {
	case []byte:
		if len(v) < len(data) {
			return io.ErrShortBuffer
		}
		copy(v, data)
	case io.Writer:
		_, err = v.Write(data)
	default:
		err = json.Unmarshal(data, obj)
	}
	return err
}

// WriteObject writes the provided object to disk.
// In this implementation, key is a file path.
func (b *localDirBackend) WriteObject(ctx context.Context, fpath string, obj interface{}) error {

	w, err := b.GetWriter(ctx, fpath)
	if err != nil {
		return err
	}
	defer w.(io.WriteCloser).Close()

	var data []byte
	switch v := obj.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	case io.Reader:
		data, err = io.ReadAll(v)
	default:
		data, err = json.Marshal(obj)
	}
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

// GetWriter returns an os.File as a writer.
// In this implementation, key is a file path.
func (b *localDirBackend) GetWriter(_ context.Context, fpath string) (io.Writer, error) {

	// Create a child dirs necessary.
	if err := b.fs.MkdirAll(filepath.Dir(fpath), 0750); err != nil {
		return nil, fmt.Errorf("error creating object child path: %v", err)
	}

	w, err := b.fs.OpenFile(fpath, os.O_WRONLY|os.O_CREATE, 0640)
	if err != nil {
		return nil, fmt.Errorf("error opening object file: %v", err)
	}

	return w, nil
}

// Open reads the provided object from a local source and provides an io.ReadCloser
func (b *localDirBackend) Open(_ context.Context, fpath string) (io.ReadCloser, error) {
	return b.fs.Open(fpath)
}

// Stat checks the existence of the metadata from a local source
func (b *localDirBackend) Stat(_ context.Context, fpath string) (os.FileInfo, error) {
	info, err := b.fs.Stat(fpath)
	switch {
	case err != nil && errors.Is(err, os.ErrNotExist):
		return nil, ErrMetadataNotExist
	case err != nil:
		return nil, err
	default:
		return info, nil
	}
}

// Cleanup removes remove metadata from existing metadata from backend location
func (b *localDirBackend) Cleanup(_ context.Context, fpath string) error {
	return b.fs.RemoveAll(fpath)
}

func (b *localDirBackend) CheckConfig(storage v1alpha2.StorageConfig) error {
	if storage.Local == nil {
		return fmt.Errorf("not local backend")
	}
	return nil
}
