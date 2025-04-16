package history

import "io"

type History interface {
	Read() (map[string]struct{}, error)
	Append(map[string]struct{}) (map[string]struct{}, error)
}

type FileCreator interface {
	Create(name string) (io.WriteCloser, error)
}
