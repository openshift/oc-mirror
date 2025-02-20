package batch

type UnsafeError struct {
	errSchema mirrorErrorSchema
}

func NewUnsafeError(mes mirrorErrorSchema) error {
	return UnsafeError{mes}
}

func (e UnsafeError) Error() string { return e.errSchema.err.Error() }

// func isFailSafe(err error) bool {
// 	switch err {
// 	case nil:
// 		return true
// 	case context.Canceled, context.DeadlineExceeded:
// 		return false
// 	default: // continue
// 	}

// 	type unwrapper interface {
// 		Unwrap() error
// 	}

// 	switch e := err.(type) {

// 	case errcode.Error:
// 		switch e.Code {
// 		case errcode.ErrorCodeUnauthorized, errcode.ErrorCodeDenied:
// 			// For oc-mirror this is a grey area: we consider that having Authentication
// 			// or authorization errors to a registry (whether source or destination) on one
// 			// image is very likely to give similar errors on other images.
// 			return false
// 		}
// 		// normally manifest unknown, blob unknown, etc fall into the following return
// 		return true
// 	case *net.OpError:
// 		return isFailSafe(e.Err)
// 	case *url.Error: // This includes errors returned by the net/http client.
// 		if e.Err == io.EOF { // Happens when a server accepts a HTTP connection and sends EOF
// 			return true
// 		}
// 		return isFailSafe(e.Err)
// 	case syscall.Errno:
// 		return isErrnoFailSafe(e)
// 	case errcode.Errors:
// 		// if this error is a group of errors, process them all in turn
// 		for i := range e {
// 			if !isFailSafe(e[i]) {
// 				return false
// 			}
// 		}
// 		return true
// 	case *multierror.Error:
// 		// if this error is a group of errors, process them all in turn
// 		for i := range e.Errors {
// 			if !isFailSafe(e.Errors[i]) {
// 				return false
// 			}
// 		}
// 		return true
// 	case net.Error:
// 		if e.Timeout() {
// 			return true
// 		}
// 		if unwrappable, ok := e.(unwrapper); ok {
// 			err = unwrappable.Unwrap()
// 			return isFailSafe(err)
// 		}
// 	case unwrapper: // Test this last, because various error types might implement .Unwrap()
// 		err = e.Unwrap()
// 		return isFailSafe(err)
// 	}

// 	return false
// }

// func isErrnoFailSafe(e error) bool {
// 	switch e {
// 	case syscall.ECONNREFUSED, syscall.ENETDOWN, syscall.ENETUNREACH, syscall.EHOSTDOWN, syscall.EHOSTUNREACH:
// 		return false
// 	case syscall.EINTR, syscall.EAGAIN, syscall.EBUSY, syscall.ENETRESET, syscall.ECONNABORTED, syscall.ECONNRESET, syscall.ETIMEDOUT:
// 		return true
// 	}
// 	return isErrnoERESTART(e)
// }

// func isErrnoERESTART(e error) bool {
// 	return e == syscall.ERESTART
// }
