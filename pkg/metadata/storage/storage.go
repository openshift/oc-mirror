package storage

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

var (
	// ErrMetadataNotExist should be returned by ReadMetadata() when no metadata is found.
	// Callers should check for this error, since in certain conditions no metadata is desired.
	ErrMetadataNotExist = errors.New("metadata does not exist")
)

// TODO: consider consolidating {Read,Write}Metadata() into the
// generic {Read,Write}Object() methods respectively.

type Backend interface {
	ReadMetadata(context.Context, *v1alpha1.Metadata, string) error
	WriteMetadata(context.Context, *v1alpha1.Metadata, string) error
	ReadObject(context.Context, string, interface{}) error
	WriteObject(context.Context, string, interface{}) error
	GetWriter(context.Context, string) (io.Writer, error)
}

// Committer is a Backend that collects a set of write operations into a transaction
// then commits them atomically. The technologies underlying these Backends are
// typically transactional by nature (like git); this interface exposes that nature.
type Committer interface {
	// Commit the set of writes to the Backend for persistence.
	// Commit is NOT guaranteed to be threadsafe, see implementer comments for details.
	Commit(context.Context) error
}

// CallBackend calls a new backend by string prefix
func CallBackend(b string) (Backend, error) {
	switch {
	case strings.Contains(b, "file://"):
		return NewLocalBackend(b)
	case strings.Contains(b, "git@") || (strings.Contains(b, "https://") && strings.Contains(b, ".git")):
		return nil, errors.New("git backend is not implemented yet")
	case strings.Contains(b, "s3://"):
		return nil, errors.New("s3 backend is not implemented yet")
	default:
		return nil, errors.New("unknown backend syntax")
	}
}
