package history

import "io"

//go:generate mockgen -source=./interface.go -destination=./mock/interface_generated.go -package=mock

type History interface {
	Read() (map[string]string, error)
	Append(map[string]string) (map[string]string, error)
}

type FileCreator interface {
	Create(name string) (io.WriteCloser, error)
}
