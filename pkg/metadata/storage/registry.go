package storage

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/operator-framework/operator-registry/pkg/image"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"github.com/sirupsen/logrus"

	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
)

var _ Backend = &registryBackend{}

type registryBackend struct {
	// Since image contents are represented locally as directories,
	// use the local dir backend as the underlying Backend.
	*localDirBackend
	// Image to use when pushing and pulling
	src imagesource.TypedImageReference
	// Registry client options
	insecure bool
}

func NewRegistryBackend(cfg *v1alpha1.RegistryConfig, dir string) (Backend, error) {
	b := registryBackend{}
	b.insecure = cfg.SkipTLS

	ref, err := imagesource.ParseReference(cfg.ImageURL)
	if err != nil {
		return nil, err
	}
	if len(ref.Ref.Tag) == 0 {
		ref.Ref.Tag = "latest"
	}
	b.src = ref

	logrus.Info(ref)

	if b.localDirBackend == nil {
		// Create the local dir backend for local r/w.
		lb, err := NewLocalBackend(dir)
		if err != nil {
			return nil, fmt.Errorf("error creating local backend for registry: %w", err)
		}
		b.localDirBackend = lb.(*localDirBackend)
	}

	return &b, nil
}

// ReadMetadata unpacks the metadata image and read it from disk
func (b *registryBackend) ReadMetadata(ctx context.Context, meta *v1alpha1.Metadata, path string) error {
	logrus.Debugf("Checking for existing metadata image at %s", b.src)
	// Check if image exists
	if err := b.exists(ctx); err != nil {
		return err
	}

	// Get metadata from image
	err := b.unpack(ctx, b.localDirBackend.dir)
	if err != nil {
		return fmt.Errorf("error pulling image %q with metadata: %v", b.src, err)
	}
	// adjust perms, unpack leaves the file user-writable only
	fpath := filepath.Join(b.localDirBackend.dir, path)
	err = os.Chmod(fpath, 0600)
	if err != nil {
		return err
	}
	return b.localDirBackend.ReadMetadata(ctx, meta, path)
}

// WriteMetadata writes the provided metadata to disk anf registry.
func (b *registryBackend) WriteMetadata(ctx context.Context, meta *v1alpha1.Metadata, path string) error {
	return b.WriteObject(ctx, path, meta)
}

// ReadObject reads the provided object from disk.
// In this implementation, key is a file path.
func (b *registryBackend) ReadObject(ctx context.Context, fpath string, obj interface{}) error {
	return b.localDirBackend.ReadObject(ctx, fpath, obj)
}

// WriteObject writes the provided object to disk and registry.
// In this implementation, key is a file path.
func (b *registryBackend) WriteObject(ctx context.Context, fpath string, obj interface{}) (err error) {
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

	// Write metadata to disk for packing into archive
	if err := b.localDirBackend.WriteObject(ctx, fpath, obj); err != nil {
		return err
	}
	logrus.Debugf("Pushing metadata to registry at %s", b.src)
	return b.pushImage(ctx, data, fpath)
}

// GetWriter returns an os.File as a writeb.
// In this implementation, key is a file path.
func (b *registryBackend) GetWriter(ctx context.Context, fpath string) (io.Writer, error) {
	return b.localDirBackend.GetWriter(ctx, fpath)
}

// Open reads the provided object from a registry source and provides an io.ReadCloser
func (b *registryBackend) Open(fpath string) (io.ReadCloser, error) {
	// Assumes this is in place on disk
	// QUESTION(jpower): can we make this assumption here?
	return b.localDirBackend.Open(fpath)
}

// Stat checks the existence of the metadata from a registry source
func (b *registryBackend) Stat(ctx context.Context, fpath string) (os.FileInfo, error) {
	logrus.Debugf("Checking for existing metadata image at %s", b.src.String())
	// Check if image exists
	if err := b.exists(ctx); err != nil {
		return nil, err
	}
	// Assumes this is in place on disk
	// QUESTION(jpower): can we make this assumption here?
	return b.localDirBackend.Stat(ctx, fpath)
}

// Cleanup removes metadata from existing metadata from backend location
func (b *registryBackend) Cleanup(ctx context.Context, fpath string) error {
	options := b.getOpts(ctx)
	if err := crane.Delete(b.src.Ref.Exact(), options...); err != nil {
		return err
	}
	return b.localDirBackend.Cleanup(ctx, fpath)
}

// CheckConfig will return an error if the StorageConfig
// is not a registry
func (b *registryBackend) CheckConfig(storage v1alpha1.StorageConfig) error {
	if storage.Registry == nil {
		return fmt.Errorf("not registry backend")
	}
	return nil
}

// pushImage will push a v1.Image with provided contents
func (b *registryBackend) pushImage(ctx context.Context, data []byte, fpath string) error {
	options := b.getOpts(ctx)
	contents := map[string][]byte{
		fpath: data,
	}
	i, _ := crane.Image(contents)
	return crane.Push(i, b.src.Ref.Exact(), options...)
}

func (b *registryBackend) createRegistry() (*containerdregistry.Registry, error) {
	cacheDir, err := os.MkdirTemp("", "imageset-catalog-registry-")
	if err != nil {
		return nil, err
	}

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)
	nullLogger := logrus.NewEntry(logger)

	return containerdregistry.NewRegistry(
		containerdregistry.WithCacheDir(cacheDir),
		containerdregistry.SkipTLS(b.insecure),
		// The containerd registry impl is somewhat verbose, even on the happy path,
		// so discard all logger logs}. Any important failures will be returned from
		// registry methods and eventually logged as fatal errors.
		containerdregistry.WithLog(nullLogger),
	)
}

func (b *registryBackend) unpack(ctx context.Context, path string) error {
	reg, err := b.createRegistry()
	if err != nil {
		return fmt.Errorf("error creating container registry: %v", err)
	}
	defer reg.Destroy()
	ref := image.SimpleReference(b.src.Ref.String())
	if err := reg.Pull(ctx, ref); err != nil {
		return err
	}
	_, err = reg.Labels(ctx, ref)
	if err != nil {
		return err
	}
	return reg.Unpack(ctx, ref, path)
}

// exists checks if the image exists
func (b *registryBackend) exists(ctx context.Context) error {
	_, err := crane.Manifest(b.src.Ref.Exact(), b.getOpts(ctx)...)
	var terr *transport.Error
	switch {
	case err == nil:
		return nil
	case err != nil && errors.As(err, &terr):
		return ErrMetadataNotExist
	default:
		return err
	}
}

func (b *registryBackend) createRT() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: b.insecure}
	return transport
}

// TODO: Get default auth will need to update if user
// can specify custom locations
func (b *registryBackend) getOpts(ctx context.Context) (options []crane.Option) {
	return append(
		options,
		crane.WithAuthFromKeychain(authn.DefaultKeychain),
		crane.WithContext(ctx),
		crane.WithTransport(b.createRT()),
	)
}
