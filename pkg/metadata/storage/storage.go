package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

var (
	// ErrMetadataNotExist should be returned by ReadMetadata() when no metadata is found.
	// Callers should check for this error, since in certain conditions no metadata is desired.
	ErrMetadataNotExist     = errors.New("metadata does not exist")
	ErrBackendNotConfigured = errors.New("no backend specified in config")
)

// TODO: consider consolidating {Read,Write}Metadata() into the
// generic {Read,Write}Object() methods respectively.

type Backend interface {
	ReadMetadata(context.Context, *v1alpha1.Metadata, string) error
	WriteMetadata(context.Context, *v1alpha1.Metadata, string) error
	ReadObject(context.Context, string, interface{}) error
	WriteObject(context.Context, string, interface{}) error
	GetWriter(context.Context, string) (io.Writer, error)
	CheckConfig(v1alpha1.StorageConfig) error
	Open(string) (io.ReadCloser, error)
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
func ByConfig(dir string, storage v1alpha1.StorageConfig) (Backend, error) {
	var b interface{}
	for _, bk := range backends {
		if err := bk.CheckConfig(storage); err == nil {
			b = bk
			break
		}
	}
	switch b.(type) {
	case *localDirBackend:
		return NewLocalBackend(storage.Local.Path)
	case *registryBackend:
		logrus.Infof("Using registry backend at location %s", storage.Registry.ImageURL)
		return NewRegistryBackend(storage.Registry, dir)
	default:
		// If no local or registry backend is
		// configured, send back the default option with an error
		be, err := NewLocalBackend(dir)
		if err != nil {
			return nil, err
		}
		return be, ErrBackendNotConfigured
	}
}

func getTypeMeta(data []byte) (typeMeta metav1.TypeMeta, err error) {
	if err := yaml.Unmarshal(data, &typeMeta); err != nil {
		return typeMeta, fmt.Errorf("get type meta: %v", err)
	}
	return typeMeta, nil
}
