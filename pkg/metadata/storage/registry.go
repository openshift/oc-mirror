package storage

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/operator-framework/operator-registry/pkg/image"
	"github.com/operator-framework/operator-registry/pkg/image/containerdregistry"
	"github.com/sirupsen/logrus"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

var _ Backend = &registryBackend{}

type registryBackend struct {
	// Since image contents are represented locally as directories,
	// use the local dir backend as the underlying Backend.
	*localDirBackend
	// Image to use when pushing and pulling
	src string
	// Registry client options
	insecure bool
	ctx      context.Context
}

func NewRegistryBackend(ctx context.Context, cfg *v1alpha1.RegistryConfig, dir string) (Backend, error) {
	r := registryBackend{}
	r.src = cfg.ImageURL
	r.insecure = cfg.SkipTLS
	r.ctx = ctx

	if r.localDirBackend == nil {
		// Create the local dir backend for local r/w.
		lb, err := NewLocalBackend(dir)
		if err != nil {
			return nil, fmt.Errorf("error creating local backend for registry: %w", err)
		}
		r.localDirBackend = lb.(*localDirBackend)
	}

	return &r, nil
}

// ReadMetadata unpacks the metadata image and read it from disk
func (r *registryBackend) ReadMetadata(ctx context.Context, meta *v1alpha1.Metadata, path string) error {
	logrus.Debugf("Checking for existing metadata image at %s", r.src)
	// Check if image exists
	exists, err := r.exists(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return ErrMetadataNotExist
	}

	// Get metadata from image
	err = r.unpack(r.localDirBackend.dir)
	if err != nil {
		return fmt.Errorf("error pulling image %q with metadata: %v", r.src, err)
	}

	// adjust perms, unpack leaves the file user-writable only
	fpath := filepath.Join(r.localDirBackend.dir, path)
	err = os.Chmod(fpath, 0600)
	if err != nil {
		return err
	}

	return r.localDirBackend.ReadMetadata(ctx, meta, path)
}

// WriteMetadata writes the provided metadata to disk anf registry.
func (r *registryBackend) WriteMetadata(ctx context.Context, meta *v1alpha1.Metadata, path string) error {
	return r.WriteObject(ctx, path, meta)
}

// ReadObject reads the provided object from disk.
// In this implementation, key is a file path.
func (r *registryBackend) ReadObject(ctx context.Context, fpath string, obj interface{}) error {
	return r.localDirBackend.ReadObject(ctx, fpath, obj)
}

// WriteObject writes the provided object to disk and registry.
// In this implementation, key is a file path.
func (r *registryBackend) WriteObject(ctx context.Context, fpath string, obj interface{}) (err error) {
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
	if err := r.localDirBackend.WriteObject(ctx, fpath, obj); err != nil {
		return err
	}
	logrus.Debugf("Pushing metadata to registry at %s", r.src)
	return r.pushImage(data, fpath)
}

// GetWriter returns an os.File as a writer.
// In this implementation, key is a file path.
func (r *registryBackend) GetWriter(ctx context.Context, fpath string) (io.Writer, error) {
	return r.localDirBackend.GetWriter(ctx, fpath)
}

// CheckConfig will return an error if the StorageConfig
// is not a registry
func (r *registryBackend) CheckConfig(storage v1alpha1.StorageConfig) error {
	if storage.Registry == nil {
		return fmt.Errorf("not registry backend")
	}
	return nil
}

// pushImage will push a v1.Image with provided contents
func (r *registryBackend) pushImage(data []byte, fpath string) error {
	var options []crane.Option

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: r.insecure}
	var rt http.RoundTripper = transport
	options = append(options, crane.WithTransport(rt))
	options = append(options, crane.WithContext(r.ctx))

	contents := map[string][]byte{
		fpath: data,
	}
	i, _ := crane.Image(contents)
	return crane.Push(i, r.src, options...)
}

func (r *registryBackend) createRegistry() (*containerdregistry.Registry, error) {
	cacheDir, err := os.MkdirTemp("", "imageset-catalog-registry-")
	if err != nil {
		return nil, err
	}

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)
	nullLogger := logrus.NewEntry(logger)

	return containerdregistry.NewRegistry(
		containerdregistry.WithCacheDir(cacheDir),
		containerdregistry.SkipTLS(r.insecure),
		// The containerd registry impl is somewhat verbose, even on the happy path,
		// so discard all logger logs}. Any important failures will be returned from
		// registry methods and eventually logged as fatal errors.
		containerdregistry.WithLog(nullLogger),
	)
}

func (r *registryBackend) unpack(path string) error {
	reg, err := r.createRegistry()
	if err != nil {
		return fmt.Errorf("error creating container registry: %v", err)
	}
	defer reg.Destroy()
	ref := image.SimpleReference(r.src)
	if err := reg.Pull(r.ctx, ref); err != nil {
		return err
	}
	_, err = reg.Labels(r.ctx, ref)
	if err != nil {
		return err
	}
	return reg.Unpack(r.ctx, ref, path)
}

// exists checks if the image exists
func (r *registryBackend) exists(ctx context.Context) (bool, error) {
	repo, err := r.repoExists(ctx)
	if err != nil {
		return false, err
	}
	if repo {
		tag, err := r.tagExists()
		if err != nil {
			return false, err
		}
		if tag {
			return true, nil
		}
	}
	return false, nil
}

// tagExists checks if the image tag exists in the repository
func (r *registryBackend) tagExists() (bool, error) {
	idx := strings.LastIndex(r.src, ":")
	if idx == -1 {
		return false, fmt.Errorf("image %q has no tag or digest component", r.src)
	}
	repo, err := name.NewRepository(r.src[:idx])
	if err != nil {
		return false, err
	}

	// TODO: Get default auth will need to update if user
	// can specifiy custom locations
	opts := remote.WithAuthFromKeychain(authn.DefaultKeychain)
	tags, err := remote.List(repo, opts)
	if err != nil {
		return false, err
	}
	for _, tag := range tags {
		if tag == r.src[idx+1:] {
			return true, nil
		}
	}
	return false, nil
}

// repoExists checks if the image repo exists in the registry
func (r *registryBackend) repoExists(ctx context.Context) (bool, error) {
	idx := strings.Index(r.src, "/")
	if idx == -1 {
		return false, fmt.Errorf("image %q has image name componenet", r.src)
	}
	reg, err := name.NewRegistry(r.src[:idx])
	if err != nil {
		return false, err
	}
	image := r.src[idx+1:]
	if idIdx := strings.LastIndex(image, ":"); idIdx != -1 {
		idx = idIdx
	}

	// TODO: Get default auth will need to update if user
	// can specifiy custom locations
	opts := remote.WithAuthFromKeychain(authn.DefaultKeychain)
	repos, err := remote.Catalog(ctx, reg, opts)
	if err != nil {
		return false, err
	}
	for _, repo := range repos {
		if repo == image[:idx] {
			return true, nil
		}
	}
	return false, nil
}
