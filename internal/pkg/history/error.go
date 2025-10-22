package history

import "fmt"

// This specific error type is returned when .history folder
// doesn't contain any files, especially by the `Read` method
type EmptyHistoryError struct {
	message string
}

func (e *EmptyHistoryError) Error() string {
	return e.message
}

func EmptyHistoryErrorf(format string, a ...any) *EmptyHistoryError {
	return &EmptyHistoryError{
		message: fmt.Sprintf(format, a...),
	}
}

func (e *EmptyHistoryError) Is(err error) bool {
	_, ok := err.(*EmptyHistoryError)
	return ok
}
