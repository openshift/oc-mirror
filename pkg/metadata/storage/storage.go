package storage

import (
	"context"
	"errors"
	"io"

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
