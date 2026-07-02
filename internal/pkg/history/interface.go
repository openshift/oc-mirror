package history

import (
	"io"

	"k8s.io/apimachinery/pkg/util/sets"
)

type History interface {
	Read() (sets.Set[string], error)
	Append(sets.Set[string]) (sets.Set[string], error)
}

type FileCreator interface {
	Create(name string) (io.WriteCloser, error)
}
