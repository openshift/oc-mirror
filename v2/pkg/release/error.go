package release

import "fmt"

type NotImplementedError struct {
	message string
}

func (e *NotImplementedError) Error() string {
	return e.message
}

func NotImplementedErrorf(format string, a ...any) *NotImplementedError {
	return &NotImplementedError{
		message: fmt.Sprintf(format, a...),
	}
}

func (e *NotImplementedError) Is(err error) bool {
	_, ok := err.(*NotImplementedError)
	return ok
}
