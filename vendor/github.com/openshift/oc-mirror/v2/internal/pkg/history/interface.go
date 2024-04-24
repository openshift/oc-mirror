package history

import "io"

type History interface {
	Read() (map[string]string, error)
	Append(map[string]string) (map[string]string, error)
}

type FileCreator interface {
	Create(name string) (io.WriteCloser, error)
}
