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

type ExecutorSchemaError struct {
	orig     error
	exitCode uint8
}

func NewExecutorSchemaError(err error, exitCode uint8) *ExecutorSchemaError {
	return &ExecutorSchemaError{err, exitCode}
}

func (e *ExecutorSchemaError) Error() string {
	return e.orig.Error()
}

func (e *ExecutorSchemaError) Code() uint8 {
	return e.exitCode
}
