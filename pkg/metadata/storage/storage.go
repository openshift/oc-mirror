package storage

import (
	"context"
	"errors"
	"io"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

// TODO: consider consolidating {Read,Write}Metadata() into the
// generic {Read,Write}Object() methods respectively.

type Backend interface {
	ReadMetadata(context.Context, *v1alpha1.Metadata) error
	WriteMetadata(context.Context, *v1alpha1.Metadata) error
	ReadObject(context.Context, string, interface{}) error
	WriteObject(context.Context, string, interface{}) error
	GetWriter(context.Context, string) (io.Writer, error)
}

var (
	// ErrMetadataNotExist should be returned by ReadMetadata() when no metadata is found.
	// Callers should check for this error, since in certain conditions no metadata is desired.
	ErrMetadataNotExist = errors.New("metadata does not exist")
)
