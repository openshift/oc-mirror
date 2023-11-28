package cli

import "fmt"

type NormalStorageInterruptError struct {
	message string
}

func (e *NormalStorageInterruptError) Error() string {
	return e.message
}

func NormalStorageInterruptErrorf(format string, a ...any) *NormalStorageInterruptError {
	return &NormalStorageInterruptError{
		message: fmt.Sprintf(format, a...),
	}
}

func (e *NormalStorageInterruptError) Is(err error) bool {
	_, ok := err.(*NormalStorageInterruptError)
	return ok
}
