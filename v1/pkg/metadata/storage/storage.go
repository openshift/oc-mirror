package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

var (
	// ErrMetadataNotExist should be returned by ReadMetadata() when no metadata is found.
	// Callers should check for this error, since in certain conditions no metadata is desired.
	ErrMetadataNotExist = errors.New("metadata does not exist")
)

// TODO: consider consolidating {Read,Write}Metadata() into the
// generic {Read,Write}Object() methods respectively.

type Backend interface {
	ReadMetadata(context.Context, *v1alpha2.Metadata, string) error
	WriteMetadata(context.Context, *v1alpha2.Metadata, string) error
	ReadObject(context.Context, string, interface{}) error
	WriteObject(context.Context, string, interface{}) error
	GetWriter(context.Context, string) (io.Writer, error)
	CheckConfig(v1alpha2.StorageConfig) error
	Open(context.Context, string) (io.ReadCloser, error)
	Stat(context.Context, string) (os.FileInfo, error)
	Cleanup(context.Context, string) error
}

// Committer is a Backend that collects a set of write operations into a transaction
// then commits them atomically. The technologies underlying these Backends are
// typically transactional by nature (like git); this interface exposes that nature.
type Committer interface {
	// Commit the set of writes to the Backend for persistence.
	// Commit is NOT guaranteed to be threadsafe, see implementer comments for details.
	Commit(context.Context) error
}

var backends = []Backend{
	&localDirBackend{},
	&registryBackend{},
}

// ByConfig returns backend interface based on provided config
func ByConfig(dir string, storage v1alpha2.StorageConfig) (Backend, error) {
	var b interface{}
	for _, bk := range backends {
		if err := bk.CheckConfig(storage); err == nil {
			b = bk
			break
		}
	}
	switch b.(type) {
	case *localDirBackend:
		klog.V(1).Infof("Using local backend at location %s", storage.Local.Path)
		return NewLocalBackend(storage.Local.Path)
	case *registryBackend:
		klog.V(1).Infof("Using registry backend at location %s", storage.Registry.ImageURL)
		return NewRegistryBackend(storage.Registry, dir)
	default:
		return nil, errors.New("unsupported backend configuration")
	}
}

func getTypeMeta(data []byte) (typeMeta metav1.TypeMeta, err error) {
	if err := yaml.Unmarshal(data, &typeMeta); err != nil {
		return typeMeta, fmt.Errorf("get type meta: %v", err)
	}
	return typeMeta, nil
}
