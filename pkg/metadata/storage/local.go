package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

var _ Backend = &localDirBackend{}

type localDirBackend struct {
	fs  afero.Fs
	dir string
}

func NewLocalBackend(dir string) (Backend, error) {
	b := localDirBackend{
		dir: dir,
	}
	return &b, b.init()
}

func (b *localDirBackend) init() error {
	if b.fs == nil {
		b.fs = afero.NewOsFs()
	}

	if err := b.fs.MkdirAll(b.dir, 0755); err != nil {
		return err
	}

	// Use a basepath FS to obviate joining paths later.
	// Do this after creating the dir using the underlying fs
	// so b.dir is not created under the base (itself).
	b.fs = afero.NewBasePathFs(b.fs, b.dir)

	return nil
}

// WriteMetadata reads the provided metadata from disk.
func (b *localDirBackend) ReadMetadata(_ context.Context, meta *v1alpha1.Metadata) error {

	logrus.Debugf("looking for metadata file at %q", config.MetadataBasePath)

	data, err := afero.ReadFile(b.fs, config.MetadataBasePath)
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
	case v1alpha1.GroupVersion.WithKind(v1alpha1.MetadataKind):
		*meta, err = v1alpha1.LoadMetadata(data)
	default:
		return fmt.Errorf("config GVK not recognized: %s", typeMeta.GroupVersionKind())
	}
	if err != nil {
		return err
	}

	return nil
}

func getTypeMeta(data []byte) (typeMeta metav1.TypeMeta, err error) {
	if err := yaml.Unmarshal(data, &typeMeta); err != nil {
		return typeMeta, fmt.Errorf("get type meta: %v", err)
	}
	return typeMeta, nil
}

// WriteMetadata writes the provided metadata to disk.
func (b *localDirBackend) WriteMetadata(ctx context.Context, meta *v1alpha1.Metadata) error {
	return b.WriteObject(ctx, config.MetadataBasePath, meta)
}

// ReadObject reads the provided object from disk.
// In this implementation, key is a file path.
func (b *localDirBackend) ReadObject(_ context.Context, fpath string, obj interface{}) error {

	data, err := afero.ReadFile(b.fs, fpath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, obj)
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

	// Create an child dirs necessary.
	if err := b.fs.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
		return nil, fmt.Errorf("error creating object child path: %v", err)
	}

	w, err := b.fs.OpenFile(fpath, os.O_WRONLY|os.O_CREATE, 0640)
	if err != nil {
		return nil, fmt.Errorf("error opening object file: %v", err)
	}

	return w, nil
}
