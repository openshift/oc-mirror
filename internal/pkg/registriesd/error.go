package registriesd

import (
	"fmt"
)

type configUnmarshalError struct {
	err error
}

func (e configUnmarshalError) Error() string {
	return fmt.Sprintf("registriesd config unmarshal error: %s", e.err)
}
